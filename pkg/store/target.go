package store

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// SSMAPI is the interface for SSM operations used by the target store.
type SSMAPI interface {
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	GetParametersByPath(ctx context.Context, params *ssm.GetParametersByPathInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error)
}

// Target represents a ZOA Boundary target stored as an SSM parameter.
// Path: /zoa/targets/<deployment>/<targetId>
// Value: JSON-encoded target metadata.
type Target struct {
	TargetID          string `json:"target_id"`
	DeploymentName    string `json:"deployment_name"`
	VpcId             string `json:"vpc_id"`
	SubnetIds         string `json:"subnet_ids"`
	SecurityGroupId   string `json:"security_group_id"`
	EcsClusterArn     string `json:"ecs_cluster_arn"`
	TaskDefinitionArn string `json:"task_definition_arn"`
	FunctionUrl       string `json:"function_url"`
	AccountId         string `json:"account_id"`
	TargetType        string `json:"target_type"`
	Region            string `json:"region"`
	Status            string `json:"status"`
}

// TargetStore defines operations for boundary target discovery.
type TargetStore interface {
	Get(ctx context.Context, targetID string) (*Target, error)
	List(ctx context.Context) ([]*Target, error)
}

// SSMTargetStore implements TargetStore backed by SSM Parameter Store.
// Parameters are written by each cluster's Terraform and auto-removed on destroy.
type SSMTargetStore struct {
	client    SSMAPI
	ssmPrefix string // e.g., /zoa/targets/us-east-1
}

func NewTargetStore(client SSMAPI, ssmPrefix string) *SSMTargetStore {
	return &SSMTargetStore{
		client:    client,
		ssmPrefix: strings.TrimSuffix(ssmPrefix, "/"),
	}
}

func (s *SSMTargetStore) Get(ctx context.Context, targetID string) (*Target, error) {
	paramName := path.Join(s.ssmPrefix, targetID)
	out, err := s.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(paramName),
	})
	if err != nil {
		if strings.Contains(err.Error(), "ParameterNotFound") {
			return nil, nil
		}
		return nil, fmt.Errorf("getting target parameter %q: %w", paramName, err)
	}

	var target Target
	if err := json.Unmarshal([]byte(aws.ToString(out.Parameter.Value)), &target); err != nil {
		return nil, fmt.Errorf("unmarshaling target %q: %w", targetID, err)
	}
	if target.TargetID == "" {
		target.TargetID = targetID
	}
	return &target, nil
}

func (s *SSMTargetStore) List(ctx context.Context) ([]*Target, error) {
	prefix := s.ssmPrefix + "/"
	var targets []*Target

	input := &ssm.GetParametersByPathInput{
		Path:      aws.String(prefix),
		Recursive: aws.Bool(false),
	}

	for {
		out, err := s.client.GetParametersByPath(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("listing targets under %q: %w", prefix, err)
		}

		for _, param := range out.Parameters {
			var target Target
			if err := json.Unmarshal([]byte(aws.ToString(param.Value)), &target); err != nil {
				return nil, fmt.Errorf("unmarshaling target from %q: %w", aws.ToString(param.Name), err)
			}
			if target.TargetID == "" {
				name := aws.ToString(param.Name)
				target.TargetID = name[strings.LastIndex(name, "/")+1:]
			}
			targets = append(targets, &target)
		}

		if out.NextToken == nil {
			break
		}
		input.NextToken = out.NextToken
	}

	return targets, nil
}
