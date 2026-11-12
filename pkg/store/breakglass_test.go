package store

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func TestBreakglassTargetStatusKey_WhenPending_ItShouldPrefixStatus(t *testing.T) {
	deadline := "2026-10-17T12:00:00.000000000Z"
	got := BreakglassTargetStatusKey(StatusPendingApproval, deadline)
	if got != "pending_approval#"+deadline {
		t.Errorf("unexpected key: %q", got)
	}
}

func TestDynamoDBBreakglassStore_Create_WhenPending_ItShouldSetTTLAndTargetStatusKey(t *testing.T) {
	var captured *dynamodb.PutItemInput
	mock := &mockDynamoDBAPI{
		putItemFn: func(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			captured = params
			return &dynamodb.PutItemOutput{}, nil
		},
	}
	s := NewBreakglassStore(mock, "test-breakglass", 30)
	expires := time.Now().Add(24 * time.Hour).Format(time.RFC3339Nano)
	req := &BreakglassRequest{
		RequestID:        "req-1",
		Scope:            "kube-read",
		Reason:           "incident",
		Operator:         "alice",
		TargetCluster:    "mc01",
		Status:           StatusPendingApproval,
		RequestExpiresAt: expires,
	}
	if err := s.Create(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil {
		t.Fatal("PutItem not called")
	}
	if req.TTL == 0 {
		t.Error("expected ttl to be set")
	}
	wantKey := BreakglassTargetStatusKey(StatusPendingApproval, expires)
	if req.TargetStatusKey != wantKey {
		t.Errorf("expected targetStatusKey %q, got %q", wantKey, req.TargetStatusKey)
	}
}
