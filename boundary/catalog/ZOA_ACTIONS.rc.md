# ZOA Trusted Actions catalog

**Deployment target:** `rc` (baked at image build from source registry).

Use `zoa describe <action>` inside the boundary for the live API view, or `zoa run` with the flags below.

## Quick rules

- Every `zoa run` **must** include `--jira TICKET`.
- Parameters map to CLI flags where listed; otherwise `--param name=value`.

## delete_pod

- **Scope:** kube-api · **Type:** write · **Mode:** sync
- **Also on:** rc, mc
- Delete a pod and wait for it to terminate. Refuses to delete standalone pods without owner references.

**Examples:**

```bash
zoa run delete_pod -n openshift-ingress --name router-default-abc12 --jira ROSAENG-1234
```

```bash
zoa run delete_pod -n cert-manager --name cert-manager-webhook-xyz --jira ROSAENG-1234 --dry-run
```

| Parameter | Required | CLI |
|-----------|----------|-----|
| `namespace` | yes | -n, --namespace |
| `name` | yes | --name |

## describe_eks_cluster

- **Scope:** aws-api · **Type:** read · **Mode:** sync
- **Also on:** rc, mc
- Describe a specific EKS cluster.

**Examples:**

```bash
zoa run describe_eks_cluster --name my-eks-cluster --jira ROSAENG-1234
```

| Parameter | Required | CLI |
|-----------|----------|-----|
| `name` | yes | --name |

## describe_vpc_endpoint

- **Scope:** aws-api · **Type:** read · **Mode:** sync
- **Also on:** rc, mc
- Describe a specific VPC endpoint by ID.

**Examples:**

```bash
zoa run describe_vpc_endpoint --name vpce-0123456789abcdef0 --jira ROSAENG-1234
```

| Parameter | Required | CLI |
|-----------|----------|-----|
| `name` | yes | --name |

## get_resource

- **Scope:** kube-api · **Type:** read · **Mode:** sync
- **Also on:** rc, mc
- Get or list Kubernetes resources by type, namespace, name, or label/field selectors (including CRDs). Does not support Secrets — use get_secret instead (HCP namespace protection).

**Examples:**

```bash
zoa run get_resource --resource pods -n openshift-ingress --jira ROSAENG-1234
```

```bash
zoa run get_resource --resource pods -A -l app=nginx --jira ROSAENG-1234
```

```bash
zoa run get_resource --resource nodes --jira ROSAENG-1234
```

```bash
zoa run get_resource --resource events --param field_selector=involvedObject.name=my-pod -n default --jira ROSAENG-1234
```

| Parameter | Required | CLI |
|-----------|----------|-----|
| `resource` | yes | --resource |
| `namespace` |  | -n, --namespace |
| `all_namespaces` |  | -A, --all-namespaces (sets true) |
| `name` |  | --name |
| `label_selector` |  | -l, --selector |
| `field_selector` |  | --param field_selector=... |
| `verbose` |  | -v, --verbose (sets true) |

## get_secret

- **Scope:** kube-api · **Type:** read · **Mode:** sync
- **Also on:** rc, mc
- Get Kubernetes secrets with HCP namespace protection. Shows secret metadata and data keys by default; use verbose for base64 values.

**Examples:**

```bash
zoa run get_secret -n openshift-config --name pull-secret --jira ROSAENG-1234
```

```bash
zoa run get_secret -n grafana -l app=grafana --jira ROSAENG-1234
```

| Parameter | Required | CLI |
|-----------|----------|-----|
| `namespace` | yes | -n, --namespace |
| `name` |  | --name |
| `label_selector` |  | -l, --selector |
| `verbose` |  | -v, --verbose (sets true) |

## list_eks_clusters

- **Scope:** aws-api · **Type:** read · **Mode:** sync
- **Also on:** rc, mc
- List all EKS clusters in the configured AWS region.

**Examples:**

```bash
zoa run list_eks_clusters --jira ROSAENG-1234
```

## list_vpc_endpoints

- **Scope:** aws-api · **Type:** read · **Mode:** sync
- **Also on:** rc, mc
- List all VPC endpoints in the configured AWS region.

**Examples:**

```bash
zoa run list_vpc_endpoints --jira ROSAENG-1234
```

## must_gather

- **Scope:** kube-api · **Type:** read · **Mode:** async
- **Also on:** rc, mc
- Collect must-gather bundle (logs, events, resources) for hosted cluster, MC platform, or RC platform. MC ZOA: gather mc|hcp (hcp needs cluster-id). RC ZOA: gather rc.

**Examples:**

```bash
zoa run must_gather --gather mc --jira ROSAENG-1234 --wait
```

```bash
zoa run must_gather --gather hcp --cluster-id 01234567-89ab-cdef-0123-456789abcdef --jira ROSAENG-1234 --wait
```

```bash
zoa run must_gather --gather rc --jira ROSAENG-1234 --wait
```

| Parameter | Required | CLI |
|-----------|----------|-----|
| `gather` | yes | --gather |
| `cluster_id` |  | --cluster-id |
| `extra_namespaces` |  | --param extra_namespaces=... |
| `skip_must_gather_image` |  | --param skip_must_gather_image=... |

## rollout_restart

- **Scope:** kube-api · **Type:** write · **Mode:** sync
- **Also on:** rc, mc
- Restart a workload by patching the pod template annotation, equivalent to kubectl rollout restart. Supports deployments, daemonsets, and statefulsets.

**Examples:**

```bash
zoa run rollout_restart --resource deployment -n openshift-ingress --name router-default --jira ROSAENG-1234
```

```bash
zoa run rollout_restart --resource deployment -n cert-manager --name cert-manager-webhook --jira ROSAENG-1234 --dry-run
```

| Parameter | Required | CLI |
|-----------|----------|-----|
| `resource` | yes | --resource |
| `namespace` | yes | -n, --namespace |
| `name` | yes | --name |

