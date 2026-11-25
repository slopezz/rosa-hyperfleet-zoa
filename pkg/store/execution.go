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

type ExecutionStore interface {
	Create(ctx context.Context, exec *Execution) error
	Get(ctx context.Context, executionID string) (*Execution, error)
	List(ctx context.Context, accountID string, limit int, filter *ListFilter) ([]*Execution, error)

	// ListAll returns executions across all accounts via date-bucket-index.
	// All optional filters (target, status, action, etc.) are applied as
	// DynamoDB FilterExpressions server-side.
	ListAll(ctx context.Context, limit int, filter *ListFilter) ([]*Execution, error)

	// TransitionStatus atomically transitions status from→to using DynamoDB conditional writes.
	// Returns error (ConditionalCheckFailedException) if current status != from.
	TransitionStatus(ctx context.Context, id string, from, to Status) error

	// TransitionWithMetadata atomically transitions status and updates additional fields.
	TransitionWithMetadata(ctx context.Context, id string, from, to Status, updates map[string]interface{}) error

	// QueryByStatus queries the status-index GSI for executions in a given status.
	QueryByStatus(ctx context.Context, status Status) ([]*Execution, error)

	// QueryByStatusAndClass queries for executions matching both status and execution class.
	QueryByStatusAndClass(ctx context.Context, status Status, class string) ([]*Execution, error)

	// QueryTerminal returns terminal executions that haven't been cleaned, older than the given age (for GC).
	QueryTerminal(ctx context.Context, olderThan time.Duration) ([]*Execution, error)

	// QueryByTargetAndStatus queries the target-status-index GSI for executions
	// matching both target cluster and status. Used by reconciler/GC to scope
	// queries to their own cluster only.
	QueryByTargetAndStatus(ctx context.Context, target string, status Status) ([]*Execution, error)

	// QueryTerminalByTarget returns terminal executions for a specific target that
	// haven't been cleaned, older than the given age. Used by per-cluster GC.
	QueryTerminalByTarget(ctx context.Context, target string, olderThan time.Duration) ([]*Execution, error)

	// MarkCleaned sets the cleaned flag after K8s resources are garbage-collected.
	MarkCleaned(ctx context.Context, id string) error

	ListByTargetAndAction(ctx context.Context, target, action string, since time.Time) ([]*Execution, error)
	CountActiveByTarget(ctx context.Context, target string) (int, error)
}

type DynamoDBExecutionStore struct {
	client    DynamoDBAPI
	tableName string
	ttlDays   int
}

func NewExecutionStore(client DynamoDBAPI, tableName string, ttlDays int) *DynamoDBExecutionStore {
	return &DynamoDBExecutionStore{
		client:    client,
		tableName: tableName,
		ttlDays:   ttlDays,
	}
}

func (s *DynamoDBExecutionStore) Create(ctx context.Context, exec *Execution) error {
	exec.TTL = time.Now().AddDate(0, 0, s.ttlDays).Unix()
	exec.TargetStatusKey = string(exec.Status) + "#" + exec.CreatedAt
	exec.DateBucket = exec.CreatedAt[:10]

	item, err := attributevalue.MarshalMap(exec)
	if err != nil {
		return fmt.Errorf("marshaling execution: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           &s.tableName,
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(executionId)"),
	})
	if err != nil {
		return fmt.Errorf("creating execution: %w", err)
	}
	return nil
}

func (s *DynamoDBExecutionStore) Get(ctx context.Context, executionID string) (*Execution, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"executionId": &types.AttributeValueMemberS{Value: executionID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting execution: %w", err)
	}
	if out.Item == nil {
		return nil, nil
	}

	var exec Execution
	if err := attributevalue.UnmarshalMap(out.Item, &exec); err != nil {
		return nil, fmt.Errorf("unmarshaling execution: %w", err)
	}
	return &exec, nil
}

func (s *DynamoDBExecutionStore) List(ctx context.Context, accountID string, limit int, filter *ListFilter) ([]*Execution, error) {
	resultLimit := limit
	if filter != nil && filter.Limit > 0 {
		resultLimit = filter.Limit
	}

	// Route to status-index GSI when status is the primary filter.
	// This avoids scanning the entire account partition and filtering in memory.
	if filter != nil && filter.Status != nil {
		return s.listByStatus(ctx, accountID, *filter.Status, filter, resultLimit)
	}

	return s.listByAccount(ctx, accountID, filter, resultLimit)
}

// ListAll returns executions across all accounts via date-bucket-index GSI
// (PK=dateBucket, SK=createdAt). All optional filters (target, status, action,
// etc.) are applied as DynamoDB FilterExpressions on the server side.
//
// The date-bucket-index path requires a time bound (filter.Since) to function.
// The CLI enforces a default of 24h, ensuring no unbounded queries ever reach DynamoDB.
func (s *DynamoDBExecutionStore) ListAll(ctx context.Context, limit int, filter *ListFilter) ([]*Execution, error) {
	resultLimit := limit
	if filter != nil && filter.Limit > 0 {
		resultLimit = filter.Limit
	}

	return s.listAllByDateBucket(ctx, filter, resultLimit)
}

// listAllByDateBucket queries the date-bucket-index GSI day-by-day from today
// (or filter.Before) backwards until the Since boundary. Each bucket query returns
// items sorted by createdAt descending (newest first within that day). Because we
// iterate buckets from newest to oldest, the combined result is naturally newest-first.
func (s *DynamoDBExecutionStore) listAllByDateBucket(ctx context.Context, filter *ListFilter, resultLimit int) ([]*Execution, error) {
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

	var executions []*Execution
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
		builder = s.applyFilterConditions(builder, filter, false)

		expr, err := builder.Build()
		if err != nil {
			return nil, fmt.Errorf("building date-bucket query expression: %w", err)
		}

		items, err := s.paginateQuery(ctx, expr, aws.String("date-bucket-index"), 0)
		if err != nil {
			return nil, err
		}
		executions = append(executions, items...)

		if resultLimit > 0 && len(executions) >= resultLimit {
			executions = executions[:resultLimit]
			return executions, nil
		}
	}

	return executions, nil
}

// listByAccount queries account-index GSI (PK=accountId, SK=createdAt).
// Efficient for time-bounded queries and general listing.
func (s *DynamoDBExecutionStore) listByAccount(ctx context.Context, accountID string, filter *ListFilter, resultLimit int) ([]*Execution, error) {
	keyCond := expression.KeyAnd(
		expression.Key("accountId").Equal(expression.Value(accountID)),
		expression.Key("createdAt").GreaterThan(expression.Value("0")),
	)

	if filter != nil && filter.Since != nil {
		keyCond = expression.KeyAnd(
			expression.Key("accountId").Equal(expression.Value(accountID)),
			expression.Key("createdAt").GreaterThanEqual(expression.Value(filter.Since.Format(time.RFC3339Nano))),
		)
	}

	builder := expression.NewBuilder().WithKeyCondition(keyCond)
	builder = s.applyFilterConditions(builder, filter, false)

	expr, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("building query expression: %w", err)
	}

	return s.paginateQuery(ctx, expr, aws.String("account-index"), resultLimit)
}

// listByStatus queries status-index GSI (PK=status, SK=createdAt).
// Efficient when status is the primary filter — DynamoDB reads only matching status partition.
func (s *DynamoDBExecutionStore) listByStatus(ctx context.Context, accountID string, status Status, filter *ListFilter, resultLimit int) ([]*Execution, error) {
	keyCond := expression.KeyAnd(
		expression.Key("status").Equal(expression.Value(string(status))),
		expression.Key("createdAt").GreaterThan(expression.Value("0")),
	)

	if filter != nil && filter.Since != nil {
		keyCond = expression.KeyAnd(
			expression.Key("status").Equal(expression.Value(string(status))),
			expression.Key("createdAt").GreaterThanEqual(expression.Value(filter.Since.Format(time.RFC3339Nano))),
		)
	}

	// On status-index, accountId is not a key — must filter in FilterExpression.
	// Build all filter conditions together including accountId.
	var conditions []expression.ConditionBuilder
	conditions = append(conditions, expression.Name("accountId").Equal(expression.Value(accountID)))
	if filter != nil {
		if filter.ExecutionMode != nil {
			conditions = append(conditions, expression.Name("executionMode").Equal(expression.Value(*filter.ExecutionMode)))
		}
		if filter.Action != nil {
			conditions = append(conditions, expression.Name("action").Equal(expression.Value(*filter.Action)))
		}
		if filter.Type != nil {
			conditions = append(conditions, expression.Name("type").Equal(expression.Value(*filter.Type)))
		}
		if filter.Scope != nil {
			conditions = append(conditions, executionScopeCondition(*filter.Scope))
		}
		if filter.Operator != nil {
			conditions = append(conditions, expression.Name("operator").Contains(*filter.Operator))
		}
		if filter.DryRun != nil {
			conditions = append(conditions, expression.Name("dryRun").Equal(expression.Value(*filter.DryRun)))
		}
		if filter.Force != nil {
			conditions = append(conditions, expression.Name("force").Equal(expression.Value(*filter.Force)))
		}
	}

	combined := conditions[0]
	for _, c := range conditions[1:] {
		combined = combined.And(c)
	}

	builder := expression.NewBuilder().WithKeyCondition(keyCond).WithFilter(combined)
	expr, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("building query expression: %w", err)
	}

	return s.paginateQuery(ctx, expr, aws.String("status-index"), resultLimit)
}

// applyFilterConditions adds non-key filters to the expression builder.
// skipStatus=true when querying status-index (status is already in KeyCondition).
func (s *DynamoDBExecutionStore) applyFilterConditions(builder expression.Builder, filter *ListFilter, skipStatus bool) expression.Builder {
	if filter == nil {
		return builder
	}

	var conditions []expression.ConditionBuilder
	if filter.Target != nil {
		conditions = append(conditions, expression.Name("targetCluster").Equal(expression.Value(*filter.Target)))
	}
	if !skipStatus && filter.Status != nil {
		conditions = append(conditions, expression.Name("status").Equal(expression.Value(string(*filter.Status))))
	}
	if filter.ExecutionMode != nil {
		conditions = append(conditions, expression.Name("executionMode").Equal(expression.Value(*filter.ExecutionMode)))
	}
	if filter.Action != nil {
		conditions = append(conditions, expression.Name("action").Equal(expression.Value(*filter.Action)))
	}
	if filter.Type != nil {
		conditions = append(conditions, expression.Name("type").Equal(expression.Value(*filter.Type)))
	}
	if filter.Scope != nil {
		conditions = append(conditions, executionScopeCondition(*filter.Scope))
	}
	if filter.Operator != nil {
		conditions = append(conditions, expression.Name("operator").Contains(*filter.Operator))
	}
	if filter.DryRun != nil {
		conditions = append(conditions, expression.Name("dryRun").Equal(expression.Value(*filter.DryRun)))
	}
	if filter.Force != nil {
		conditions = append(conditions, expression.Name("force").Equal(expression.Value(*filter.Force)))
	}
	if len(conditions) > 0 {
		combined := conditions[0]
		for _, c := range conditions[1:] {
			combined = combined.And(c)
		}
		builder = builder.WithFilter(combined)
	}
	return builder
}

// paginateQuery executes a DynamoDB query with pagination and max-pages guard.
func (s *DynamoDBExecutionStore) paginateQuery(ctx context.Context, expr expression.Expression, indexName *string, resultLimit int) ([]*Execution, error) {
	const maxPages = 10

	var executions []*Execution
	var lastKey map[string]types.AttributeValue
	for page := 0; page < maxPages; page++ {
		input := &dynamodb.QueryInput{
			TableName:                 &s.tableName,
			IndexName:                 indexName,
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			FilterExpression:          expr.Filter(),
			ScanIndexForward:          aws.Bool(false),
			ExclusiveStartKey:         lastKey,
		}

		out, err := s.client.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("querying executions: %w", err)
		}

		items, err := unmarshalExecutions(out.Items)
		if err != nil {
			return nil, err
		}
		executions = append(executions, items...)

		if resultLimit > 0 && len(executions) >= resultLimit {
			executions = executions[:resultLimit]
			break
		}

		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}
	return executions, nil
}

func (s *DynamoDBExecutionStore) TransitionStatus(ctx context.Context, id string, from, to Status) error {
	now := time.Now().Format(time.RFC3339Nano)
	update := expression.Set(
		expression.Name("status"), expression.Value(string(to)),
	).Set(
		expression.Name("updatedAt"), expression.Value(now),
	).Set(
		expression.Name("targetStatusKey"), expression.Value(string(to)+"#"+now),
	)

	switch to {
	case StatusDispatched:
		update = update.Set(expression.Name("dispatchedAt"),
			expression.Value(now))
	case StatusSucceeded, StatusFailed, StatusTimedOut:
		update = update.Set(expression.Name("completedAt"),
			expression.Value(now))
	}

	condition := expression.Name("status").Equal(expression.Value(string(from)))

	expr, err := expression.NewBuilder().WithUpdate(update).WithCondition(condition).Build()
	if err != nil {
		return fmt.Errorf("building transition expression: %w", err)
	}

	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"executionId": &types.AttributeValueMemberS{Value: id},
		},
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	return err
}

func (s *DynamoDBExecutionStore) TransitionWithMetadata(ctx context.Context, id string, from, to Status, updates map[string]interface{}) error {
	now := time.Now().Format(time.RFC3339Nano)
	update := expression.Set(
		expression.Name("status"), expression.Value(string(to)),
	).Set(
		expression.Name("updatedAt"), expression.Value(now),
	).Set(
		expression.Name("targetStatusKey"), expression.Value(string(to)+"#"+now),
	)

	switch to {
	case StatusDispatched:
		update = update.Set(expression.Name("dispatchedAt"),
			expression.Value(now))
	case StatusSucceeded, StatusFailed, StatusTimedOut:
		update = update.Set(expression.Name("completedAt"),
			expression.Value(now))
	}

	for key, val := range updates {
		update = update.Set(expression.Name(key), expression.Value(val))
	}

	condition := expression.Name("status").Equal(expression.Value(string(from)))

	expr, err := expression.NewBuilder().WithUpdate(update).WithCondition(condition).Build()
	if err != nil {
		return fmt.Errorf("building transition expression: %w", err)
	}

	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"executionId": &types.AttributeValueMemberS{Value: id},
		},
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	return err
}

func (s *DynamoDBExecutionStore) QueryByStatus(ctx context.Context, status Status) ([]*Execution, error) {
	keyCond := expression.KeyAnd(
		expression.Key("status").Equal(expression.Value(string(status))),
		expression.Key("createdAt").GreaterThan(expression.Value("0")),
	)

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, fmt.Errorf("building query expression: %w", err)
	}

	var all []*Execution
	var lastKey map[string]types.AttributeValue
	for {
		input := &dynamodb.QueryInput{
			TableName:                 &s.tableName,
			IndexName:                 aws.String("status-index"),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			ScanIndexForward:          aws.Bool(true),
			ExclusiveStartKey:         lastKey,
		}
		out, err := s.client.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("querying by status: %w", err)
		}
		items, err := unmarshalExecutions(out.Items)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}
	return all, nil
}

func (s *DynamoDBExecutionStore) QueryByStatusAndClass(ctx context.Context, status Status, class string) ([]*Execution, error) {
	keyCond := expression.KeyAnd(
		expression.Key("status").Equal(expression.Value(string(status))),
		expression.Key("createdAt").GreaterThan(expression.Value("0")),
	)

	filterExpr := expression.Name("executionMode").Equal(expression.Value(class))

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).WithFilter(filterExpr).Build()
	if err != nil {
		return nil, fmt.Errorf("building query expression: %w", err)
	}

	var all []*Execution
	var lastKey map[string]types.AttributeValue
	for {
		input := &dynamodb.QueryInput{
			TableName:                 &s.tableName,
			IndexName:                 aws.String("status-index"),
			KeyConditionExpression:    expr.KeyCondition(),
			FilterExpression:          expr.Filter(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			ScanIndexForward:          aws.Bool(true),
			ExclusiveStartKey:         lastKey,
		}
		out, err := s.client.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("querying by status and class: %w", err)
		}
		items, err := unmarshalExecutions(out.Items)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}
	return all, nil
}

// QueryTerminal returns terminal executions that haven't been cleaned yet,
// older than the given duration (for GC).
func (s *DynamoDBExecutionStore) QueryTerminal(ctx context.Context, olderThan time.Duration) ([]*Execution, error) {
	threshold := time.Now().Add(-olderThan).Format(time.RFC3339Nano)
	var all []*Execution

	cleanedFilter := expression.Name("cleaned").Equal(expression.Value(false)).
		Or(expression.Name("cleaned").AttributeNotExists())

	for _, status := range []Status{StatusSucceeded, StatusFailed, StatusTimedOut} {
		keyCond := expression.KeyAnd(
			expression.Key("status").Equal(expression.Value(string(status))),
			expression.Key("createdAt").LessThan(expression.Value(threshold)),
		)

		expr, err := expression.NewBuilder().WithKeyCondition(keyCond).WithFilter(cleanedFilter).Build()
		if err != nil {
			return nil, fmt.Errorf("building terminal query expression: %w", err)
		}

		var lastKey map[string]types.AttributeValue
		for {
			input := &dynamodb.QueryInput{
				TableName:                 &s.tableName,
				IndexName:                 aws.String("status-index"),
				KeyConditionExpression:    expr.KeyCondition(),
				FilterExpression:          expr.Filter(),
				ExpressionAttributeNames:  expr.Names(),
				ExpressionAttributeValues: expr.Values(),
				ExclusiveStartKey:         lastKey,
			}
			out, err := s.client.Query(ctx, input)
			if err != nil {
				return nil, fmt.Errorf("querying terminal %s: %w", status, err)
			}
			items, err := unmarshalExecutions(out.Items)
			if err != nil {
				return nil, err
			}
			all = append(all, items...)
			if out.LastEvaluatedKey == nil {
				break
			}
			lastKey = out.LastEvaluatedKey
		}
	}

	return all, nil
}

// MarkCleaned sets the cleaned flag to true for a terminal execution.
// Uses a conditional write to ensure the execution is still in a terminal status.
func (s *DynamoDBExecutionStore) MarkCleaned(ctx context.Context, id string) error {
	update := expression.Set(
		expression.Name("cleaned"), expression.Value(true),
	).Set(
		expression.Name("cleanedAt"), expression.Value(time.Now().Format(time.RFC3339Nano)),
	)

	condition := expression.Name("cleaned").Equal(expression.Value(false)).
		Or(expression.Name("cleaned").AttributeNotExists())

	expr, err := expression.NewBuilder().WithUpdate(update).WithCondition(condition).Build()
	if err != nil {
		return fmt.Errorf("building mark-cleaned expression: %w", err)
	}

	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"executionId": &types.AttributeValueMemberS{Value: id},
		},
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	return err
}

func (s *DynamoDBExecutionStore) ListByTargetAndAction(ctx context.Context, target, action string, since time.Time) ([]*Execution, error) {
	sinceStr := since.Format(time.RFC3339Nano)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	startDay := since.UTC().Truncate(24 * time.Hour)

	filterExpr := expression.Name("targetCluster").Equal(expression.Value(target)).
		And(expression.Name("action").Equal(expression.Value(action)))

	var all []*Execution
	for day := today; !day.Before(startDay); day = day.AddDate(0, 0, -1) {
		bucket := day.Format("2006-01-02")

		keyCond := expression.KeyAnd(
			expression.Key("dateBucket").Equal(expression.Value(bucket)),
			expression.Key("createdAt").GreaterThanEqual(expression.Value(sinceStr)),
		)

		expr, err := expression.NewBuilder().WithKeyCondition(keyCond).WithFilter(filterExpr).Build()
		if err != nil {
			return nil, fmt.Errorf("building query expression: %w", err)
		}

		items, err := s.paginateQuery(ctx, expr, aws.String("date-bucket-index"), 0)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

func (s *DynamoDBExecutionStore) CountActiveByTarget(ctx context.Context, target string) (int, error) {
	var total int
	for _, status := range []Status{StatusDispatched, StatusApproved} {
		count, err := s.countByTargetAndStatus(ctx, target, status)
		if err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func (s *DynamoDBExecutionStore) countByTargetAndStatus(ctx context.Context, target string, status Status) (int, error) {
	keyCond := expression.KeyAnd(
		expression.Key("targetCluster").Equal(expression.Value(target)),
		expression.Key("targetStatusKey").BeginsWith(string(status)+"#"),
	)

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return 0, fmt.Errorf("building query expression: %w", err)
	}

	var total int
	var lastKey map[string]types.AttributeValue
	for {
		input := &dynamodb.QueryInput{
			TableName:                 &s.tableName,
			IndexName:                 aws.String("target-status-index"),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			Select:                    types.SelectCount,
			ExclusiveStartKey:         lastKey,
		}
		out, err := s.client.Query(ctx, input)
		if err != nil {
			return 0, fmt.Errorf("querying active executions by target: %w", err)
		}
		total += int(out.Count)
		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}
	return total, nil
}

func (s *DynamoDBExecutionStore) QueryByTargetAndStatus(ctx context.Context, target string, status Status) ([]*Execution, error) {
	keyCond := expression.KeyAnd(
		expression.Key("targetCluster").Equal(expression.Value(target)),
		expression.Key("targetStatusKey").BeginsWith(string(status)+"#"),
	)

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return nil, fmt.Errorf("building target-status query expression: %w", err)
	}

	var all []*Execution
	var lastKey map[string]types.AttributeValue
	for {
		input := &dynamodb.QueryInput{
			TableName:                 &s.tableName,
			IndexName:                 aws.String("target-status-index"),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			ScanIndexForward:          aws.Bool(true),
			ExclusiveStartKey:         lastKey,
		}
		out, err := s.client.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("querying by target and status: %w", err)
		}
		items, err := unmarshalExecutions(out.Items)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}
	return all, nil
}

func (s *DynamoDBExecutionStore) QueryTerminalByTarget(ctx context.Context, target string, olderThan time.Duration) ([]*Execution, error) {
	threshold := time.Now().Add(-olderThan)
	var all []*Execution

	for _, status := range []Status{StatusSucceeded, StatusFailed, StatusTimedOut} {
		items, err := s.QueryByTargetAndStatus(ctx, target, status)
		if err != nil {
			return nil, fmt.Errorf("querying terminal %s by target: %w", status, err)
		}
		for _, exec := range items {
			if exec.Cleaned {
				continue
			}
			createdAt, err := exec.CreatedAtTime()
			if err != nil {
				continue
			}
			if createdAt.Before(threshold) {
				all = append(all, exec)
			}
		}
	}

	return all, nil
}

func unmarshalExecutions(items []map[string]types.AttributeValue) ([]*Execution, error) {
	executions := make([]*Execution, 0, len(items))
	for _, item := range items {
		var exec Execution
		if err := attributevalue.UnmarshalMap(item, &exec); err != nil {
			return nil, fmt.Errorf("unmarshaling execution: %w", err)
		}
		executions = append(executions, &exec)
	}
	return executions, nil
}
