package executor

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BreakglassKubeCluster describes API server endpoint material for kubeconfig wiring.
type BreakglassKubeCluster struct {
	Server                   string `json:"server"`
	CertificateAuthorityData string `json:"certificate_authority_data"`
	ServiceAccount           string `json:"service_account"`
	Namespace                string `json:"namespace"`
	RequestID                string `json:"request_id"`
}

// BreakglassKubeActivate prepares RBAC and returns cluster metadata for exec-based auth.
func (e *Executor) BreakglassKubeActivate(ctx context.Context, requestID, scopeID string) (*BreakglassKubeCluster, error) {
	saName, err := e.EnsureBreakglassKube(ctx, requestID, scopeID)
	if err != nil {
		return nil, err
	}
	if e.restConfig == nil {
		return nil, fmt.Errorf("EKS REST config not configured")
	}
	ca := ""
	if len(e.restConfig.CAData) > 0 {
		ca = base64.StdEncoding.EncodeToString(e.restConfig.CAData)
	}
	return &BreakglassKubeCluster{
		Server:                   e.restConfig.Host,
		CertificateAuthorityData: ca,
		ServiceAccount:           saName,
		Namespace:                jobNamespace,
		RequestID:                requestID,
	}, nil
}

// IssueBreakglassKubeToken returns a SA token for the grant (max 1h).
func (e *Executor) IssueBreakglassKubeToken(ctx context.Context, requestID, scopeID string, grantRemaining time.Duration) (string, time.Time, error) {
	saName, err := e.EnsureBreakglassKube(ctx, requestID, scopeID)
	if err != nil {
		return "", time.Time{}, err
	}
	secs := int64(grantRemaining.Seconds())
	if secs > 3600 {
		secs = 3600
	}
	if secs < 600 {
		secs = 600
	}
	tr, err := e.kubeClient.CoreV1().ServiceAccounts(jobNamespace).CreateToken(ctx, saName, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         []string{"https://kubernetes.default.svc"},
			ExpirationSeconds: &secs,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("creating break-glass token: %w", err)
	}
	exp := time.Now().Add(grantRemaining)
	if !tr.Status.ExpirationTimestamp.IsZero() {
		exp = tr.Status.ExpirationTimestamp.Time
	}
	return tr.Status.Token, exp, nil
}
