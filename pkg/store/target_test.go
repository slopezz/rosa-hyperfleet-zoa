package store

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestDynamoDBTargetStore_Get_WhenItemExists_ItShouldReturnTarget(t *testing.T) {
	item, _ := attributevalue.MarshalMap(&Target{
		TargetID:       "mc01",
		DeploymentName: "us-east-1",
		VpcId:          "vpc-abc123",
		TargetType:     "MC",
		Region:         "us-east-1",
		Status:         "ready",
	})

	mock := &mockDynamoDBAPI{
		getItemFn: func(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			if *params.TableName != "test-targets" {
				t.Errorf("expected table 'test-targets', got %q", *params.TableName)
			}
			key := params.Key["targetId"].(*types.AttributeValueMemberS)
			if key.Value != "mc01" {
				t.Errorf("expected key 'mc01', got %q", key.Value)
			}
			return &dynamodb.GetItemOutput{Item: item}, nil
		},
	}

	s := NewTargetStore(mock, "test-targets")
	got, err := s.Get(context.Background(), "mc01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected target, got nil")
		return
	}
	if got.TargetID != "mc01" {
		t.Errorf("expected targetID 'mc01', got %q", got.TargetID)
	}
	if got.TargetType != "MC" {
		t.Errorf("expected targetType 'MC', got %q", got.TargetType)
	}
}

func TestDynamoDBTargetStore_Get_WhenItemNotFound_ItShouldReturnNil(t *testing.T) {
	mock := &mockDynamoDBAPI{
		getItemFn: func(_ context.Context, _ *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: nil}, nil
		},
	}

	s := NewTargetStore(mock, "test-targets")
	got, err := s.Get(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestDynamoDBTargetStore_List_WhenItemsExist_ItShouldReturnAllTargets(t *testing.T) {
	targets := []Target{
		{TargetID: "rc", TargetType: "RC", DeploymentName: "us-east-1"},
		{TargetID: "mc01", TargetType: "MC", DeploymentName: "us-east-1"},
	}
	items := make([]map[string]types.AttributeValue, 0, len(targets))
	for _, tgt := range targets {
		item, _ := attributevalue.MarshalMap(tgt)
		items = append(items, item)
	}

	mock := &mockDynamoDBAPI{
		scanFn: func(_ context.Context, _ *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
			return &dynamodb.ScanOutput{Items: items}, nil
		},
	}

	s := NewTargetStore(mock, "test-targets")
	got, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(got))
	}
}

func TestDynamoDBTargetStore_ListByDeployment_WhenFilterApplied_ItShouldUseFilterExpression(t *testing.T) {
	item, _ := attributevalue.MarshalMap(&Target{TargetID: "mc01", DeploymentName: "us-east-1", TargetType: "MC"})

	mock := &mockDynamoDBAPI{
		scanFn: func(_ context.Context, params *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
			if params.FilterExpression == nil {
				t.Error("expected filter expression for deployment filter")
			}
			return &dynamodb.ScanOutput{Items: []map[string]types.AttributeValue{item}}, nil
		},
	}

	s := NewTargetStore(mock, "test-targets")
	got, err := s.ListByDeployment(context.Background(), "us-east-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 target, got %d", len(got))
	}
	if got[0].TargetID != "mc01" {
		t.Errorf("expected 'mc01', got %q", got[0].TargetID)
	}
}
