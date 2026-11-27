package store

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// BreakglassRequest is a single DynamoDB item for break-glass lifecycle (request + grant).
type BreakglassRequest struct {
	RequestID        string            `json:"id" dynamodbav:"requestId"`
	Profile          string            `json:"profile" dynamodbav:"profile"`
	Reason           string            `json:"reason" dynamodbav:"reason"`
	Operator         string            `json:"operator" dynamodbav:"operator"`
	ReqSignerARN     string            `json:"req_signer_arn,omitempty" dynamodbav:"reqSignerARN,omitempty"`
	ReqSessionID     string            `json:"req_session_id,omitempty" dynamodbav:"reqSessionId,omitempty"`
	ReqTaskID        string            `json:"req_task_id,omitempty" dynamodbav:"reqTaskId,omitempty"`
	TargetCluster    string            `json:"target_cluster" dynamodbav:"targetCluster"`
	TargetType       string            `json:"target_type" dynamodbav:"targetType"`
	AccountID        string            `json:"account_id" dynamodbav:"accountId"`
	Status           Status            `json:"status" dynamodbav:"status"`
	Approval         string            `json:"approval" dynamodbav:"approval"` // workflow: pending | approved | rejected (never expired)
	Revision         string            `json:"revision,omitempty" dynamodbav:"revision,omitempty"`
	CreatedAt        string            `json:"created_at" dynamodbav:"createdAt"`
	RequestExpiresAt string            `json:"request_expires_at" dynamodbav:"requestExpiresAt"`
	GrantExpiresAt   string            `json:"grant_expires_at,omitempty" dynamodbav:"grantExpiresAt,omitempty"`
	ExpiredAt        string            `json:"expired_at,omitempty" dynamodbav:"expiredAt,omitempty"`
	DecidedAt        string            `json:"decided_at,omitempty" dynamodbav:"decidedAt,omitempty"`
	DecidedBy        string            `json:"decided_by,omitempty" dynamodbav:"decidedBy,omitempty"`
	ActivateEvents   []ActivateEvent   `json:"activate_events,omitempty" dynamodbav:"activateEvents,omitempty"`
	DecisionNote     string            `json:"decision_note,omitempty" dynamodbav:"decisionNote,omitempty"`
	Cleaned          bool              `json:"cleaned" dynamodbav:"cleaned"`
	CleanedAt        string            `json:"cleaned_at,omitempty" dynamodbav:"cleanedAt,omitempty"`
	TargetStatusKey  string            `json:"-" dynamodbav:"targetStatusKey,omitempty"`
	DateBucket       string            `json:"-" dynamodbav:"dateBucket,omitempty"`
	TTL              int64             `json:"-" dynamodbav:"ttl,omitempty"`
	Labels           map[string]string `json:"-" dynamodbav:"-"`
}

// ActivateEvent records one successful POST …/activate (credential issue).
type ActivateEvent struct {
	At        string `json:"at" dynamodbav:"at"`
	TaskID    string `json:"task_id" dynamodbav:"taskId"`
	SessionID string `json:"session_id,omitempty" dynamodbav:"sessionId,omitempty"`
}

// BreakglassFilter filters list queries.
type BreakglassFilter struct {
	Operator     *string
	Target       *string
	Profile      *string
	Status       *Status
	Approval     *string // workflow approval column
	DecidedBy    *string
	Since        *time.Time // created_at window (date-bucket index)
	DecidedSince *time.Time // post-filter on decided_at
	Before       *time.Time
	Limit        int
}

// BreakglassStore persists break-glass requests.
type BreakglassStore interface {
	Create(ctx context.Context, req *BreakglassRequest) error
	Get(ctx context.Context, requestID string) (*BreakglassRequest, error)
	ListByOperator(ctx context.Context, operator string, filter *BreakglassFilter) ([]*BreakglassRequest, error)
	ListAll(ctx context.Context, filter *BreakglassFilter) ([]*BreakglassRequest, error)
	Transition(ctx context.Context, requestID string, from, to Status, updates map[string]interface{}) error
	QueryByTargetAndStatus(ctx context.Context, target string, status Status) ([]*BreakglassRequest, error)
	MarkCleaned(ctx context.Context, requestID string) error
	AppendActivateEvent(ctx context.Context, requestID string, ev ActivateEvent) error
}

// DynamoDBBreakglassStore implements BreakglassStore.
type DynamoDBBreakglassStore struct {
	client    DynamoDBAPI
	tableName string
	ttlDays   int
}

func NewBreakglassStore(client DynamoDBAPI, tableName string, ttlDays int) *DynamoDBBreakglassStore {
	return &DynamoDBBreakglassStore{
		client:    client,
		tableName: tableName,
		ttlDays:   ttlDays,
	}
}

// BreakglassTargetStatusKey builds the target-status-index sort key.
func BreakglassTargetStatusKey(status Status, suffix string) string {
	return string(status) + "#" + suffix
}

func (s *DynamoDBBreakglassStore) prepareForWrite(req *BreakglassRequest) error {
	if req.RequestID == "" {
		return fmt.Errorf("requestId is required")
	}
	if req.Approval == "" {
		req.Approval = WorkflowApprovalPending
	}
	if req.CreatedAt == "" {
		req.CreatedAt = time.Now().Format(time.RFC3339Nano)
	}
	req.DateBucket = req.CreatedAt[:10]
	req.TTL = time.Now().AddDate(0, 0, s.ttlDays).Unix()
	switch req.Status {
	case StatusPending:
		if req.RequestExpiresAt == "" {
			return fmt.Errorf("requestExpiresAt required for pending")
		}
		req.TargetStatusKey = BreakglassTargetStatusKey(req.Status, req.RequestExpiresAt)
	case StatusActive:
		if req.GrantExpiresAt == "" {
			return fmt.Errorf("grantExpiresAt required for active")
		}
		req.TargetStatusKey = BreakglassTargetStatusKey(req.Status, req.GrantExpiresAt)
	case StatusDenied, StatusExpired, StatusFailed:
		req.TargetStatusKey = BreakglassTargetStatusKey(req.Status, req.CreatedAt)
	default:
		req.TargetStatusKey = BreakglassTargetStatusKey(req.Status, req.CreatedAt)
	}
	return nil
}

func (s *DynamoDBBreakglassStore) Create(ctx context.Context, req *BreakglassRequest) error {
	if err := s.prepareForWrite(req); err != nil {
		return err
	}
	item, err := attributevalue.MarshalMap(req)
	if err != nil {
		return fmt.Errorf("marshaling breakglass request: %w", err)
	}
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           &s.tableName,
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(requestId)"),
	})
	if err != nil {
		return fmt.Errorf("creating breakglass request: %w", err)
	}
	return nil
}

func (s *DynamoDBBreakglassStore) Get(ctx context.Context, requestID string) (*BreakglassRequest, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"requestId": &types.AttributeValueMemberS{Value: requestID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting breakglass request: %w", err)
	}
	if out.Item == nil {
		return nil, nil
	}
	var req BreakglassRequest
	if err := attributevalue.UnmarshalMap(out.Item, &req); err != nil {
		return nil, fmt.Errorf("unmarshaling breakglass request: %w", err)
	}
	return &req, nil
}

func (s *DynamoDBBreakglassStore) Transition(ctx context.Context, requestID string, from, to Status, updates map[string]interface{}) error {
	key, err := attributevalue.MarshalMap(map[string]string{"requestId": requestID})
	if err != nil {
		return err
	}
	update := expression.Set(expression.Name("status"), expression.Value(string(to)))
	for k, v := range updates {
		update = update.Set(expression.Name(k), expression.Value(v))
	}
	if suffix, ok := stringAttrFromUpdates(updates, "grantExpiresAt"); ok && to == StatusActive {
		update = update.Set(expression.Name("targetStatusKey"), expression.Value(BreakglassTargetStatusKey(to, suffix)))
	} else if suffix, ok := stringAttrFromUpdates(updates, "requestExpiresAt"); ok && to == StatusPending {
		update = update.Set(expression.Name("targetStatusKey"), expression.Value(BreakglassTargetStatusKey(to, suffix)))
	} else if to == StatusDenied || to == StatusExpired || to == StatusFailed {
		update = update.Set(expression.Name("targetStatusKey"), expression.Value(BreakglassTargetStatusKey(to, time.Now().Format(time.RFC3339Nano))))
	}
	cond := expression.Name("status").Equal(expression.Value(string(from)))
	expr, err := expression.NewBuilder().WithUpdate(update).WithCondition(cond).Build()
	if err != nil {
		return fmt.Errorf("building update expression: %w", err)
	}
	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &s.tableName,
		Key:                       key,
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		return fmt.Errorf("transitioning breakglass request: %w", err)
	}
	return nil
}

func (s *DynamoDBBreakglassStore) QueryByTargetAndStatus(ctx context.Context, target string, status Status) ([]*BreakglassRequest, error) {
	keyCond := expression.Key("targetCluster").Equal(expression.Value(target)).
		And(expression.Key("targetStatusKey").BeginsWith(string(status) + "#"))
	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, err
	}
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 &s.tableName,
		IndexName:                 aws.String("target-status-index"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		return nil, fmt.Errorf("querying breakglass by target/status: %w", err)
	}
	var results []*BreakglassRequest
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *DynamoDBBreakglassStore) MarkCleaned(ctx context.Context, requestID string) error {
	now := time.Now().Format(time.RFC3339Nano)
	key, err := attributevalue.MarshalMap(map[string]string{"requestId": requestID})
	if err != nil {
		return err
	}
	update := expression.Set(expression.Name("cleaned"), expression.Value(true)).
		Set(expression.Name("cleanedAt"), expression.Value(now))
	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return err
	}
	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &s.tableName,
		Key:                       key,
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		return fmt.Errorf("marking breakglass cleaned: %w", err)
	}
	return nil
}

// ListAll queries date-bucket-index with optional filters (operator omitted = all operators; target omitted = all targets in table).
func (s *DynamoDBBreakglassStore) ListAll(ctx context.Context, filter *BreakglassFilter) ([]*BreakglassRequest, error) {
	operator := ""
	if filter != nil && filter.Operator != nil {
		operator = *filter.Operator
	}
	return s.ListByOperator(ctx, operator, filter)
}

// ListByOperator queries date-bucket-index with operator filter (newest days first).
func (s *DynamoDBBreakglassStore) ListByOperator(ctx context.Context, operator string, filter *BreakglassFilter) ([]*BreakglassRequest, error) {
	limit := 50
	if filter != nil && filter.Limit > 0 {
		limit = filter.Limit
	}
	since := time.Now().Add(-24 * time.Hour)
	if filter != nil && filter.Since != nil {
		since = *filter.Since
	}
	var results []*BreakglassRequest
	for day := time.Now(); !day.Before(since.Truncate(24 * time.Hour)); day = day.AddDate(0, 0, -1) {
		if len(results) >= limit {
			break
		}
		bucket := day.Format("2006-01-02")
		keyCond := expression.Key("dateBucket").Equal(expression.Value(bucket))
		builder := expression.NewBuilder().WithKeyCondition(keyCond)
		var filt expression.ConditionBuilder
		hasFilter := false
		addFilter := func(part expression.ConditionBuilder) {
			if hasFilter {
				filt = filt.And(part)
			} else {
				filt = part
				hasFilter = true
			}
		}
		if operator != "" {
			addFilter(expression.Name("operator").Equal(expression.Value(operator)))
		}
		if filter != nil {
			if filter.Status != nil {
				addFilter(expression.Name("status").Equal(expression.Value(string(*filter.Status))))
			}
			if filter.Approval != nil {
				addFilter(expression.Name("approval").Equal(expression.Value(*filter.Approval)))
			}
			if filter.DecidedBy != nil {
				addFilter(expression.Name("decidedBy").Equal(expression.Value(*filter.DecidedBy)))
			}
			if filter.Target != nil && *filter.Target != "" {
				addFilter(expression.Name("targetCluster").Equal(expression.Value(*filter.Target)))
			}
		}
		if hasFilter {
			builder = builder.WithFilter(filt)
		}
		expr, err := builder.Build()
		if err != nil {
			return nil, err
		}
		out, err := s.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 &s.tableName,
			IndexName:                 aws.String("date-bucket-index"),
			KeyConditionExpression:    expr.KeyCondition(),
			FilterExpression:          expr.Filter(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			ScanIndexForward:          aws.Bool(false),
			Limit:                     aws.Int32(int32(limit - len(results))),
		})
		if err != nil {
			return nil, fmt.Errorf("listing breakglass requests: %w", err)
		}
		var batch []*BreakglassRequest
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &batch); err != nil {
			return nil, err
		}
		results = append(results, batch...)
	}
	return filterBreakglassItems(results, filter), nil
}

func filterBreakglassItems(items []*BreakglassRequest, filter *BreakglassFilter) []*BreakglassRequest {
	if filter == nil {
		return items
	}
	var out []*BreakglassRequest
	for _, item := range items {
		if filter.Status != nil && item.Status != *filter.Status {
			continue
		}
		if filter.Approval != nil && item.Approval != *filter.Approval {
			continue
		}
		if filter.DecidedBy != nil && item.DecidedBy != *filter.DecidedBy {
			continue
		}
		if filter.Target != nil && *filter.Target != "" && item.TargetCluster != *filter.Target {
			continue
		}
		if filter.Profile != nil && *filter.Profile != "" && item.Profile != *filter.Profile {
			continue
		}
		if filter.Before != nil && item.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, item.CreatedAt); err == nil && t.After(*filter.Before) {
				continue
			}
		}
		if filter.DecidedSince != nil {
			if item.DecidedAt == "" {
				continue
			}
			t, err := time.Parse(time.RFC3339Nano, item.DecidedAt)
			if err != nil || t.Before(*filter.DecidedSince) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func (s *DynamoDBBreakglassStore) AppendActivateEvent(ctx context.Context, requestID string, ev ActivateEvent) error {
	if ev.At == "" {
		ev.At = time.Now().Format(time.RFC3339Nano)
	}
	req, err := s.Get(ctx, requestID)
	if err != nil {
		return err
	}
	if req == nil {
		return fmt.Errorf("breakglass request not found")
	}
	req.ActivateEvents = append(req.ActivateEvents, ev)
	item, err := attributevalue.MarshalMap(map[string]interface{}{
		"requestId":      requestID,
		"activateEvents": req.ActivateEvents,
	})
	if err != nil {
		return err
	}
	key := map[string]types.AttributeValue{
		"requestId": item["requestId"],
	}
	delete(item, "requestId")
	update := expression.Set(expression.Name("activateEvents"), expression.Value(req.ActivateEvents))
	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return fmt.Errorf("building activate append: %w", err)
	}
	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &s.tableName,
		Key:                       key,
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		return fmt.Errorf("appending activateEvents: %w", err)
	}
	return nil
}
