package store

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type mockSSMAPI struct {
	getParameterFn        func(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	getParametersByPathFn func(ctx context.Context, params *ssm.GetParametersByPathInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error)
}

func (m *mockSSMAPI) GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return m.getParameterFn(ctx, params, optFns...)
}

func (m *mockSSMAPI) GetParametersByPath(ctx context.Context, params *ssm.GetParametersByPathInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error) {
	return m.getParametersByPathFn(ctx, params, optFns...)
}

func TestSSMTargetStore_Get_WhenParameterExists_ItShouldReturnTarget(t *testing.T) {
	mock := &mockSSMAPI{
		getParameterFn: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			expected := "/zoa/targets/us-east-1/mc01"
			if aws.ToString(params.Name) != expected {
				t.Errorf("expected param name %q, got %q", expected, aws.ToString(params.Name))
			}
			return &ssm.GetParameterOutput{
				Parameter: &ssmtypes.Parameter{
					Name:  params.Name,
					Value: aws.String(`{"target_id":"mc01","deployment_name":"us-east-1","vpc_id":"vpc-abc123","target_type":"mc","region":"us-east-1","status":"ready"}`),
				},
			}, nil
		},
	}

	s := NewTargetStore(mock, "/zoa/targets/us-east-1")
	got, err := s.Get(context.Background(), "mc01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected target, got nil")
	}
	if got.TargetID != "mc01" {
		t.Errorf("expected targetID 'mc01', got %q", got.TargetID)
	}
	if got.TargetType != "mc" {
		t.Errorf("expected targetType 'mc', got %q", got.TargetType)
	}
	if got.VpcId != "vpc-abc123" {
		t.Errorf("expected vpcId 'vpc-abc123', got %q", got.VpcId)
	}
}

func TestSSMTargetStore_Get_WhenParameterNotFound_ItShouldReturnNil(t *testing.T) {
	mock := &mockSSMAPI{
		getParameterFn: func(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return nil, &ssmtypes.ParameterNotFound{Message: aws.String("ParameterNotFound")}
		},
	}

	s := NewTargetStore(mock, "/zoa/targets/us-east-1")
	got, err := s.Get(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestSSMTargetStore_List_WhenParametersExist_ItShouldReturnAllTargets(t *testing.T) {
	mock := &mockSSMAPI{
		getParametersByPathFn: func(_ context.Context, params *ssm.GetParametersByPathInput, _ ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error) {
			expected := "/zoa/targets/us-east-1/"
			if aws.ToString(params.Path) != expected {
				t.Errorf("expected path %q, got %q", expected, aws.ToString(params.Path))
			}
			return &ssm.GetParametersByPathOutput{
				Parameters: []ssmtypes.Parameter{
					{
						Name:  aws.String("/zoa/targets/us-east-1/rc"),
						Value: aws.String(`{"target_id":"rc","target_type":"rc","deployment_name":"us-east-1"}`),
					},
					{
						Name:  aws.String("/zoa/targets/us-east-1/mc01"),
						Value: aws.String(`{"target_id":"mc01","target_type":"mc","deployment_name":"us-east-1"}`),
					},
				},
			}, nil
		},
	}

	s := NewTargetStore(mock, "/zoa/targets/us-east-1")
	got, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(got))
	}
	if got[0].TargetID != "rc" {
		t.Errorf("expected first target 'rc', got %q", got[0].TargetID)
	}
	if got[1].TargetID != "mc01" {
		t.Errorf("expected second target 'mc01', got %q", got[1].TargetID)
	}
}

func TestSSMTargetStore_List_WhenEmpty_ItShouldReturnEmptySlice(t *testing.T) {
	mock := &mockSSMAPI{
		getParametersByPathFn: func(_ context.Context, _ *ssm.GetParametersByPathInput, _ ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error) {
			return &ssm.GetParametersByPathOutput{
				Parameters: []ssmtypes.Parameter{},
			}, nil
		},
	}

	s := NewTargetStore(mock, "/zoa/targets/us-east-1")
	got, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil && len(got) != 0 {
		t.Fatalf("expected empty slice, got %d targets", len(got))
	}
}

func TestSSMTargetStore_List_WhenPaginated_ItShouldReturnAllPages(t *testing.T) {
	callCount := 0
	mock := &mockSSMAPI{
		getParametersByPathFn: func(_ context.Context, params *ssm.GetParametersByPathInput, _ ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error) {
			callCount++
			if callCount == 1 {
				return &ssm.GetParametersByPathOutput{
					Parameters: []ssmtypes.Parameter{
						{
							Name:  aws.String("/zoa/targets/us-east-1/rc"),
							Value: aws.String(`{"target_id":"rc","target_type":"rc"}`),
						},
					},
					NextToken: aws.String("page2"),
				}, nil
			}
			return &ssm.GetParametersByPathOutput{
				Parameters: []ssmtypes.Parameter{
					{
						Name:  aws.String("/zoa/targets/us-east-1/mc01"),
						Value: aws.String(`{"target_id":"mc01","target_type":"mc"}`),
					},
				},
			}, nil
		},
	}

	s := NewTargetStore(mock, "/zoa/targets/us-east-1")
	got, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 targets across pages, got %d", len(got))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
}

func TestSSMTargetStore_Get_WhenTargetIDMissing_ItShouldInferFromPath(t *testing.T) {
	mock := &mockSSMAPI{
		getParameterFn: func(_ context.Context, params *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
			return &ssm.GetParameterOutput{
				Parameter: &ssmtypes.Parameter{
					Name:  params.Name,
					Value: aws.String(`{"target_type":"mc","vpc_id":"vpc-123"}`),
				},
			}, nil
		},
	}

	s := NewTargetStore(mock, "/zoa/targets/us-east-1")
	got, err := s.Get(context.Background(), "mc02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TargetID != "mc02" {
		t.Errorf("expected targetID inferred from path as 'mc02', got %q", got.TargetID)
	}
}
