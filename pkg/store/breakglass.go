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
	Scope            string            `json:"scope" dynamodbav:"scope"`
	Reason           string            `json:"reason" dynamodbav:"reason"`
	Operator         string            `json:"operator" dynamodbav:"operator"`
	SignerARN        string            `json:"signer_arn,omitempty" dynamodbav:"signerARN,omitempty"`
	SessionID        string            `json:"session_id,omitempty" dynamodbav:"sessionId,omitempty"`
	CreatorTaskID    string            `json:"creator_task_id,omitempty" dynamodbav:"creatorTaskId,omitempty"`
	CreatorTaskArn   string            `json:"creator_task_arn,omitempty" dynamodbav:"creatorTaskArn,omitempty"`
	TargetCluster    string            `json:"target_cluster" dynamodbav:"targetCluster"`
	TargetType       string            `json:"target_type" dynamodbav:"targetType"`
	AccountID        string            `json:"account_id" dynamodbav:"accountId"`
	Status           Status            `json:"status" dynamodbav:"status"`
	Approval         string            `json:"approval" dynamodbav:"approval"`
	Revision         string            `json:"revision,omitempty" dynamodbav:"revision,omitempty"`
	CreatedAt        string            `json:"created_at" dynamodbav:"createdAt"`
	RequestExpiresAt string            `json:"request_expires_at" dynamodbav:"requestExpiresAt"`
	GrantExpiresAt   string            `json:"grant_expires_at,omitempty" dynamodbav:"grantExpiresAt,omitempty"`
	ApprovedBy       string            `json:"approved_by,omitempty" dynamodbav:"approvedBy,omitempty"`
	ApprovedAt       string            `json:"approved_at,omitempty" dynamodbav:"approvedAt,omitempty"`
	RejectedBy       string            `json:"rejected_by,omitempty" dynamodbav:"rejectedBy,omitempty"`
	RejectedAt       string            `json:"rejected_at,omitempty" dynamodbav:"rejectedAt,omitempty"`
	DecisionNote     string            `json:"decision_note,omitempty" dynamodbav:"decisionNote,omitempty"`
	Cleaned          bool              `json:"cleaned" dynamodbav:"cleaned"`
	CleanedAt        string            `json:"cleaned_at,omitempty" dynamodbav:"cleanedAt,omitempty"`
	TargetStatusKey  string            `json:"-" dynamodbav:"targetStatusKey,omitempty"`
	DateBucket       string            `json:"-" dynamodbav:"dateBucket,omitempty"`
	TTL              int64             `json:"-" dynamodbav:"ttl,omitempty"`
	Labels           map[string]string `json:"-" dynamodbav:"-"`
}

// BreakglassFilter filters list queries.
type BreakglassFilter struct {
	Operator *string
	Target   *string
	Status   *Status
	Since    *time.Time
	Before   *time.Time
	Limit    int
}

// BreakglassStore persists break-glass requests.
type BreakglassStore interface {
	Create(ctx context.Context, req *BreakglassRequest) error
	Get(ctx context.Context, requestID string) (*BreakglassRequest, error)
	ListByOperator(ctx context.Context, operator string, filter *BreakglassFilter) ([]*BreakglassRequest, error)
	Transition(ctx context.Context, requestID string, from, to Status, updates map[string]interface{}) error
	QueryByTargetAndStatus(ctx context.Context, target string, status Status) ([]*BreakglassRequest, error)
	MarkCleaned(ctx context.Context, requestID string) error
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
	if req.CreatedAt == "" {
		req.CreatedAt = time.Now().Format(time.RFC3339Nano)
	}
	req.DateBucket = req.CreatedAt[:10]
	req.TTL = time.Now().AddDate(0, 0, s.ttlDays).Unix()
	switch req.Status {
	case StatusPendingApproval:
		if req.RequestExpiresAt == "" {
			return fmt.Errorf("requestExpiresAt required for pending_approval")
		}
		req.TargetStatusKey = BreakglassTargetStatusKey(req.Status, req.RequestExpiresAt)
	case StatusApproved:
		if req.GrantExpiresAt == "" {
			return fmt.Errorf("grantExpiresAt required for approved")
		}
		req.TargetStatusKey = BreakglassTargetStatusKey(req.Status, req.GrantExpiresAt)
	case StatusRejected, StatusExpired, StatusFailed:
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
	if suffix, ok := stringAttrFromUpdates(updates, "grantExpiresAt"); ok && to == StatusApproved {
		update = update.Set(expression.Name("targetStatusKey"), expression.Value(BreakglassTargetStatusKey(to, suffix)))
	} else if suffix, ok := stringAttrFromUpdates(updates, "requestExpiresAt"); ok && to == StatusPendingApproval {
		update = update.Set(expression.Name("targetStatusKey"), expression.Value(BreakglassTargetStatusKey(to, suffix)))
	} else if to == StatusRejected || to == StatusExpired || to == StatusFailed {
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
		if operator != "" {
			filt := expression.Name("operator").Equal(expression.Value(operator))
			if filter != nil && filter.Status != nil {
				filt = filt.And(expression.Name("status").Equal(expression.Value(string(*filter.Status))))
			}
			if filter != nil && filter.Target != nil && *filter.Target != "" {
				filt = filt.And(expression.Name("targetCluster").Equal(expression.Value(*filter.Target)))
			}
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
	return results, nil
}
