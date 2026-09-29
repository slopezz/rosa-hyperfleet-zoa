package store

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Target represents a ZOA Boundary target in the boundary-targets DynamoDB table.
type Target struct {
	TargetID          string `json:"target_id" dynamodbav:"targetId"`
	DeploymentName    string `json:"deployment_name" dynamodbav:"deploymentName"`
	VpcId             string `json:"vpc_id" dynamodbav:"vpcId"`
	SubnetIds         string `json:"subnet_ids" dynamodbav:"subnetIds"`
	SecurityGroupId   string `json:"security_group_id" dynamodbav:"securityGroupId"`
	EcsClusterArn     string `json:"ecs_cluster_arn" dynamodbav:"ecsClusterArn"`
	TaskDefinitionArn string `json:"task_definition_arn" dynamodbav:"taskDefinitionArn"`
	FunctionUrl       string `json:"function_url" dynamodbav:"functionUrl"`
	AccountId         string `json:"account_id" dynamodbav:"accountId"`
	TargetType        string `json:"target_type" dynamodbav:"targetType"`
	Region            string `json:"region" dynamodbav:"region"`
	Status            string `json:"status" dynamodbav:"status"`
}

// TargetStore defines DynamoDB operations for boundary targets.
type TargetStore interface {
	Get(ctx context.Context, targetID string) (*Target, error)
	List(ctx context.Context) ([]*Target, error)
	ListByDeployment(ctx context.Context, deploymentName string) ([]*Target, error)
}

// DynamoDBTargetStore implements TargetStore backed by DynamoDB.
type DynamoDBTargetStore struct {
	client    DynamoDBAPI
	tableName string
}

func NewTargetStore(client DynamoDBAPI, tableName string) *DynamoDBTargetStore {
	return &DynamoDBTargetStore{
		client:    client,
		tableName: tableName,
	}
}

func (s *DynamoDBTargetStore) Get(ctx context.Context, targetID string) (*Target, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"targetId": &types.AttributeValueMemberS{Value: targetID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting target: %w", err)
	}
	if out.Item == nil {
		return nil, nil
	}

	var target Target
	if err := attributevalue.UnmarshalMap(out.Item, &target); err != nil {
		return nil, fmt.Errorf("unmarshaling target: %w", err)
	}
	return &target, nil
}

func (s *DynamoDBTargetStore) List(ctx context.Context) ([]*Target, error) {
	input := &dynamodb.ScanInput{
		TableName: &s.tableName,
	}

	out, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("scanning targets: %w", err)
	}

	targets := make([]*Target, 0, len(out.Items))
	for _, item := range out.Items {
		var target Target
		if err := attributevalue.UnmarshalMap(item, &target); err != nil {
			return nil, fmt.Errorf("unmarshaling target: %w", err)
		}
		targets = append(targets, &target)
	}
	return targets, nil
}

func (s *DynamoDBTargetStore) ListByDeployment(ctx context.Context, deploymentName string) ([]*Target, error) {
	filterCond := expression.Name("deploymentName").Equal(expression.Value(deploymentName))

	expr, err := expression.NewBuilder().WithFilter(filterCond).Build()
	if err != nil {
		return nil, fmt.Errorf("building deployment filter expression: %w", err)
	}

	input := &dynamodb.ScanInput{
		TableName:                 &s.tableName,
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	out, err := s.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("scanning targets by deployment: %w", err)
	}

	targets := make([]*Target, 0, len(out.Items))
	for _, item := range out.Items {
		var target Target
		if err := attributevalue.UnmarshalMap(item, &target); err != nil {
			return nil, fmt.Errorf("unmarshaling target: %w", err)
		}
		targets = append(targets, &target)
	}
	return targets, nil
}
