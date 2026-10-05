package actions

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/eks"
)

func init() {
	Register(&listEKSClusters{})
	Register(&describeEKSCluster{})
}

type EKSClient interface {
	ListClusters(ctx context.Context, params *eks.ListClustersInput, optFns ...func(*eks.Options)) (*eks.ListClustersOutput, error)
	DescribeCluster(ctx context.Context, params *eks.DescribeClusterInput, optFns ...func(*eks.Options)) (*eks.DescribeClusterOutput, error)
}

type listEKSClusters struct{}

func (l *listEKSClusters) Metadata() ActionMetadata {
	return ActionMetadata{
		Name:              "list_eks_clusters",
		Scope:             "aws-api",
		Type:              "read",
		ExecutionMode:     "sync",
		Description:       "List all EKS clusters in the configured AWS region.",
		Authorization:     AuthorizationConfig{Approval: "none"},
		DeploymentTargets: []string{DeploymentTargetRC, DeploymentTargetMC},
		TimeoutSeconds:    60,
		Examples: []string{
			"zoa run list_eks_clusters --jira ROSAENG-1234",
		},
		Parameters: []ParameterDef{},
	}
}

func (l *listEKSClusters) Validate(_ context.Context, params *ExecutionParams) error {
	if params.AWSConfig == nil {
		return fmt.Errorf("AWS configuration is required")
	}
	return nil
}

func (l *listEKSClusters) Execute(ctx context.Context, params *ExecutionParams) (*ActionResult, error) {
	client := eks.NewFromConfig(*params.AWSConfig)
	return listClusters(ctx, client, params)
}

func listClusters(ctx context.Context, client EKSClient, params *ExecutionParams) (*ActionResult, error) {
	params.Logger.Info("listing EKS clusters")

	var allClusters []string
	var nextToken *string

	for {
		out, err := client.ListClusters(ctx, &eks.ListClustersInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list EKS clusters: %w", err)
		}
		allClusters = append(allClusters, out.Clusters...)
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return &ActionResult{
		Success: true,
		Output: map[string]interface{}{
			"clusters": allClusters,
			"count":    len(allClusters),
		},
		Summary: fmt.Sprintf("Found %d EKS clusters", len(allClusters)),
	}, nil
}

type describeEKSCluster struct{}

func (d *describeEKSCluster) Metadata() ActionMetadata {
	return ActionMetadata{
		Name:              "describe_eks_cluster",
		Scope:             "aws-api",
		Type:              "read",
		ExecutionMode:     "sync",
		Description:       "Describe a specific EKS cluster.",
		Authorization:     AuthorizationConfig{Approval: "none"},
		DeploymentTargets: []string{DeploymentTargetRC, DeploymentTargetMC},
		TimeoutSeconds:    60,
		Examples: []string{
			"zoa run describe_eks_cluster --name my-eks-cluster --jira ROSAENG-1234",
		},
		Parameters: []ParameterDef{
			{Name: "name", Required: true, Description: "EKS cluster name"},
		},
	}
}

func (d *describeEKSCluster) Validate(_ context.Context, params *ExecutionParams) error {
	if params.AWSConfig == nil {
		return fmt.Errorf("AWS configuration is required")
	}
	if params.Params["name"] == "" {
		return fmt.Errorf("parameter 'name' is required")
	}
	return nil
}

func (d *describeEKSCluster) Execute(ctx context.Context, params *ExecutionParams) (*ActionResult, error) {
	client := eks.NewFromConfig(*params.AWSConfig)
	return describeCluster(ctx, client, params)
}

func describeCluster(ctx context.Context, client EKSClient, params *ExecutionParams) (*ActionResult, error) {
	name := params.Params["name"]
	params.Logger.Info("describing EKS cluster", "name", name)

	out, err := client.DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: &name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe EKS cluster %q: %w", name, err)
	}

	cluster := out.Cluster
	detail := map[string]interface{}{
		"name":            eksSafeStr(cluster.Name),
		"arn":             eksSafeStr(cluster.Arn),
		"status":          string(cluster.Status),
		"version":         eksSafeStr(cluster.Version),
		"platformVersion": eksSafeStr(cluster.PlatformVersion),
		"endpoint":        eksSafeStr(cluster.Endpoint),
		"roleArn":         eksSafeStr(cluster.RoleArn),
	}

	if cluster.KubernetesNetworkConfig != nil {
		detail["serviceIpv4Cidr"] = eksSafeStr(cluster.KubernetesNetworkConfig.ServiceIpv4Cidr)
	}
	if cluster.ResourcesVpcConfig != nil {
		detail["vpcId"] = eksSafeStr(cluster.ResourcesVpcConfig.VpcId)
		detail["subnetIds"] = cluster.ResourcesVpcConfig.SubnetIds
		detail["securityGroupIds"] = cluster.ResourcesVpcConfig.SecurityGroupIds
		detail["endpointPublicAccess"] = cluster.ResourcesVpcConfig.EndpointPublicAccess
		detail["endpointPrivateAccess"] = cluster.ResourcesVpcConfig.EndpointPrivateAccess
	}
	if cluster.CreatedAt != nil {
		detail["createdAt"] = cluster.CreatedAt.Format("2006-01-02T15:04:05Z")
	}

	return &ActionResult{
		Success: true,
		Output:  detail,
		Summary: fmt.Sprintf("EKS cluster %q: status=%s, version=%s", name, cluster.Status, eksSafeStr(cluster.Version)),
	}, nil
}

func eksSafeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
