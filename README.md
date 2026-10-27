# kubectl-jfs-plugin

`kubectl-jfs` is a kubectl plugin for inspecting, debugging, and upgrading JuiceFS CSI workloads.
After installation, invoke it as `kubectl jfs`.

## Installation

```bash
curl -sSL https://d.juicefs.com/kubectl-jfs-install | sh -
```

Verify the installation:

```bash
kubectl jfs --help
kubectl jfs --version
```

The plugin uses the current kubeconfig and context by default. Standard kubectl flags such as
`--kubeconfig`, `--context`, and `--namespace` are supported.

If the JuiceFS CSI driver is not installed in `kube-system`, specify its namespace with
`-m` or `--mount-namespace`:

```bash
kubectl jfs -m juicefs-system mount
```

## Command overview

```bash
# List JuiceFS mount pods
kubectl jfs mount

# List application pods using JuiceFS volumes
kubectl jfs pod
kubectl jfs pod -A

# List JuiceFS PVCs and PVs
kubectl jfs pvc -A
kubectl jfs pv

# Show debugging information
kubectl jfs debug pod <pod-name> --namespace <namespace>
kubectl jfs debug pvc <pvc-name> --namespace <namespace>
kubectl jfs debug pv <pv-name>

# Warm up files in a pod
kubectl jfs warmup <pod-name>

# Collect JuiceFS access logs from a pod
kubectl jfs accesslog <pod-name>

# Upgrade one mount pod
kubectl jfs upgrade <mount-pod-name>
kubectl jfs upgrade <mount-pod-name> --recreate
```

Run `kubectl jfs <command> --help` for the complete options of a command.

## Batch upgrade

A batch upgrade creates a Kubernetes Job that upgrades mount pods or injected JuiceFS
sidecars. The recommended workflow is:

1. Preview the changes with `batch diff`.
2. Create the upgrade Job with `batch upgrade`.
3. Monitor the Job with `batch list` and `batch describe`.
4. Pause, resume, stop, or delete the Job when necessary.

### Preview mount pod changes

List all mount pods that need to be upgraded:

```bash
kubectl jfs batch diff
```

Show the configuration diff for one mount pod:

```bash
kubectl jfs batch diff <mount-pod-name>
```

Limit the preview to one node:

```bash
kubectl jfs batch diff --node <node-name>
```

### Upgrade mount pods

Upgrade all mount pods that need an update:

```bash
kubectl jfs batch upgrade
```

The command prints the planned changes and asks for confirmation before creating the Job.
Mount pods are recreated as part of a batch upgrade.

Common filters and options:

```bash
# Upgrade mount pods on one node
kubectl jfs batch upgrade --node <node-name>

# Upgrade mount pods associated with one PVC
kubectl jfs batch upgrade --pvc <pvc-name>

# Run up to five upgrades concurrently
kubectl jfs batch upgrade --worker 5

# Continue when an individual upgrade fails
kubectl jfs batch upgrade --ignore-error

# Skip the confirmation prompt
kubectl jfs batch upgrade --quiet
# Short form:
kubectl jfs batch upgrade -y

# Combine options
kubectl jfs batch upgrade --worker 5 --ignore-error -y
```

| Option | Default | Description |
| --- | --- | --- |
| `--node <name>` | all nodes | Upgrade only targets on the specified node. |
| `--pvc <name>` | all PVCs | Upgrade only mount pods associated with the specified PVC. |
| `--worker <number>` | `1` | Maximum number of concurrent upgrades. |
| `--ignore-error` | `false` | Continue after an individual target fails. |
| `-y`, `--quiet` | `false` | Skip the confirmation prompt. |

Targets already covered by a running batch upgrade Job are skipped automatically.

### Configure the dashboard image

The batch Job runs the `juicefs-csi-dashboard upgrade` command. By default, the plugin
reads its image and ServiceAccount from the `juicefs-csi-dashboard` Deployment in the
JuiceFS CSI driver namespace.

To override the image used by the batch Job, set `DASHBOARD_IMAGE` when running the
upgrade command:

```bash
DASHBOARD_IMAGE=juicedata/csi-dashboard:<version> \
  kubectl jfs batch upgrade
```

It is recommended to set the image and ServiceAccount together. When both variables are
set, the plugin does not need to read this configuration from the dashboard Deployment:

```bash
DASHBOARD_IMAGE=juicedata/csi-dashboard:<version> \
JUICEFS_CSI_DASHBOARD_SA=juicefs-csi-dashboard-sa \
  kubectl jfs batch upgrade
```

The same variables apply to sidecar batch upgrades:

```bash
DASHBOARD_IMAGE=juicedata/csi-dashboard:<version> \
JUICEFS_CSI_DASHBOARD_SA=juicefs-csi-dashboard-sa \
  kubectl jfs batch --sidecar upgrade --namespace <namespace>
```

| Environment variable | Description |
| --- | --- |
| `DASHBOARD_IMAGE` | Container image used by the batch upgrade Job. |
| `JUICEFS_CSI_DASHBOARD_SA` | ServiceAccount used by the batch upgrade Job. Defaults to `juicefs-csi-dashboard-sa` when it is not configured on the dashboard Deployment. |
| `BATCH_UPGRADE_TIMEOUT_SECONDS` | Batch upgrade timeout in seconds. The value must be an integer. |

For example, set a one-hour timeout with:

```bash
BATCH_UPGRADE_TIMEOUT_SECONDS=3600 kubectl jfs batch upgrade
```

If only one of `DASHBOARD_IMAGE` and `JUICEFS_CSI_DASHBOARD_SA` is set, the missing value
is resolved from the `juicefs-csi-dashboard` Deployment.

### Batch upgrade injected sidecars

Use the persistent `--sidecar` option on the `batch` command to operate on injected
JuiceFS sidecars. Put `--sidecar` before the batch subcommand in all examples for a
consistent command form:

```bash
kubectl jfs batch --sidecar <subcommand>
```

A namespace is required for sidecar upgrades.

Preview all sidecars that need to be upgraded in an application namespace:

```bash
kubectl jfs batch --sidecar diff --namespace <namespace>
```

Show the image and mount argument differences for sidecars in one pod:

```bash
kubectl jfs batch --sidecar diff <pod-name> --namespace <namespace>
```

Create a sidecar batch upgrade Job:

```bash
kubectl jfs batch --sidecar upgrade --namespace <namespace>
```

Control concurrency or limit the upgrade to one node:

```bash
kubectl jfs batch --sidecar upgrade \
  --namespace <namespace> \
  --node <node-name> \
  --worker 5
```

Skip confirmation and continue after individual failures:

```bash
kubectl jfs batch --sidecar upgrade \
  --namespace <namespace> \
  --worker 5 \
  --ignore-error \
  -y
```

Sidecar batch upgrades have the following constraints:

- `--namespace` is required.
- `--pvc` cannot be used with `--sidecar`.
- `--worker` must be greater than zero.
- Pods that are not Ready or are being deleted are skipped.
- Sidecars already covered by a running batch upgrade Job are skipped.

The plugin compares each sidecar with the state derived from its PVC, PV, Secret, and
Node configuration. Depending on the detected differences, the upgrade can update the
sidecar image, its mount arguments, or both. JuiceFS Community Edition sidecar mount
arguments are preserved during an image upgrade.

### Monitor and manage batch Jobs

List batch upgrade Jobs:

```bash
kubectl jfs batch list
```

List only sidecar batch upgrade Jobs:

```bash
kubectl jfs batch --sidecar list
```

Show details for a Job:

```bash
kubectl jfs batch describe <job-name>
```

Manage a Job:

```bash
kubectl jfs batch pause <job-name>
kubectl jfs batch resume <job-name>
kubectl jfs batch stop <job-name>
kubectl jfs batch delete <job-name>
```

A running Job must be paused or stopped before it can be deleted. A paused Job can be
resumed or deleted.

### Non-default CSI driver namespace

Use `-m` on batch commands when the JuiceFS CSI driver is installed outside
`kube-system`:

```bash
kubectl jfs -m juicefs-system batch diff
kubectl jfs -m juicefs-system batch upgrade
kubectl jfs -m juicefs-system batch describe <job-name>

kubectl jfs -m juicefs-system batch --sidecar upgrade \
  --namespace <application-namespace>
```
