package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestSessionTargetStatusKey_WhenActive_ItShouldPrefixStatus(t *testing.T) {
	deadline := "2026-10-17T12:00:00.000000000Z"
	got := SessionTargetStatusKey(SessionStatusActive, deadline)
	if got != "active#"+deadline {
		t.Errorf("unexpected key: %q", got)
	}
}

func TestDynamoDBSessionStore_Put_WhenSuccess_ItShouldSetTTLAndTargetStatusKey(t *testing.T) {
	var capturedInput *dynamodb.PutItemInput
	mock := &mockDynamoDBAPI{
		putItemFn: func(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			capturedInput = params
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	deadline := time.Now().Add(4 * time.Hour).Format(time.RFC3339Nano)
	session := &Session{
		SessionID:     "task-abc",
		Operator:      "slopezma",
		SignerARN:     "arn:aws:sts::123:assumed-role/sre-role/slopezma",
		TargetCluster: "mc01",
		Region:        "us-east-1",
		Status:        SessionStatusCreating,
		CreatedAt:     time.Now().Format(time.RFC3339Nano),
		Deadline:      deadline,
	}

	err := s.Put(context.Background(), session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("PutItem was not called")
	}
	if *capturedInput.TableName != "test-sessions" {
		t.Errorf("expected table 'test-sessions', got %q", *capturedInput.TableName)
	}
	if session.TTL == 0 {
		t.Error("expected TTL to be set")
	}
	wantKey := SessionTargetStatusKey(SessionStatusCreating, deadline)
	if session.TargetStatusKey != wantKey {
		t.Errorf("expected TargetStatusKey %q, got %q", wantKey, session.TargetStatusKey)
	}
	if v, ok := capturedInput.Item["targetStatusKey"]; !ok {
		t.Fatal("expected targetStatusKey on DynamoDB item")
	} else if s, ok := v.(*types.AttributeValueMemberS); !ok || s.Value != wantKey {
		t.Errorf("targetStatusKey value mismatch: %+v", v)
	}
	if _, ok := capturedInput.Item["operatorARN"]; ok {
		t.Error("expected signerARN attribute, not legacy operatorARN")
	}
	if v, ok := capturedInput.Item["signerARN"]; !ok {
		t.Fatal("expected signerARN on DynamoDB item")
	} else if s, ok := v.(*types.AttributeValueMemberS); !ok || s.Value != session.SignerARN {
		t.Errorf("signerARN value mismatch: %+v", v)
	}
}

func TestDynamoDBSessionStore_Get_WhenItemExists_ItShouldReturnSession(t *testing.T) {
	item, _ := attributevalue.MarshalMap(&Session{
		SessionID:     "task-abc",
		Operator:      "slopezma",
		TargetCluster: "mc01",
		Status:        SessionStatusActive,
	})

	mock := &mockDynamoDBAPI{
		getItemFn: func(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			key := params.Key["sessionId"].(*types.AttributeValueMemberS)
			if key.Value != "task-abc" {
				t.Errorf("expected key 'task-abc', got %q", key.Value)
			}
			return &dynamodb.GetItemOutput{Item: item}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	got, err := s.Get(context.Background(), "task-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected session, got nil")
		return
	}
	if got.Operator != "slopezma" {
		t.Errorf("expected operator 'slopezma', got %q", got.Operator)
	}
	if got.Status != SessionStatusActive {
		t.Errorf("expected status 'active', got %q", got.Status)
	}
}

func TestDynamoDBSessionStore_Get_WhenItemNotFound_ItShouldReturnNil(t *testing.T) {
	mock := &mockDynamoDBAPI{
		getItemFn: func(_ context.Context, _ *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: nil}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	got, err := s.Get(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestDynamoDBSessionStore_UpdateStatus_WhenActivating_ItShouldSetTargetStatusKey(t *testing.T) {
	var capturedInput *dynamodb.UpdateItemInput
	mock := &mockDynamoDBAPI{
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			capturedInput = params
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}

	deadline := time.Now().Add(2 * time.Hour).Format(time.RFC3339Nano)
	s := NewSessionStore(mock, "test-sessions", 30)
	err := s.UpdateStatus(context.Background(), "task-abc", SessionStatusCreating, SessionStatusActive,
		map[string]interface{}{"deadline": deadline})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("UpdateItem was not called")
	}
	if !updateSetsAttribute(capturedInput, "targetStatusKey") {
		t.Errorf("expected targetStatusKey in update, got %v", capturedInput.UpdateExpression)
	}
	want := SessionTargetStatusKey(SessionStatusActive, deadline)
	if !updateHasStringValue(capturedInput, want) {
		t.Errorf("expected targetStatusKey value %q in expression values", want)
	}
}

func TestDynamoDBSessionStore_UpdateStatus_WhenActiveWithoutDeadline_ItShouldError(t *testing.T) {
	s := NewSessionStore(&mockDynamoDBAPI{}, "test-sessions", 30)
	err := s.UpdateStatus(context.Background(), "task-abc", SessionStatusCreating, SessionStatusActive, nil)
	if err == nil {
		t.Fatal("expected error when activating without deadline in updates")
	}
}

func TestDynamoDBSessionStore_UpdateStatus_WhenTerminated_ItShouldSetTerminatedAt(t *testing.T) {
	var capturedInput *dynamodb.UpdateItemInput
	mock := &mockDynamoDBAPI{
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			capturedInput = params
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	err := s.UpdateStatus(context.Background(), "task-abc", SessionStatusActive, SessionStatusTerminated,
		map[string]interface{}{"stopReason": StopReasonOperatorStop})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("UpdateItem was not called")
	}
	if !updateSetsAttribute(capturedInput, "targetStatusKey") {
		t.Error("expected targetStatusKey update for terminated session")
	}
	if !updateHasStringValue(capturedInput, "terminated#") {
		t.Error("expected terminated# prefix in targetStatusKey value")
	}
}

func updateSetsAttribute(input *dynamodb.UpdateItemInput, attr string) bool {
	for _, name := range input.ExpressionAttributeNames {
		if name == attr {
			return true
		}
	}
	return strings.Contains(awsString(input.UpdateExpression), attr)
}

func updateHasStringValue(input *dynamodb.UpdateItemInput, substr string) bool {
	for _, v := range input.ExpressionAttributeValues {
		if sv, ok := v.(*types.AttributeValueMemberS); ok && strings.Contains(sv.Value, substr) {
			return true
		}
	}
	return false
}

func awsString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func TestDynamoDBSessionStore_ListExpired_WhenExpiredSessionsExist_ItShouldUseTargetStatusIndex(t *testing.T) {
	expired := &Session{
		SessionID:     "task-expired",
		Operator:      "slopezma",
		Status:        SessionStatusActive,
		Deadline:      time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
		TargetCluster: "mc01",
	}
	item, _ := attributevalue.MarshalMap(expired)

	queryCalls := 0
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			queryCalls++
			if params.IndexName == nil || *params.IndexName != "target-status-index" {
				t.Errorf("expected target-status-index GSI, got %v", params.IndexName)
			}
			if params.KeyConditionExpression == nil {
				t.Error("expected key condition expression for GSI query")
			}
			// ListExpired queries creating then active; only active partition has a hit in this test.
			if queryCalls == 1 {
				return &dynamodb.QueryOutput{}, nil
			}
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{item}}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	sessions, err := s.ListExpired(context.Background(), "mc01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if queryCalls != 2 {
		t.Fatalf("expected 2 GSI queries (creating+active), got %d", queryCalls)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 expired session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "task-expired" {
		t.Errorf("expected session 'task-expired', got %q", sessions[0].SessionID)
	}
}

func TestDynamoDBSessionStore_ListExpired_WhenTargetClusterEmpty_ItShouldError(t *testing.T) {
	s := NewSessionStore(&mockDynamoDBAPI{}, "test-sessions", 30)
	_, err := s.ListExpired(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty targetCluster")
	}
}

func TestDynamoDBSessionStore_ListActiveBeforeDeadline_WhenTargetClusterEmpty_ItShouldError(t *testing.T) {
	s := NewSessionStore(&mockDynamoDBAPI{}, "test-sessions", 30)
	_, err := s.ListActiveBeforeDeadline(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty targetCluster")
	}
}

func TestDynamoDBSessionStore_ListActiveBeforeDeadline_WhenSessionsExist_ItShouldUseTargetStatusIndex(t *testing.T) {
	active := &Session{
		SessionID:     "task-active",
		TargetCluster: "mc01",
		Status:        SessionStatusActive,
		Deadline:      time.Now().Add(2 * time.Hour).Format(time.RFC3339Nano),
	}
	item, _ := attributevalue.MarshalMap(active)

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName == nil || *params.IndexName != "target-status-index" {
				t.Errorf("expected target-status-index GSI, got %v", params.IndexName)
			}
			if params.FilterExpression != nil {
				t.Errorf("targetStatusKey is a key attribute and cannot be filtered, got %q", *params.FilterExpression)
			}
			if params.KeyConditionExpression == nil || !strings.Contains(*params.KeyConditionExpression, "BETWEEN") {
				t.Errorf("expected BETWEEN on the sort key, got %v", params.KeyConditionExpression)
			}
			var sawCluster, sawActiveLower, sawCeiling bool
			for _, v := range params.ExpressionAttributeValues {
				s, ok := v.(*types.AttributeValueMemberS)
				if !ok {
					continue
				}
				switch {
				case s.Value == "mc01":
					sawCluster = true
				case s.Value == activeStatusKeyCeiling:
					sawCeiling = true
				case strings.HasPrefix(s.Value, "active#") && strings.HasSuffix(s.Value, "\uffff"):
					sawActiveLower = true
				}
			}
			if !sawCluster || !sawActiveLower || !sawCeiling {
				t.Errorf("key condition values = %#v", params.ExpressionAttributeValues)
			}
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{item}}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	sessions, err := s.ListActiveBeforeDeadline(context.Background(), "mc01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "task-active" {
		t.Fatalf("unexpected sessions: %+v", sessions)
	}
}

func TestDynamoDBSessionStore_GetByTaskID_WhenFound_ItShouldUseTaskIDIndex(t *testing.T) {
	want := &Session{
		SessionID: "sess-uuid",
		TaskID:    testECSTaskID,
		Operator:  "slopezma",
	}
	item, _ := attributevalue.MarshalMap(want)

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName == nil || *params.IndexName != "task-id-index" {
				t.Errorf("expected task-id-index GSI, got %v", params.IndexName)
			}
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{item}}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	got, err := s.GetByTaskID(context.Background(), testECSTaskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Operator != "slopezma" || got.SessionID != "sess-uuid" {
		t.Fatalf("unexpected session: %+v", got)
	}
}

func TestDynamoDBSessionStore_GetByTaskID_WhenMissing_ItShouldReturnNil(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}
	s := NewSessionStore(mock, "test-sessions", 30)
	got, err := s.GetByTaskID(context.Background(), testECSTaskID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil session, got %+v", got)
	}
}

const testECSTaskID = "6e8699e3938a4bcd1234567890abcdef"

func TestSessionStatus_WhenValues_ItShouldMatchExpected(t *testing.T) {
	if string(SessionStatusCreating) != "creating" {
		t.Errorf("expected 'creating', got %q", SessionStatusCreating)
	}
	if string(SessionStatusActive) != "active" {
		t.Errorf("expected 'active', got %q", SessionStatusActive)
	}
	if string(SessionStatusTerminated) != "terminated" {
		t.Errorf("expected 'terminated', got %q", SessionStatusTerminated)
	}
	if string(SessionStatusFailed) != "failed" {
		t.Errorf("expected 'failed', got %q", SessionStatusFailed)
	}
}

func TestDynamoDBSessionStore_RecordExecSession_WhenSuccess_ItShouldEscapeOperatorReservedKeyword(t *testing.T) {
	var capturedInput *dynamodb.UpdateItemInput
	mock := &mockDynamoDBAPI{
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			capturedInput = params
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	err := s.RecordExecSession(context.Background(), "4033d11e-2b15-4da4-8b20-3d7b558260c3", "slopezma", "ecs-execute-command-vciif3zr6rblyj3fi48gi6fcvu")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("UpdateItem was not called")
	}
	if capturedInput.ConditionExpression == nil {
		t.Fatal("expected ConditionExpression")
	}
	cond := *capturedInput.ConditionExpression
	if cond != "#st = :active AND #operator = :op" {
		t.Errorf("unexpected condition: %q", cond)
	}
	if capturedInput.ExpressionAttributeNames["#operator"] != "operator" {
		t.Errorf("expected #operator -> operator, got %q", capturedInput.ExpressionAttributeNames["#operator"])
	}
}
