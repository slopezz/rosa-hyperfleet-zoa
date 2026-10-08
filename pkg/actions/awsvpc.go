package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func init() {
	Register(&listVPCEndpoints{})
	Register(&describeVPCEndpoint{})
}

type EC2Client interface {
	DescribeVpcEndpoints(ctx context.Context, params *ec2.DescribeVpcEndpointsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcEndpointsOutput, error)
}

type listVPCEndpoints struct{}

func (l *listVPCEndpoints) Metadata() ActionMetadata {
	return ActionMetadata{
		Name:              "list_vpc_endpoints",
		Scope:             "aws-api",
		Type:              "read",
		ExecutionMode:     "sync",
		Description:       "List all VPC endpoints in the configured AWS region.",
		Authorization:     AuthorizationConfig{Approval: "none"},
		DeploymentTargets: []string{DeploymentTargetRC, DeploymentTargetMC},
		TimeoutSeconds:    60,
		Examples: []string{
			"zoa run list_vpc_endpoints --reason ROSAENG-1234",
		},
		Parameters: []ParameterDef{},
	}
}

func (l *listVPCEndpoints) Validate(_ context.Context, params *ExecutionParams) error {
	if params.AWSConfig == nil {
		return fmt.Errorf("AWS configuration is required")
	}
	return nil
}

func (l *listVPCEndpoints) Execute(ctx context.Context, params *ExecutionParams) (*ActionResult, error) {
	client := ec2.NewFromConfig(*params.AWSConfig)
	return listEndpoints(ctx, client, params)
}

func listEndpoints(ctx context.Context, client EC2Client, params *ExecutionParams) (*ActionResult, error) {
	params.Logger.Info("listing VPC endpoints")

	var allEndpoints []map[string]interface{}
	var nextToken *string

	for {
		out, err := client.DescribeVpcEndpoints(ctx, &ec2.DescribeVpcEndpointsInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list VPC endpoints: %w", err)
		}
		for _, ep := range out.VpcEndpoints {
			allEndpoints = append(allEndpoints, compactEndpoint(ep))
		}
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return &ActionResult{
		Success: true,
		Output: map[string]interface{}{
			"endpoints": allEndpoints,
			"count":     len(allEndpoints),
		},
		Summary: fmt.Sprintf("Found %d VPC endpoints", len(allEndpoints)),
	}, nil
}

type describeVPCEndpoint struct{}

func (d *describeVPCEndpoint) Metadata() ActionMetadata {
	return ActionMetadata{
		Name:              "describe_vpc_endpoint",
		Scope:             "aws-api",
		Type:              "read",
		ExecutionMode:     "sync",
		Description:       "Describe a specific VPC endpoint by ID.",
		Authorization:     AuthorizationConfig{Approval: "none"},
		DeploymentTargets: []string{DeploymentTargetRC, DeploymentTargetMC},
		TimeoutSeconds:    60,
		Examples: []string{
			"zoa run describe_vpc_endpoint --name vpce-0123456789abcdef0 --reason ROSAENG-1234",
		},
		Parameters: []ParameterDef{
			{Name: "name", Required: true, Description: "VPC endpoint ID (e.g. vpce-0123456789abcdef0)"},
		},
	}
}

func (d *describeVPCEndpoint) Validate(_ context.Context, params *ExecutionParams) error {
	if params.AWSConfig == nil {
		return fmt.Errorf("AWS configuration is required")
	}
	name := params.Params["name"]
	if name == "" {
		return fmt.Errorf("parameter 'name' is required")
	}
	if !strings.HasPrefix(name, "vpce-") {
		return fmt.Errorf("invalid VPC endpoint ID %q — must start with 'vpce-'", name)
	}
	return nil
}

func (d *describeVPCEndpoint) Execute(ctx context.Context, params *ExecutionParams) (*ActionResult, error) {
	client := ec2.NewFromConfig(*params.AWSConfig)
	return describeEndpoint(ctx, client, params)
}

func describeEndpoint(ctx context.Context, client EC2Client, params *ExecutionParams) (*ActionResult, error) {
	endpointID := params.Params["name"]
	params.Logger.Info("describing VPC endpoint", "id", endpointID)

	out, err := client.DescribeVpcEndpoints(ctx, &ec2.DescribeVpcEndpointsInput{
		VpcEndpointIds: []string{endpointID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe VPC endpoint %q: %w", endpointID, err)
	}
	if len(out.VpcEndpoints) == 0 {
		return nil, fmt.Errorf("VPC endpoint %q not found", endpointID)
	}

	ep := out.VpcEndpoints[0]
	detail := map[string]interface{}{
		"id":                  vpcSafeStr(ep.VpcEndpointId),
		"vpcId":               vpcSafeStr(ep.VpcId),
		"serviceName":         vpcSafeStr(ep.ServiceName),
		"state":               string(ep.State),
		"type":                string(ep.VpcEndpointType),
		"subnetIds":           ep.SubnetIds,
		"routeTableIds":       ep.RouteTableIds,
		"networkInterfaceIds": ep.NetworkInterfaceIds,
		"securityGroupIds":    securityGroupIDs(ep.Groups),
		"privateDnsEnabled":   ep.PrivateDnsEnabled,
	}
	if ep.CreationTimestamp != nil {
		detail["createdAt"] = ep.CreationTimestamp.Format("2006-01-02T15:04:05Z")
	}

	tags := make(map[string]string, len(ep.Tags))
	for _, t := range ep.Tags {
		tags[vpcSafeStr(t.Key)] = vpcSafeStr(t.Value)
	}
	if len(tags) > 0 {
		detail["tags"] = tags
	}

	return &ActionResult{
		Success: true,
		Output:  detail,
		Summary: fmt.Sprintf("VPC endpoint %q: state=%s, service=%s", endpointID, ep.State, vpcSafeStr(ep.ServiceName)),
	}, nil
}

func compactEndpoint(ep ec2types.VpcEndpoint) map[string]interface{} {
	name := ""
	for _, t := range ep.Tags {
		if vpcSafeStr(t.Key) == "Name" {
			name = vpcSafeStr(t.Value)
			break
		}
	}
	return map[string]interface{}{
		"id":          vpcSafeStr(ep.VpcEndpointId),
		"serviceName": vpcSafeStr(ep.ServiceName),
		"vpcId":       vpcSafeStr(ep.VpcId),
		"state":       string(ep.State),
		"type":        string(ep.VpcEndpointType),
		"name":        name,
	}
}

func securityGroupIDs(groups []ec2types.SecurityGroupIdentifier) []string {
	ids := make([]string, len(groups))
	for i, g := range groups {
		ids[i] = vpcSafeStr(g.GroupId)
	}
	return ids
}

func vpcSafeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
