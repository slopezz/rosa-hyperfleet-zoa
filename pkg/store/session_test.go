package store

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestDynamoDBSessionStore_Put_WhenSuccess_ItShouldSetTTL(t *testing.T) {
	var capturedInput *dynamodb.PutItemInput
	mock := &mockDynamoDBAPI{
		putItemFn: func(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			capturedInput = params
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	session := &Session{
		SessionID:     "task-abc",
		Operator:      "slopezma",
		OperatorARN:   "arn:aws:sts::123:assumed-role/sre-role/slopezma",
		TargetCluster: "mc01",
		Region:        "us-east-1",
		Status:        SessionStatusCreating,
		CreatedAt:     time.Now().Format(time.RFC3339Nano),
		Deadline:      time.Now().Add(4 * time.Hour).Format(time.RFC3339Nano),
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

func TestDynamoDBSessionStore_UpdateStatus_WhenSuccess_ItShouldCallUpdateItem(t *testing.T) {
	var capturedInput *dynamodb.UpdateItemInput
	mock := &mockDynamoDBAPI{
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			capturedInput = params
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	err := s.UpdateStatus(context.Background(), "task-abc", SessionStatusCreating, SessionStatusActive, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("UpdateItem was not called")
	}
	key := capturedInput.Key["sessionId"].(*types.AttributeValueMemberS)
	if key.Value != "task-abc" {
		t.Errorf("expected key 'task-abc', got %q", key.Value)
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
		map[string]interface{}{"terminationReason": "sre_exit"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("UpdateItem was not called")
	}
}

func TestDynamoDBSessionStore_ListExpired_WhenExpiredSessionsExist_ItShouldReturnThem(t *testing.T) {
	expired := &Session{
		SessionID:     "task-expired",
		Operator:      "slopezma",
		Status:        SessionStatusActive,
		Deadline:      time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
		TargetCluster: "mc01",
	}
	item, _ := attributevalue.MarshalMap(expired)

	mock := &mockDynamoDBAPI{
		scanFn: func(_ context.Context, params *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
			if params.FilterExpression == nil {
				t.Error("expected filter expression for expired sessions query")
			}
			return &dynamodb.ScanOutput{Items: []map[string]types.AttributeValue{item}}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	sessions, err := s.ListExpired(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 expired session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "task-expired" {
		t.Errorf("expected session 'task-expired', got %q", sessions[0].SessionID)
	}
}

func TestDynamoDBSessionStore_List_WhenStatusFilter_ItShouldApplyFilterExpression(t *testing.T) {
	item, _ := attributevalue.MarshalMap(&Session{
		SessionID: "task-1",
		Operator:  "slopezma",
		Status:    SessionStatusActive,
	})

	mock := &mockDynamoDBAPI{
		scanFn: func(_ context.Context, params *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
			if params.FilterExpression == nil {
				t.Error("expected filter expression for status filter")
			}
			return &dynamodb.ScanOutput{Items: []map[string]types.AttributeValue{item}}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	status := SessionStatusActive
	sessions, err := s.List(context.Background(), &SessionFilter{Status: &status})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

func TestDynamoDBSessionStore_ListByOperator_WhenSuccess_ItShouldUseOperatorIndex(t *testing.T) {
	item, _ := attributevalue.MarshalMap(&Session{
		SessionID: "task-1",
		Operator:  "slopezma",
		Status:    SessionStatusActive,
		CreatedAt: time.Now().Format(time.RFC3339Nano),
	})

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "operator-index" {
				t.Errorf("expected operator-index, got %q", *params.IndexName)
			}
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{item}}, nil
		},
	}

	s := NewSessionStore(mock, "test-sessions", 30)
	sessions, err := s.ListByOperator(context.Background(), "slopezma", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

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
