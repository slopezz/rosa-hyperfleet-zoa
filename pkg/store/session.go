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

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/boundaryexec"
)

// SessionStatus represents the lifecycle state of a boundary session.
type SessionStatus string

const (
	SessionStatusCreating   SessionStatus = "creating"
	SessionStatusActive     SessionStatus = "active"
	SessionStatusTerminated SessionStatus = "terminated"
	SessionStatusFailed     SessionStatus = "failed"
)

// StopReason is why a boundary session was terminated. Stored in DynamoDB (stopReason).
type StopReason string

const (
	StopReasonOperatorStop       StopReason = "operatorStop"
	StopReasonDeadlineReaperStop StopReason = "deadlineReaperStop"
	StopReasonIdleReaperStop       StopReason = "idleReaperStop"
	StopReasonProvisionFailed      StopReason = "provisionFailed"
	StopReasonStaleCreatingReaper  StopReason = "staleCreatingReaperStop"
)

// Session represents a ZOA Boundary session in DynamoDB.
// PK sessionId is a UUID from session_start; taskId (GSI task-id-index) is set when the ECS task is active.
type Session struct {
	SessionID      string        `json:"session_id" dynamodbav:"sessionId"`
	Operator       string        `json:"operator" dynamodbav:"operator"`
	SignerARN      string        `json:"signer_arn" dynamodbav:"signerARN"`
	AccountID      string        `json:"account_id,omitempty" dynamodbav:"accountId,omitempty"`
	Reason         string        `json:"reason" dynamodbav:"reason"`
	TargetCluster  string        `json:"target_cluster" dynamodbav:"targetCluster"`
	Region         string        `json:"region" dynamodbav:"region"`
	TaskArn        string        `json:"task_arn,omitempty" dynamodbav:"taskArn,omitempty"`
	TaskID         string        `json:"task_id,omitempty" dynamodbav:"taskId,omitempty"`
	EcsCluster     string        `json:"ecs_cluster,omitempty" dynamodbav:"ecsCluster,omitempty"`
	Status         SessionStatus `json:"status" dynamodbav:"status"`
	CreatedAt      string        `json:"created_at" dynamodbav:"createdAt"`
	Deadline       string        `json:"deadline" dynamodbav:"deadline"`
	TerminatedAt   string        `json:"terminated_at,omitempty" dynamodbav:"terminatedAt,omitempty"`
	StopReason     string        `json:"stop_reason,omitempty" dynamodbav:"stopReason,omitempty"`
	VpcId          string        `json:"vpc_id,omitempty" dynamodbav:"vpcId,omitempty"`
	DeploymentName string        `json:"deployment_name,omitempty" dynamodbav:"deploymentName,omitempty"`

	// ECS Exec (SSM) session ids for each join; stream names are ecs-execute-command-<id>.
	ExecSessionIDs []string `json:"exec_session_ids,omitempty" dynamodbav:"execSessionIds,omitempty"`

	DateBucket string `json:"-" dynamodbav:"dateBucket,omitempty"`
	TTL        int64  `json:"-" dynamodbav:"ttl,omitempty"`

	// target-status-index sort key: "<status>#<deadline|terminatedAt|...>" (RFC3339Nano suffix for active).
	TargetStatusKey string `json:"-" dynamodbav:"targetStatusKey,omitempty"`
}

// SessionFilter holds optional filters for listing sessions.
type SessionFilter struct {
	Status   *SessionStatus
	Operator *string
	Target   *string
	Since    *time.Time
	Before   *time.Time
	Limit    int
}

// SessionStore defines DynamoDB operations for boundary sessions.
type SessionStore interface {
	Put(ctx context.Context, session *Session) error
	Get(ctx context.Context, sessionID string) (*Session, error)

	// GetByTaskID looks up a session by ECS task ID via the task-id-index GSI.
	// Used by the identity bridge to resolve SRE identity from a SigV4 caller
	// ARN (task role's RoleSessionName = task ID). Returns nil if not found.
	GetByTaskID(ctx context.Context, taskID string) (*Session, error)

	// ListAll returns sessions across all operators via date-bucket-index.
	// Follows the same pattern as ExecutionStore.ListAll — queries day-by-day
	// from newest to oldest. CLI defaults to 24h to prevent unbounded queries.
	ListAll(ctx context.Context, filter *SessionFilter) ([]*Session, error)

	UpdateStatus(ctx context.Context, sessionID string, from, to SessionStatus, updates map[string]interface{}) error

	// ListExpired returns creating and active sessions past deadline for targetCluster (target-status-index).
	ListExpired(ctx context.Context, targetCluster string) ([]*Session, error)

	// ListOpenByTargetStatus returns all sessions in the given status for targetCluster (target-status-index).
	ListOpenByTargetStatus(ctx context.Context, targetCluster string, status SessionStatus) ([]*Session, error)

	// ListActiveBeforeDeadline returns active sessions whose deadline is still in the future for targetCluster.
	ListActiveBeforeDeadline(ctx context.Context, targetCluster string) ([]*Session, error)

	// RecordExecSession appends an ECS Exec session id after the CLI runs ExecuteCommand.
	RecordExecSession(ctx context.Context, sessionID, operator, execSessionID string) error
}

// DynamoDBSessionStore implements SessionStore backed by DynamoDB.
type DynamoDBSessionStore struct {
	client    DynamoDBAPI
	tableName string
	ttlDays   int
}

// activeStatusKeyCeiling is the first target-status-index sort key that does not
// belong to an active session. The active prefix is "active#", and '$' is the
// next code point after '#'.
const activeStatusKeyCeiling = "active$"

const creatingStatusKeyCeiling = "creating$"

// SessionTargetStatusKey builds the target-status-index sort key for a session row.
func SessionTargetStatusKey(status SessionStatus, suffix string) string {
	return string(status) + "#" + suffix
}

func sessionTargetStatusKeyForStatus(to SessionStatus, now string, updates map[string]interface{}) (string, error) {
	switch to {
	case SessionStatusCreating, SessionStatusActive:
		deadline, ok := stringAttrFromUpdates(updates, "deadline")
		if !ok {
			return "", fmt.Errorf("deadline required in updates when session status is %s", to)
		}
		return SessionTargetStatusKey(to, deadline), nil
	case SessionStatusTerminated, SessionStatusFailed:
		return SessionTargetStatusKey(to, now), nil
	default:
		return "", nil
	}
}

func stringAttrFromUpdates(updates map[string]interface{}, key string) (string, bool) {
	if updates == nil {
		return "", false
	}
	v, ok := updates[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func NewSessionStore(client DynamoDBAPI, tableName string, ttlDays int) *DynamoDBSessionStore {
	return &DynamoDBSessionStore{
		client:    client,
		tableName: tableName,
		ttlDays:   ttlDays,
	}
}

func (s *DynamoDBSessionStore) Put(ctx context.Context, session *Session) error {
	session.TTL = time.Now().AddDate(0, 0, s.ttlDays).Unix()
	session.DateBucket = session.CreatedAt[:10]
	session.TargetStatusKey = SessionTargetStatusKey(session.Status, session.Deadline)

	item, err := attributevalue.MarshalMap(session)
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           &s.tableName,
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(sessionId)"),
	})
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	return nil
}

func (s *DynamoDBSessionStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"sessionId": &types.AttributeValueMemberS{Value: sessionID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting session: %w", err)
	}
	if out.Item == nil {
		return nil, nil
	}

	var session Session
	if err := attributevalue.UnmarshalMap(out.Item, &session); err != nil {
		return nil, fmt.Errorf("unmarshaling session: %w", err)
	}
	return &session, nil
}

// GetByTaskID is the identity bridge: ECS task UUID (STS RoleSessionName on the task role)
// → session row → human operator. Uses GSI task-id-index (see docs/boundary/identity-bridge.md).
func (s *DynamoDBSessionStore) GetByTaskID(ctx context.Context, taskID string) (*Session, error) {
	keyCond := expression.Key("taskId").Equal(expression.Value(taskID))
	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, fmt.Errorf("building task-id query expression: %w", err)
	}

	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 &s.tableName,
		IndexName:                 aws.String("task-id-index"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(1),
	})
	if err != nil {
		return nil, fmt.Errorf("querying session by task ID: %w", err)
	}
	if len(out.Items) == 0 {
		return nil, nil
	}

	var session Session
	if err := attributevalue.UnmarshalMap(out.Items[0], &session); err != nil {
		return nil, fmt.Errorf("unmarshaling session: %w", err)
	}
	return &session, nil
}

// ListAll returns sessions across all operators via date-bucket-index GSI
// (PK=dateBucket, SK=createdAt). Queries day-by-day from newest to oldest,
// matching the ExecutionStore.ListAll pattern. CLI enforces a default of 24h.
func (s *DynamoDBSessionStore) ListAll(ctx context.Context, filter *SessionFilter) ([]*Session, error) {
	resultLimit := 100
	if filter != nil && filter.Limit > 0 {
		resultLimit = filter.Limit
	}

	since := time.Now().Add(-24 * time.Hour)
	if filter != nil && filter.Since != nil {
		since = *filter.Since
	}

	endTime := time.Now().UTC()
	if filter != nil && filter.Before != nil {
		endTime = filter.Before.UTC()
	}

	sinceStr := since.Format(time.RFC3339Nano)
	endDay := endTime.Truncate(24 * time.Hour)
	startDay := since.UTC().Truncate(24 * time.Hour)

	var sessions []*Session
	for day := endDay; !day.Before(startDay); day = day.AddDate(0, 0, -1) {
		bucket := day.Format("2006-01-02")

		var keyCond expression.KeyConditionBuilder
		if filter != nil && filter.Before != nil {
			beforeStr := filter.Before.Format(time.RFC3339Nano)
			keyCond = expression.KeyAnd(
				expression.Key("dateBucket").Equal(expression.Value(bucket)),
				expression.Key("createdAt").Between(
					expression.Value(sinceStr),
					expression.Value(beforeStr),
				),
			)
		} else {
			keyCond = expression.KeyAnd(
				expression.Key("dateBucket").Equal(expression.Value(bucket)),
				expression.Key("createdAt").GreaterThanEqual(expression.Value(sinceStr)),
			)
		}

		builder := expression.NewBuilder().WithKeyCondition(keyCond)

		// Apply non-key filters.
		var conditions []expression.ConditionBuilder
		if filter != nil {
			if filter.Status != nil {
				conditions = append(conditions, expression.Name("status").Equal(expression.Value(string(*filter.Status))))
			}
			if filter.Operator != nil {
				conditions = append(conditions, expression.Name("operator").Equal(expression.Value(*filter.Operator)))
			}
			if filter.Target != nil {
				conditions = append(conditions, expression.Name("targetCluster").Equal(expression.Value(*filter.Target)))
			}
		}
		if len(conditions) > 0 {
			combined := conditions[0]
			for _, c := range conditions[1:] {
				combined = combined.And(c)
			}
			builder = builder.WithFilter(combined)
		}

		expr, err := builder.Build()
		if err != nil {
			return nil, fmt.Errorf("building date-bucket query expression: %w", err)
		}

		const maxPages = 10
		var lastKey map[string]types.AttributeValue
		for page := 0; page < maxPages; page++ {
			input := &dynamodb.QueryInput{
				TableName:                 &s.tableName,
				IndexName:                 aws.String("date-bucket-index"),
				KeyConditionExpression:    expr.KeyCondition(),
				ExpressionAttributeNames:  expr.Names(),
				ExpressionAttributeValues: expr.Values(),
				FilterExpression:          expr.Filter(),
				ScanIndexForward:          aws.Bool(false),
				ExclusiveStartKey:         lastKey,
			}

			out, err := s.client.Query(ctx, input)
			if err != nil {
				return nil, fmt.Errorf("querying session date-bucket: %w", err)
			}

			for _, item := range out.Items {
				var session Session
				if err := attributevalue.UnmarshalMap(item, &session); err != nil {
					return nil, fmt.Errorf("unmarshaling session: %w", err)
				}
				sessions = append(sessions, &session)
			}

			if resultLimit > 0 && len(sessions) >= resultLimit {
				sessions = sessions[:resultLimit]
				return sessions, nil
			}

			if out.LastEvaluatedKey == nil {
				break
			}
			lastKey = out.LastEvaluatedKey
		}
	}

	return sessions, nil
}

func (s *DynamoDBSessionStore) UpdateStatus(ctx context.Context, sessionID string, from, to SessionStatus, updates map[string]interface{}) error {
	now := time.Now().Format(time.RFC3339Nano)
	update := expression.Set(
		expression.Name("status"), expression.Value(string(to)),
	)

	if to == SessionStatusTerminated {
		update = update.Set(expression.Name("terminatedAt"), expression.Value(now))
	}

	for key, val := range updates {
		update = update.Set(expression.Name(key), expression.Value(val))
	}

	targetStatusKey, err := sessionTargetStatusKeyForStatus(to, now, updates)
	if err != nil {
		return err
	}
	if targetStatusKey != "" {
		update = update.Set(expression.Name("targetStatusKey"), expression.Value(targetStatusKey))
	}

	condition := expression.Name("status").Equal(expression.Value(string(from)))

	expr, err := expression.NewBuilder().WithUpdate(update).WithCondition(condition).Build()
	if err != nil {
		return fmt.Errorf("building update expression: %w", err)
	}

	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"sessionId": &types.AttributeValueMemberS{Value: sessionID},
		},
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	return err
}

// ListExpired queries target-status-index for creating and active sessions past their deadline.
func (s *DynamoDBSessionStore) ListExpired(ctx context.Context, targetCluster string) ([]*Session, error) {
	if targetCluster == "" {
		return nil, fmt.Errorf("targetCluster is required for ListExpired")
	}
	var out []*Session
	for _, status := range []SessionStatus{SessionStatusCreating, SessionStatusActive} {
		part, err := s.listPastDeadlineByStatus(ctx, targetCluster, status)
		if err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	return out, nil
}

func (s *DynamoDBSessionStore) listPastDeadlineByStatus(ctx context.Context, targetCluster string, status SessionStatus) ([]*Session, error) {
	now := time.Now().Format(time.RFC3339Nano)
	prefix := SessionTargetStatusKey(status, "")
	upper := SessionTargetStatusKey(status, now)

	keyCond := expression.KeyAnd(
		expression.Key("targetCluster").Equal(expression.Value(targetCluster)),
		expression.Key("targetStatusKey").Between(expression.Value(prefix), expression.Value(upper)),
	)

	built, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, fmt.Errorf("building expired %s sessions expression: %w", status, err)
	}

	return s.queryTargetStatusIndex(ctx, built)
}

// ListOpenByTargetStatus returns every session row in status for the target partition.
func (s *DynamoDBSessionStore) ListOpenByTargetStatus(ctx context.Context, targetCluster string, status SessionStatus) ([]*Session, error) {
	if targetCluster == "" {
		return nil, fmt.Errorf("targetCluster is required for ListOpenByTargetStatus")
	}
	prefix := SessionTargetStatusKey(status, "")
	ceiling, ok := statusKeyCeiling(status)
	if !ok {
		return nil, fmt.Errorf("unsupported status for ListOpenByTargetStatus: %s", status)
	}

	keyCond := expression.KeyAnd(
		expression.Key("targetCluster").Equal(expression.Value(targetCluster)),
		expression.Key("targetStatusKey").Between(expression.Value(prefix), expression.Value(ceiling)),
	)

	built, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, fmt.Errorf("building open %s sessions expression: %w", status, err)
	}
	return s.queryTargetStatusIndex(ctx, built)
}

func statusKeyCeiling(status SessionStatus) (string, bool) {
	switch status {
	case SessionStatusActive:
		return activeStatusKeyCeiling, true
	case SessionStatusCreating:
		return creatingStatusKeyCeiling, true
	default:
		return "", false
	}
}

func (s *DynamoDBSessionStore) queryTargetStatusIndex(ctx context.Context, built expression.Expression) ([]*Session, error) {
	var sessions []*Session
	var lastKey map[string]types.AttributeValue
	for {
		input := &dynamodb.QueryInput{
			TableName:                 &s.tableName,
			IndexName:                 aws.String("target-status-index"),
			KeyConditionExpression:    built.KeyCondition(),
			ExpressionAttributeNames:  built.Names(),
			ExpressionAttributeValues: built.Values(),
			ExclusiveStartKey:         lastKey,
		}

		out, err := s.client.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("querying target-status-index: %w", err)
		}

		for _, item := range out.Items {
			var session Session
			if err := attributevalue.UnmarshalMap(item, &session); err != nil {
				return nil, fmt.Errorf("unmarshaling session: %w", err)
			}
			sessions = append(sessions, &session)
		}

		lastKey = out.LastEvaluatedKey
		if lastKey == nil {
			break
		}
	}
	return sessions, nil
}

// ListActiveBeforeDeadline queries target-status-index for active sessions not yet past deadline.
// targetStatusKey is the sort key, so the deadline bound has to be part of the key
// condition. DynamoDB rejects that attribute in FilterExpression.
func (s *DynamoDBSessionStore) ListActiveBeforeDeadline(ctx context.Context, targetCluster string) ([]*Session, error) {
	if targetCluster == "" {
		return nil, fmt.Errorf("targetCluster is required for ListActiveBeforeDeadline")
	}
	now := time.Now().Format(time.RFC3339Nano)
	// Strictly after active#<now>, so a deadline equal to now stays on the expired side
	// of the range (ListExpired includes that key). Real deadline keys are not
	// extensions of the current timestamp, so this still sits below the next later key.
	lower := SessionTargetStatusKey(SessionStatusActive, now) + "\uffff"

	keyCond := expression.KeyAnd(
		expression.Key("targetCluster").Equal(expression.Value(targetCluster)),
		expression.Key("targetStatusKey").Between(
			expression.Value(lower),
			expression.Value(activeStatusKeyCeiling),
		),
	)

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, fmt.Errorf("building active sessions expression: %w", err)
	}

	var sessions []*Session
	var lastKey map[string]types.AttributeValue
	for {
		out, err := s.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 &s.tableName,
			IndexName:                 aws.String("target-status-index"),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			ExclusiveStartKey:         lastKey,
		})
		if err != nil {
			return nil, fmt.Errorf("querying active sessions: %w", err)
		}

		for _, item := range out.Items {
			var session Session
			if err := attributevalue.UnmarshalMap(item, &session); err != nil {
				return nil, fmt.Errorf("unmarshaling session: %w", err)
			}
			sessions = append(sessions, &session)
		}

		lastKey = out.LastEvaluatedKey
		if lastKey == nil {
			break
		}
	}
	return sessions, nil
}

func (s *DynamoDBSessionStore) RecordExecSession(ctx context.Context, sessionID, operator, execSessionID string) error {
	streamName := boundaryexec.LogStreamName(execSessionID)

	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"sessionId": &types.AttributeValueMemberS{Value: sessionID},
		},
		UpdateExpression:    aws.String(`SET execSessionIds = list_append(if_not_exists(execSessionIds, :empty), :sid)`),
		ConditionExpression: aws.String("#st = :active AND #operator = :op"),
		ExpressionAttributeNames: map[string]string{
			"#st":       "status",
			"#operator": "operator",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":empty":  &types.AttributeValueMemberL{Value: []types.AttributeValue{}},
			":sid":    &types.AttributeValueMemberL{Value: []types.AttributeValue{&types.AttributeValueMemberS{Value: streamName}}},
			":active": &types.AttributeValueMemberS{Value: string(SessionStatusActive)},
			":op":     &types.AttributeValueMemberS{Value: operator},
		},
	})
	if err != nil {
		return fmt.Errorf("recording exec session: %w", err)
	}
	return nil
}
