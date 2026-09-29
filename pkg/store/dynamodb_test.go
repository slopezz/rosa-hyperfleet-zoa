package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type mockDynamoDBAPI struct {
	putItemFn    func(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	getItemFn    func(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	queryFn      func(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	updateItemFn func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	scanFn       func(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
}

func (m *mockDynamoDBAPI) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if m.putItemFn != nil {
		return m.putItemFn(ctx, params, optFns...)
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (m *mockDynamoDBAPI) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if m.getItemFn != nil {
		return m.getItemFn(ctx, params, optFns...)
	}
	return &dynamodb.GetItemOutput{}, nil
}

func (m *mockDynamoDBAPI) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, params, optFns...)
	}
	return &dynamodb.QueryOutput{}, nil
}

func (m *mockDynamoDBAPI) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	if m.updateItemFn != nil {
		return m.updateItemFn(ctx, params, optFns...)
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

func (m *mockDynamoDBAPI) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	if m.scanFn != nil {
		return m.scanFn(ctx, params, optFns...)
	}
	return &dynamodb.ScanOutput{}, nil
}

// --- ExecutionStore tests using mocked DynamoDB ---

func TestDynamoDBExecutionStore_Create_WhenSuccess_ItShouldSetTTLAndTargetStatusKey(t *testing.T) {
	var capturedInput *dynamodb.PutItemInput
	mock := &mockDynamoDBAPI{
		putItemFn: func(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			capturedInput = params
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	exec := &Execution{
		ID:        "exec-1",
		Action:    "get_pods",
		AccountID: "123456",
		Status:    StatusDispatched,
		CreatedAt: time.Now().Format(time.RFC3339Nano),
	}

	err := s.Create(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedInput == nil {
		t.Fatal("PutItem was not called")
	}
	if *capturedInput.TableName != "test-table" {
		t.Errorf("expected table 'test-table', got %q", *capturedInput.TableName)
	}
	if exec.TTL == 0 {
		t.Error("expected TTL to be set")
	}
	if exec.TargetStatusKey == "" {
		t.Error("expected TargetStatusKey to be set")
	}
	expectedPrefix := string(StatusDispatched) + "#"
	if len(exec.TargetStatusKey) < len(expectedPrefix) {
		t.Errorf("TargetStatusKey %q doesn't start with %q", exec.TargetStatusKey, expectedPrefix)
	}
}

func TestDynamoDBExecutionStore_Create_WhenConflict_ItShouldReturnError(t *testing.T) {
	mock := &mockDynamoDBAPI{
		putItemFn: func(_ context.Context, _ *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			return nil, &types.ConditionalCheckFailedException{Message: ptrStr("The conditional request failed")}
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	exec := &Execution{ID: "dup-1", Status: StatusDispatched, CreatedAt: time.Now().Format(time.RFC3339Nano)}

	err := s.Create(context.Background(), exec)
	if err == nil {
		t.Fatal("expected error for conditional check failure")
	}
}

func TestDynamoDBExecutionStore_Get_WhenItemExists_ItShouldReturnExecution(t *testing.T) {
	item, _ := attributevalue.MarshalMap(&Execution{
		ID:        "exec-get-1",
		Action:    "get_pods",
		Status:    StatusSucceeded,
		CreatedAt: "2024-06-15T10:00:00Z",
	})

	mock := &mockDynamoDBAPI{
		getItemFn: func(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			key := params.Key["executionId"].(*types.AttributeValueMemberS)
			if key.Value != "exec-get-1" {
				t.Errorf("expected key 'exec-get-1', got %q", key.Value)
			}
			return &dynamodb.GetItemOutput{Item: item}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	exec, err := s.Get(context.Background(), "exec-get-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec == nil {
		t.Fatal("expected execution, got nil")
		return
	}
	if exec.ID != "exec-get-1" {
		t.Errorf("expected ID 'exec-get-1', got %q", exec.ID)
	}
	if exec.Status != StatusSucceeded {
		t.Errorf("expected status 'succeeded', got %q", exec.Status)
	}
}

func TestDynamoDBExecutionStore_Get_WhenItemNotFound_ItShouldReturnNil(t *testing.T) {
	mock := &mockDynamoDBAPI{
		getItemFn: func(_ context.Context, _ *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	exec, err := s.Get(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec != nil {
		t.Fatalf("expected nil, got %+v", exec)
	}
}

func TestDynamoDBExecutionStore_TransitionStatus_WhenSuccess_ItShouldCallUpdateItem(t *testing.T) {
	var capturedInput *dynamodb.UpdateItemInput
	mock := &mockDynamoDBAPI{
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			capturedInput = params
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	err := s.TransitionStatus(context.Background(), "exec-t1", StatusDispatched, StatusSucceeded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("UpdateItem was not called")
	}
	if *capturedInput.TableName != "test-table" {
		t.Errorf("expected table 'test-table', got %q", *capturedInput.TableName)
	}
	key := capturedInput.Key["executionId"].(*types.AttributeValueMemberS)
	if key.Value != "exec-t1" {
		t.Errorf("expected key 'exec-t1', got %q", key.Value)
	}
}

func TestDynamoDBExecutionStore_TransitionStatus_WhenConditionFails_ItShouldReturnError(t *testing.T) {
	mock := &mockDynamoDBAPI{
		updateItemFn: func(_ context.Context, _ *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			return nil, &types.ConditionalCheckFailedException{Message: ptrStr("condition not met")}
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	err := s.TransitionStatus(context.Background(), "exec-t2", StatusDispatched, StatusSucceeded)
	if err == nil {
		t.Fatal("expected error for conditional check failure")
	}
}

func TestDynamoDBExecutionStore_QueryByStatus_WhenItemsExist_ItShouldReturnAll(t *testing.T) {
	items := []map[string]types.AttributeValue{}
	for _, exec := range []*Execution{
		{ID: "e1", Status: StatusDispatched, CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: "e2", Status: StatusDispatched, CreatedAt: "2024-01-01T00:01:00Z"},
	} {
		item, _ := attributevalue.MarshalMap(exec)
		items = append(items, item)
	}

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{Items: items, LastEvaluatedKey: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	results, err := s.QueryByStatus(context.Background(), StatusDispatched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_MarkCleaned_WhenSuccess_ItShouldCallUpdateItem(t *testing.T) {
	called := false
	mock := &mockDynamoDBAPI{
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			called = true
			key := params.Key["executionId"].(*types.AttributeValueMemberS)
			if key.Value != "exec-clean" {
				t.Errorf("expected key 'exec-clean', got %q", key.Value)
			}
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	err := s.MarkCleaned(context.Background(), "exec-clean")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("UpdateItem was not called")
	}
}

// --- AuditStore tests using mocked DynamoDB ---

func TestDynamoDBAuditStore_Record_WhenSuccess_ItShouldSetTTL(t *testing.T) {
	var capturedInput *dynamodb.PutItemInput
	mock := &mockDynamoDBAPI{
		putItemFn: func(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			capturedInput = params
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	entry := &AuditEntry{
		AccountID:  "123456",
		Timestamp:  time.Now().Format(time.RFC3339Nano),
		Method:     "POST",
		Path:       "/api/v0/trusted-actions/get_pods/run",
		StatusCode: 200,
		Operator:   "arn:aws:iam::123456:user/test",
	}

	err := s.Record(context.Background(), entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("PutItem was not called")
	}
	if *capturedInput.TableName != "audit-table" {
		t.Errorf("expected table 'audit-table', got %q", *capturedInput.TableName)
	}
	if entry.TTL == 0 {
		t.Error("expected TTL to be set")
	}
}

func TestDynamoDBAuditStore_List_WhenItemsExist_ItShouldReturnEntries(t *testing.T) {
	items := []map[string]types.AttributeValue{}
	for _, entry := range []*AuditEntry{
		{AccountID: "123", Timestamp: "2024-06-01T10:00:00Z", Method: "GET", StatusCode: 200},
		{AccountID: "123", Timestamp: "2024-06-01T10:01:00Z", Method: "POST", StatusCode: 200},
	} {
		item, _ := attributevalue.MarshalMap(entry)
		items = append(items, item)
	}

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	results, err := s.List(context.Background(), "123", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 entries, got %d", len(results))
	}
}

func TestDynamoDBAuditStore_List_WhenFilterApplied_ItShouldRespectLimit(t *testing.T) {
	items := make([]map[string]types.AttributeValue, 0, 10)
	for i := 0; i < 10; i++ {
		entry := &AuditEntry{
			AccountID: "123",
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339Nano),
			Method:    "POST",
			Path:      fmt.Sprintf("/api/v0/trusted-actions/action%d/run", i),
		}
		item, _ := attributevalue.MarshalMap(entry)
		items = append(items, item)
	}

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	since := time.Now().Add(-24 * time.Hour)
	filter := &AuditFilter{
		Since: &since,
		Limit: 5,
	}
	results, err := s.List(context.Background(), "123", filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5 results (limit=5), got %d", len(results))
	}
}

// --- Additional ExecutionStore query tests ---

func TestDynamoDBExecutionStore_List_WhenItemsExist_ItShouldReturnAll(t *testing.T) {
	items := []map[string]types.AttributeValue{}
	for _, exec := range []*Execution{
		{ID: "e1", AccountID: "acc1", Status: StatusSucceeded, CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: "e2", AccountID: "acc1", Status: StatusFailed, CreatedAt: "2024-01-01T01:00:00Z"},
	} {
		item, _ := attributevalue.MarshalMap(exec)
		items = append(items, item)
	}

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "account-index" {
				t.Errorf("expected account-index, got %q", *params.IndexName)
			}
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	results, err := s.List(context.Background(), "acc1", 50, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_List_WhenQueryFails_ItShouldReturnError(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return nil, fmt.Errorf("dynamodb unavailable")
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	_, err := s.List(context.Background(), "acc1", 50, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDynamoDBExecutionStore_TransitionWithMetadata_WhenSuccess_ItShouldIncludeExtraFields(t *testing.T) {
	var capturedInput *dynamodb.UpdateItemInput
	mock := &mockDynamoDBAPI{
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			capturedInput = params
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	err := s.TransitionWithMetadata(context.Background(), "exec-1", StatusDispatched, StatusSucceeded, map[string]interface{}{
		"summary": "completed successfully",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("UpdateItem was not called")
	}
	key := capturedInput.Key["executionId"].(*types.AttributeValueMemberS)
	if key.Value != "exec-1" {
		t.Errorf("expected key 'exec-1', got %q", key.Value)
	}
}

func TestDynamoDBExecutionStore_QueryByStatusAndClass_WhenSuccess_ItShouldFilterByClass(t *testing.T) {
	items := []map[string]types.AttributeValue{}
	exec := &Execution{ID: "e1", Status: StatusDispatched, ExecutionMode: "sync", CreatedAt: "2024-01-01T00:00:00Z"}
	item, _ := attributevalue.MarshalMap(exec)
	items = append(items, item)

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.FilterExpression == nil {
				t.Error("expected filter expression for class filter")
			}
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	results, err := s.QueryByStatusAndClass(context.Background(), StatusDispatched, "sync")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_QueryTerminal_WhenSuccess_ItShouldQueryAllTerminalStatuses(t *testing.T) {
	queryCalls := 0
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			queryCalls++
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	results, err := s.QueryTerminal(context.Background(), 1*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	// Should query for succeeded, failed, and timedOut
	if queryCalls != 3 {
		t.Errorf("expected 3 query calls (one per terminal status), got %d", queryCalls)
	}
}

func TestDynamoDBExecutionStore_ListByTargetAndAction_WhenSuccess_ItShouldUseDateBucketIndex(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "date-bucket-index" {
				t.Errorf("expected date-bucket-index, got %q", *params.IndexName)
			}
			if params.FilterExpression == nil {
				t.Error("expected FilterExpression containing targetCluster and action")
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	_, err := s.ListByTargetAndAction(context.Background(), "cluster-1", "delete_pod", time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamoDBExecutionStore_CountActiveByTarget_WhenItemsExist_ItShouldReturnCount(t *testing.T) {
	var queryCount int
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			queryCount++
			if params.Select != types.SelectCount {
				t.Error("expected SelectCount")
			}
			if *params.IndexName != "target-status-index" {
				t.Errorf("expected target-status-index, got %q", *params.IndexName)
			}
			return &dynamodb.QueryOutput{Count: 3}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	count, err := s.CountActiveByTarget(context.Background(), "cluster-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if queryCount != 2 {
		t.Errorf("expected 2 queries (dispatched + approved), got %d", queryCount)
	}
	if count != 6 {
		t.Errorf("expected count=6 (3+3), got %d", count)
	}
}

func TestDynamoDBExecutionStore_QueryByTargetAndStatus_WhenSuccess_ItShouldUseTargetStatusIndex(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "target-status-index" {
				t.Errorf("expected target-status-index, got %q", *params.IndexName)
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	_, err := s.QueryByTargetAndStatus(context.Background(), "cluster-1", StatusDispatched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamoDBExecutionStore_QueryTerminalByTarget_WhenSuccess_ItShouldFilterOldUncleaned(t *testing.T) {
	oldTime := time.Now().Add(-2 * time.Hour).Format(time.RFC3339Nano)

	// Only return items for "succeeded" status queries (simulating that
	// only one terminal status has a matching execution)
	succeededItems := []map[string]types.AttributeValue{}
	exec := &Execution{ID: "e1", Status: StatusSucceeded, CreatedAt: oldTime, Cleaned: false, TargetCluster: "cluster-1"}
	item, _ := attributevalue.MarshalMap(exec)
	succeededItems = append(succeededItems, item)

	callCount := 0
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			callCount++
			// First call is for "succeeded" status
			if callCount == 1 {
				return &dynamodb.QueryOutput{Items: succeededItems}, nil
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	results, err := s.QueryTerminalByTarget(context.Background(), "cluster-1", 1*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

// --- Performance and GSI routing tests ---

func TestDynamoDBExecutionStore_List_WhenStatusFilter_ItShouldUseStatusIndex(t *testing.T) {
	items := []map[string]types.AttributeValue{}
	exec := &Execution{ID: "e1", AccountID: "acc1", Status: StatusFailed, CreatedAt: "2024-01-01T00:00:00Z"}
	item, _ := attributevalue.MarshalMap(exec)
	items = append(items, item)

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "status-index" {
				t.Errorf("expected status-index GSI for status filter, got %q", *params.IndexName)
			}
			if params.FilterExpression == nil {
				t.Error("expected filter expression containing accountId")
			}
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	status := StatusFailed
	filter := &ListFilter{Status: &status}
	results, err := s.List(context.Background(), "acc1", 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_List_WhenNoStatusFilter_ItShouldUseAccountIndex(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "account-index" {
				t.Errorf("expected account-index GSI for non-status filter, got %q", *params.IndexName)
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	mode := "async"
	filter := &ListFilter{ExecutionMode: &mode}
	_, err := s.List(context.Background(), "acc1", 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamoDBExecutionStore_List_WhenMaxPagesReached_ItShouldStopPaginating(t *testing.T) {
	pageCount := 0
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			pageCount++
			item, _ := attributevalue.MarshalMap(&Execution{
				ID: fmt.Sprintf("e%d", pageCount), AccountID: "acc1", Status: StatusSucceeded,
				CreatedAt: time.Now().Add(-time.Duration(pageCount) * time.Minute).Format(time.RFC3339Nano),
			})
			return &dynamodb.QueryOutput{
				Items:            []map[string]types.AttributeValue{item},
				LastEvaluatedKey: map[string]types.AttributeValue{"executionId": &types.AttributeValueMemberS{Value: fmt.Sprintf("e%d", pageCount)}},
			}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	results, err := s.List(context.Background(), "acc1", 1000, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageCount != 10 {
		t.Errorf("expected max 10 pages, got %d", pageCount)
	}
	if len(results) != 10 {
		t.Errorf("expected 10 results (1 per page, 10 pages max), got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_List_WhenLimitReached_ItShouldStopEarly(t *testing.T) {
	items := make([]map[string]types.AttributeValue, 0, 20)
	for i := 0; i < 20; i++ {
		exec := &Execution{
			ID: fmt.Sprintf("e%d", i), AccountID: "acc1", Status: StatusSucceeded,
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339Nano),
		}
		item, _ := attributevalue.MarshalMap(exec)
		items = append(items, item)
	}

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	filter := &ListFilter{Limit: 5}
	results, err := s.List(context.Background(), "acc1", 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5 results (limit=5), got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_List_WhenStatusAndSinceFilter_ItShouldUseStatusIndexWithTimeRange(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "status-index" {
				t.Errorf("expected status-index, got %q", *params.IndexName)
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	status := StatusFailed
	since := time.Now().Add(-1 * time.Hour)
	filter := &ListFilter{Status: &status, Since: &since}
	_, err := s.List(context.Background(), "acc1", 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamoDBAuditStore_List_WhenMaxPagesReached_ItShouldStopPaginating(t *testing.T) {
	pageCount := 0
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			pageCount++
			entry := &AuditEntry{
				AccountID: "123",
				Timestamp: time.Now().Add(-time.Duration(pageCount) * time.Minute).Format(time.RFC3339Nano),
				Method:    "GET",
			}
			item, _ := attributevalue.MarshalMap(entry)
			return &dynamodb.QueryOutput{
				Items:            []map[string]types.AttributeValue{item},
				LastEvaluatedKey: map[string]types.AttributeValue{"accountId": &types.AttributeValueMemberS{Value: "123"}},
			}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	filter := &AuditFilter{Limit: 1000}
	results, err := s.List(context.Background(), "123", filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageCount != 10 {
		t.Errorf("expected max 10 pages, got %d", pageCount)
	}
	if len(results) != 10 {
		t.Errorf("expected 10 results (1 per page, 10 pages max), got %d", len(results))
	}
}

func TestDynamoDBAuditStore_List_WhenBeforeFilter_ItShouldUseKeyCondition(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.KeyConditionExpression == nil {
				t.Error("expected key condition expression")
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	before := time.Now()
	since := time.Now().Add(-1 * time.Hour)
	filter := &AuditFilter{Since: &since, Before: &before}
	_, err := s.List(context.Background(), "123", filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamoDBAuditStore_List_WhenMethodFilter_ItShouldApplyFilterExpression(t *testing.T) {
	items := []map[string]types.AttributeValue{}
	entry := &AuditEntry{AccountID: "123", Timestamp: "2024-01-01T00:00:00Z", Method: "POST"}
	item, _ := attributevalue.MarshalMap(entry)
	items = append(items, item)

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.FilterExpression == nil {
				t.Error("expected filter expression for method filter")
			}
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	method := "POST"
	filter := &AuditFilter{Method: &method}
	results, err := s.List(context.Background(), "123", filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

// --- ListAll tests ---

func TestDynamoDBExecutionStore_ListAll_WhenNoTargetOrStatus_ItShouldQueryDateBucketIndex(t *testing.T) {
	queryCalled := false
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			queryCalled = true
			if params.IndexName == nil || *params.IndexName != "date-bucket-index" {
				t.Errorf("expected date-bucket-index GSI, got %v", params.IndexName)
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	since := time.Now().Add(-24 * time.Hour)
	filter := &ListFilter{Since: &since}
	_, err := s.ListAll(context.Background(), 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !queryCalled {
		t.Error("expected Query on date-bucket-index for ListAll without target/status")
	}
}

func TestDynamoDBExecutionStore_ListAll_WhenSinceSpansMultipleDays_ItShouldQueryMultipleBuckets(t *testing.T) {
	bucketsQueried := make(map[string]bool)
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName != nil && *params.IndexName == "date-bucket-index" {
				for _, v := range params.ExpressionAttributeValues {
					if s, ok := v.(*types.AttributeValueMemberS); ok {
						if len(s.Value) == 10 {
							bucketsQueried[s.Value] = true
						}
					}
				}
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	since := time.Now().Add(-3 * 24 * time.Hour)
	filter := &ListFilter{Since: &since}
	_, err := s.ListAll(context.Background(), 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bucketsQueried) < 3 {
		t.Errorf("expected at least 3 day-buckets queried for --since 3d, got %d", len(bucketsQueried))
	}
}

func TestDynamoDBExecutionStore_ListAll_WhenBucketEmpty_ItShouldContinueToPreviousBucket(t *testing.T) {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	callCount := 0
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			callCount++
			for _, v := range params.ExpressionAttributeValues {
				if s, ok := v.(*types.AttributeValueMemberS); ok && s.Value == today {
					return &dynamodb.QueryOutput{}, nil
				}
			}
			for _, v := range params.ExpressionAttributeValues {
				if s, ok := v.(*types.AttributeValueMemberS); ok && s.Value == yesterday {
					item, _ := attributevalue.MarshalMap(&Execution{
						ID:        "exec-yesterday",
						AccountID: "111",
						Status:    StatusSucceeded,
						CreatedAt: now.AddDate(0, 0, -1).Format(time.RFC3339Nano),
					})
					return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{item}}, nil
				}
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	since := now.Add(-48 * time.Hour)
	filter := &ListFilter{Since: &since}
	results, err := s.ListAll(context.Background(), 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result from yesterday's bucket, got %d", len(results))
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 Query calls (today empty + yesterday), got %d", callCount)
	}
}

func TestDynamoDBExecutionStore_ListAll_WhenStatusProvided_ItShouldQueryDateBucketIndexWithStatusFilter(t *testing.T) {
	queryCalled := false
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			queryCalled = true
			if params.IndexName == nil || *params.IndexName != "date-bucket-index" {
				t.Errorf("expected date-bucket-index GSI, got %v", params.IndexName)
			}
			if params.FilterExpression == nil {
				t.Error("expected FilterExpression containing status filter")
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	status := StatusFailed
	since := time.Now().Add(-24 * time.Hour)
	filter := &ListFilter{Status: &status, Since: &since}
	_, err := s.ListAll(context.Background(), 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !queryCalled {
		t.Error("expected Query to be called for ListAll with status")
	}
}

func TestDynamoDBExecutionStore_ListAll_WhenTargetProvided_ItShouldQueryDateBucketIndexWithTargetFilter(t *testing.T) {
	queryCalled := false
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			queryCalled = true
			if params.IndexName == nil || *params.IndexName != "date-bucket-index" {
				t.Errorf("expected date-bucket-index GSI, got %v", params.IndexName)
			}
			if params.FilterExpression == nil {
				t.Error("expected FilterExpression containing targetCluster filter")
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	target := "eph-dev-mc01"
	since := time.Now().Add(-24 * time.Hour)
	filter := &ListFilter{Target: &target, Since: &since}
	_, err := s.ListAll(context.Background(), 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !queryCalled {
		t.Error("expected Query to be called for ListAll with target")
	}
}

func TestDynamoDBExecutionStore_ListAll_WhenTargetAndStatusProvided_ItShouldQueryDateBucketIndexWithBothFilters(t *testing.T) {
	queryCalled := false
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			queryCalled = true
			if params.IndexName == nil || *params.IndexName != "date-bucket-index" {
				t.Errorf("expected date-bucket-index GSI, got %v", params.IndexName)
			}
			if params.FilterExpression == nil {
				t.Error("expected FilterExpression containing both targetCluster and status filters")
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	target := "eph-dev-mc01"
	status := StatusDispatched
	since := time.Now().Add(-24 * time.Hour)
	filter := &ListFilter{Target: &target, Status: &status, Since: &since}
	_, err := s.ListAll(context.Background(), 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !queryCalled {
		t.Error("expected Query to be called when both target and status are provided")
	}
}

func TestDynamoDBExecutionStore_ListAll_WhenDateBucketReturnsItems_ItShouldDeserializeThem(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	todayBucket := time.Now().UTC().Format("2006-01-02")
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName != nil && *params.IndexName == "date-bucket-index" {
				for _, v := range params.ExpressionAttributeValues {
					if s, ok := v.(*types.AttributeValueMemberS); ok && s.Value == todayBucket {
						return &dynamodb.QueryOutput{
							Items: []map[string]types.AttributeValue{
								{
									"executionId":   &types.AttributeValueMemberS{Value: "exec-001"},
									"accountId":     &types.AttributeValueMemberS{Value: "111111111111"},
									"targetCluster": &types.AttributeValueMemberS{Value: "mc-alpha"},
									"action":        &types.AttributeValueMemberS{Value: "get_resource"},
									"status":        &types.AttributeValueMemberS{Value: "completed"},
									"createdAt":     &types.AttributeValueMemberS{Value: now},
									"operator":      &types.AttributeValueMemberS{Value: "user@redhat.com"},
									"executionMode": &types.AttributeValueMemberS{Value: "sync"},
									"scope":         &types.AttributeValueMemberS{Value: "kube-api"},
									"type":          &types.AttributeValueMemberS{Value: "read"},
								},
								{
									"executionId":   &types.AttributeValueMemberS{Value: "exec-002"},
									"accountId":     &types.AttributeValueMemberS{Value: "222222222222"},
									"targetCluster": &types.AttributeValueMemberS{Value: "mc-beta"},
									"action":        &types.AttributeValueMemberS{Value: "list_eks_clusters"},
									"status":        &types.AttributeValueMemberS{Value: "failed"},
									"createdAt":     &types.AttributeValueMemberS{Value: now},
									"operator":      &types.AttributeValueMemberS{Value: "admin@redhat.com"},
									"executionMode": &types.AttributeValueMemberS{Value: "sync"},
									"scope":         &types.AttributeValueMemberS{Value: "aws-api"},
									"type":          &types.AttributeValueMemberS{Value: "read"},
								},
							},
						}, nil
					}
				}
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	since := time.Now().Add(-24 * time.Hour)
	filter := &ListFilter{Since: &since}
	results, err := s.ListAll(context.Background(), 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "exec-001" || results[0].TargetCluster != "mc-alpha" {
		t.Errorf("first result mismatch: %+v", results[0])
	}
	if results[1].ID != "exec-002" || results[1].TargetCluster != "mc-beta" {
		t.Errorf("second result mismatch: %+v", results[1])
	}
}

func TestDynamoDBExecutionStore_ListAll_WhenLimitReached_ItShouldTruncateResults(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName != nil && *params.IndexName == "date-bucket-index" {
				items := make([]map[string]types.AttributeValue, 5)
				for i := range items {
					items[i] = map[string]types.AttributeValue{
						"executionId":   &types.AttributeValueMemberS{Value: fmt.Sprintf("exec-%03d", i)},
						"accountId":     &types.AttributeValueMemberS{Value: "111111111111"},
						"targetCluster": &types.AttributeValueMemberS{Value: "mc-alpha"},
						"action":        &types.AttributeValueMemberS{Value: "get_resource"},
						"status":        &types.AttributeValueMemberS{Value: "completed"},
						"createdAt":     &types.AttributeValueMemberS{Value: now.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339Nano)},
						"operator":      &types.AttributeValueMemberS{Value: "user@redhat.com"},
						"executionMode": &types.AttributeValueMemberS{Value: "sync"},
						"scope":         &types.AttributeValueMemberS{Value: "kube-api"},
						"type":          &types.AttributeValueMemberS{Value: "read"},
					}
				}
				return &dynamodb.QueryOutput{Items: items}, nil
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	since := time.Now().Add(-24 * time.Hour)
	filter := &ListFilter{Since: &since}
	results, err := s.ListAll(context.Background(), 3, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected limit of 3 results, got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_ListAll_WhenQueryError_ItShouldReturnError(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName != nil && *params.IndexName == "date-bucket-index" {
				return nil, fmt.Errorf("throttling exception")
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	since := time.Now().Add(-24 * time.Hour)
	filter := &ListFilter{Since: &since}
	_, err := s.ListAll(context.Background(), 50, filter)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDynamoDBExecutionStore_ListAll_WhenStatusWithSince_ItShouldQueryDateBucketIndexWithTimeRange(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName == nil || *params.IndexName != "date-bucket-index" {
				t.Errorf("expected date-bucket-index, got %v", params.IndexName)
			}
			if params.KeyConditionExpression == nil {
				t.Error("expected key condition expression")
			}
			if params.FilterExpression == nil {
				t.Error("expected FilterExpression containing status filter")
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	status := StatusDispatched
	since := time.Now().Add(-1 * time.Hour)
	filter := &ListFilter{Status: &status, Since: &since}
	_, err := s.ListAll(context.Background(), 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamoDBExecutionStore_ListAll_WhenSinceAndBefore_ItShouldUseBetweenKeyCondition(t *testing.T) {
	usedBetween := false
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName != nil && *params.IndexName == "date-bucket-index" {
				if params.KeyConditionExpression != nil {
					expr := *params.KeyConditionExpression
					hasBetween := false
					for _, v := range params.ExpressionAttributeValues {
						_ = v
					}
					if len(expr) > 0 {
						hasBetween = len(params.ExpressionAttributeValues) >= 3
					}
					if hasBetween {
						usedBetween = true
					}
				}
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	since := time.Now().Add(-24 * time.Hour)
	before := time.Now().Add(-1 * time.Hour)
	filter := &ListFilter{Since: &since, Before: &before}
	_, err := s.ListAll(context.Background(), 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !usedBetween {
		t.Error("expected Between key condition when both Since and Before are set (need at least 3 expression attribute values: dateBucket + sinceStr + beforeStr)")
	}
}

func TestDynamoDBExecutionStore_ListAll_WhenBeforeSet_ItShouldStartFromBeforeDay(t *testing.T) {
	now := time.Now().UTC()
	before := now.Add(-48 * time.Hour)
	beforeDay := before.Truncate(24 * time.Hour).Format("2006-01-02")

	bucketsQueried := map[string]bool{}
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName != nil && *params.IndexName == "date-bucket-index" {
				for _, v := range params.ExpressionAttributeValues {
					if s, ok := v.(*types.AttributeValueMemberS); ok {
						if len(s.Value) == 10 {
							bucketsQueried[s.Value] = true
						}
					}
				}
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	since := now.Add(-72 * time.Hour)
	filter := &ListFilter{Since: &since, Before: &before}
	_, err := s.ListAll(context.Background(), 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	todayStr := now.Truncate(24 * time.Hour).Format("2006-01-02")
	if bucketsQueried[todayStr] {
		t.Errorf("should NOT query today's bucket (%s) when --until is 2 days ago", todayStr)
	}
	if !bucketsQueried[beforeDay] {
		t.Errorf("should query the --until day bucket (%s)", beforeDay)
	}
}

func TestDynamoDBAuditStore_ListAll_WhenSinceAndBefore_ItShouldUseBetweenKeyCondition(t *testing.T) {
	usedBetween := false
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName != nil && *params.IndexName == "date-bucket-index" {
				if params.KeyConditionExpression != nil {
					if len(params.ExpressionAttributeValues) >= 3 {
						usedBetween = true
					}
				}
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	store := NewAuditStore(mock, "audit-table", 365)
	since := time.Now().Add(-24 * time.Hour)
	before := time.Now().Add(-1 * time.Hour)
	filter := &AuditFilter{Since: &since, Before: &before}
	_, err := store.ListAll(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !usedBetween {
		t.Error("expected Between key condition when both Since and Before are set for audit")
	}
}

func TestDynamoDBExecutionStore_ListAll_WhenFilterWithAction_ItShouldApplyFilterExpression(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName != nil && *params.IndexName == "date-bucket-index" {
				if params.FilterExpression == nil {
					t.Error("expected filter expression for action filter on date-bucket-index")
				}
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "executions-table", 30)
	action := "get_resource"
	since := time.Now().Add(-24 * time.Hour)
	filter := &ListFilter{Action: &action, Since: &since}
	_, err := s.ListAll(context.Background(), 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamoDBAuditStore_ListAll_WhenNoTarget_ItShouldQueryDateBucketIndex(t *testing.T) {
	queryCalled := false
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName != nil && *params.IndexName == "date-bucket-index" {
				queryCalled = true
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	since := time.Now().Add(-24 * time.Hour)
	filter := &AuditFilter{Since: &since}
	_, err := s.ListAll(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !queryCalled {
		t.Error("expected Query on date-bucket-index for ListAll without target")
	}
}

func TestDynamoDBAuditStore_ListAll_WhenTargetProvided_ItShouldQueryDateBucketIndexWithTargetFilter(t *testing.T) {
	queryCalled := false
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			queryCalled = true
			if params.IndexName == nil || *params.IndexName != "date-bucket-index" {
				t.Errorf("expected date-bucket-index GSI, got %v", params.IndexName)
			}
			if params.FilterExpression == nil {
				t.Error("expected FilterExpression containing targetCluster filter")
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	target := "eph-dev-rc"
	since := time.Now().Add(-24 * time.Hour)
	filter := &AuditFilter{Target: &target, Since: &since}
	_, err := s.ListAll(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !queryCalled {
		t.Error("expected Query to be called for ListAll with target")
	}
}

func TestDynamoDBAuditStore_ListAll_WhenDateBucketReturnsItems_ItShouldDeserializeThem(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	todayBucket := time.Now().UTC().Format("2006-01-02")
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName != nil && *params.IndexName == "date-bucket-index" {
				for _, v := range params.ExpressionAttributeValues {
					if s, ok := v.(*types.AttributeValueMemberS); ok && s.Value == todayBucket {
						return &dynamodb.QueryOutput{
							Items: []map[string]types.AttributeValue{
								{
									"accountId":     &types.AttributeValueMemberS{Value: "111111111111"},
									"timestamp":     &types.AttributeValueMemberS{Value: now},
									"method":        &types.AttributeValueMemberS{Value: "POST"},
									"path":          &types.AttributeValueMemberS{Value: "/v1/executions"},
									"statusCode":    &types.AttributeValueMemberN{Value: "200"},
									"operator":      &types.AttributeValueMemberS{Value: "user@redhat.com"},
									"targetCluster": &types.AttributeValueMemberS{Value: "mc-alpha"},
									"action":        &types.AttributeValueMemberS{Value: "get_resource"},
								},
								{
									"accountId":     &types.AttributeValueMemberS{Value: "222222222222"},
									"timestamp":     &types.AttributeValueMemberS{Value: now},
									"method":        &types.AttributeValueMemberS{Value: "GET"},
									"path":          &types.AttributeValueMemberS{Value: "/v1/executions"},
									"statusCode":    &types.AttributeValueMemberN{Value: "200"},
									"operator":      &types.AttributeValueMemberS{Value: "admin@redhat.com"},
									"targetCluster": &types.AttributeValueMemberS{Value: "mc-beta"},
								},
							},
						}, nil
					}
				}
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	since := time.Now().Add(-24 * time.Hour)
	filter := &AuditFilter{Since: &since}
	results, err := s.ListAll(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].AccountID != "111111111111" || results[0].TargetCluster != "mc-alpha" {
		t.Errorf("first audit entry mismatch: %+v", results[0])
	}
	if results[1].AccountID != "222222222222" || results[1].TargetCluster != "mc-beta" {
		t.Errorf("second audit entry mismatch: %+v", results[1])
	}
}

func TestDynamoDBAuditStore_ListAll_WhenQueryError_ItShouldReturnError(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName != nil && *params.IndexName == "date-bucket-index" {
				return nil, fmt.Errorf("service unavailable")
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	since := time.Now().Add(-24 * time.Hour)
	filter := &AuditFilter{Since: &since}
	_, err := s.ListAll(context.Background(), filter)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDynamoDBAuditStore_ListAll_WhenTargetWithFilter_ItShouldQueryDateBucketIndexWithFilterExpression(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.IndexName == nil || *params.IndexName != "date-bucket-index" {
				t.Errorf("expected date-bucket-index, got %v", params.IndexName)
			}
			if params.FilterExpression == nil {
				t.Error("expected FilterExpression containing target and action filters")
			}
			return &dynamodb.QueryOutput{}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	target := "eph-dev-mc01"
	action := "get_resource"
	since := time.Now().Add(-24 * time.Hour)
	filter := &AuditFilter{Target: &target, Action: &action, Since: &since}
	_, err := s.ListAll(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func ptrStr(s string) *string { return &s }
