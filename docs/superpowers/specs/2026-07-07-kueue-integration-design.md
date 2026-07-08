# Kueue Integration — Design

**Date:** 2026-07-07
**Scope:** Add per-cluster `queue_mode` config (`"channel"` | `"kueue"`). When `"kueue"`, the judger creates K8s Jobs (instead of bare Pods) with Kueue queue-name labels. Kueue's admission controller handles queuing, priority, fair sharing, and preemption. The in-process channel + semaphore is bypassed. The `"channel"` mode (current behavior) remains the default.

## Goals

1. **`Cluster.QueueMode` DB field** — `"channel"` (default) or `"kueue"`. Admin-configurable per cluster.
2. **Kueue mode**: `Submit` skips the in-process channel + semaphore; `Dispatcher` creates a `batchv1.Job` (not a Pod) with `kueue.x-k8s.io/queue-name` label; `KubeManager` watches Job status.
3. **Channel mode** (unchanged): in-process Go channel + per-cluster semaphore + `clusterWorker` + bare Pod creation.
4. **Kueue CRD probe**: at boot, probe `workloads.kueue.x-k8s.io`. If a cluster has `queue_mode="kueue"` but the CRD is absent, log a warning and fall back to `"channel"`.
5. **Recovery**: delete `app=csoj-judger` Jobs (in addition to Pods + MPIJobs).
6. **Frontend**: cluster form gains a `queue_mode` Select; cluster status shows the mode.

## Non-goals

- Kueue operator installation (cluster-admin responsibility).
- Kueue `LocalQueue`/`ClusterQueue` provisioning (cluster-admin sets these up; the judger just references the queue name).
- WorkloadPriorityClass creation (the admin creates these in K8s; the judger references via per-step `priority_class_name`).
- Changing the MPI flow (MPIJob CRD already works with Kueue).

---

## Data model

### `models.Cluster` gains `QueueMode`

```go
type Cluster struct {
    // ... existing ...
    QueueMode string `gorm:"default:channel" json:"queue_mode"` // "channel" | "kueue"
}
```

### `ClusterState` (in-memory) gains `queueMode`

```go
type ClusterState struct {
    // ... existing ...
    queueMode string // "channel" | "kueue"
}
```

`buildClusterState` reads `cc.QueueMode` and probes the Kueue CRD. If `queueMode == "kueue"` but CRD absent → log warning + set `queueMode = "channel"`.

---

## Job spec builder (`internal/judger/podspec/mpijob.go`)

New `buildJobSpec`:
```go
type JobSpecInput struct {
    PodSpecInput        // reuse all existing fields
    QueueName    string // Kueue LocalQueue name
}

func BuildJobSpec(in JobSpecInput) *batchv1.Job {
    pod := BuildPodSpec(in.PodSpecInput)
    return &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      in.Name,
            Namespace: in.Namespace,
            Labels:    pod.Labels, // app=csoj-judger, csoj-submission, csoj-step
        },
        Spec: batchv1.JobSpec{
            Template: corev1.PodTemplateSpec{
                ObjectMeta: pod.ObjectMeta,
                Spec:       pod.Spec,
            },
            BackoffLimit:            ptrInt32(0),
            TTLSecondsAfterFinished: ptrInt32(60),
            ActiveDeadlineSeconds:   ptrInt64(in.TimeoutSec),
        },
    }
}
```

The Job's pod template inherits all resources, scheduling constraints, env, volumes, security context from the existing `BuildPodSpec`.

---

## KubeManager (`internal/judger/k8s.go`)

New methods:
```go
func (k *KubeManager) CreateJob(ctx, job *batchv1.Job) error
func (k *KubeManager) WaitForJob(ctx, jobName string) (batchv1.JobConditionType, error)
func (k *KubeManager) DeleteJob(ctx, jobName string) error
func (k *KubeManager) StreamJobLogs(ctx, jobName string, onChunk func(string)) error
func (k *KubeManager) DeleteSubmissionJobs(ctx, subID string) error
func (k *KubeManager) DeleteAllJudgerJobs(ctx) error
```

- `CreateJob`: `k.cs.BatchV1().Jobs(k.ns).Create(...)`
- `WaitForJob`: poll `.status.conditions` for `Complete=true` or `Failed=true`
- `StreamJobLogs`: find the Job's pod (label `job-name=<jobName>`), then `StreamPodLogs` on it
- `DeleteSubmissionJobs`: `DeleteCollection` with label `csoj-submission=<subID>`
- `DeleteAllJudgerJobs`: `DeleteCollection` with label `app=csoj-judger`
- `ProbeKueue`: `cs.Discovery().ServerResourcesForGroupVersion("kueue.x-k8s.io/v1")` — check for `workloads` kind

---

## Scheduler (`internal/judger/scheduler.go`)

### `Submit` branches

```go
func (s *Scheduler) Submit(submission *models.Submission, problem *Problem) {
    // ... cluster lookup ...
    if cluster.queueMode == "kueue" {
        submission.Node = ""
        submission.Status = models.StatusRunning
        database.UpdateSubmission(s.db, submission)
        go s.dispatcher.Dispatch(submission, problem, cluster, nil)
        return
    }
    // channel mode (existing)
    cluster.queue <- QueuedSubmission{...}
}
```

`clusterWorker` unchanged — only handles channel-mode submissions.

### `buildClusterState`

Reads `cc.QueueMode`. If `"kueue"`, probe CRD; if absent, fall back to `"channel"` + warn.

### `GetClusterStates`

`ClusterStateSnapshot` gains `QueueMode string`.

---

## Dispatcher (`internal/judger/dispatcher.go`)

### `runPodStep` branches

```go
func (d *Dispatcher) runPodStep(...) error {
    if cluster.queueMode == "kueue" {
        return d.runJobStep(...)
    }
    // existing Pod flow
}
```

### `runJobStep`

1. `podspec.BuildJobSpec(JobSpecInput{PodSpecInput: ..., QueueName: cluster.Name + "-queue"})`
2. `km.CreateJob(ctx, job)`
3. goroutine: `km.StreamJobLogs(ctx, jobName, onChunk)` → pubsub
4. `km.WaitForJob(ctx, jobName)` → Complete/Failed
5. Read `result.json`
6. `km.DeleteJob(ctx, jobName)` (cleanup, though TTL also handles it)

### `Dispatch` defer cleanup

In Kueue mode: `km.DeleteSubmissionJobs(ctx, sub.ID)` instead of `km.DeleteSubmissionPods`. `ReleaseSlot` not called (semaphore not acquired).

### MPI steps in Kueue mode

MPIJob CRD with `metadata.labels["kueue.x-k8s.io/queue-name"] = cluster.Name + "-queue"`. Kueue supports MPIJob admission. No code change in `buildMPIJobSpec` except adding the label.

---

## Recovery (`internal/judger/recovery.go`)

`RecoverAndCleanup` + `DeleteAllJudgerResources`:
- Existing: `DeleteAllJudgerPods` + `DeleteAllJudgerMPIJobs`
- New: `DeleteAllJudgerJobs` (label `app=csoj-judger`)

---

## Frontend

### Types

`ClusterRow` gains `queue_mode: string`.
`ClusterStateSnapshot` gains `QueueMode: string`.

### Cluster form (`ClusterFormDialog`)

New `queue_mode` Select: `channel` / `kueue`. When `kueue` selected, show a hint: "Requires Kueue operator. Jobs use queue name `<name>-queue`."

### Cluster status

`PoolStatusSection` shows the cluster's `QueueMode` badge.

---

## Testing

- **Unit**: `BuildJobSpec` test — verify labels, backoffLimit, TTL, pod template.
- **Manual**: create a cluster with `queue_mode="kueue"` (fake kubeconfig), reload, verify no crash + warning if CRD absent. With a real Kueue cluster: submit a problem, verify a Job is created, Kueue admits it, result.json is read, Job is cleaned up.

## Out of scope

- Kueue operator installation.
- LocalQueue/ClusterQueue provisioning.
- WorkloadPriorityClass creation.
- Changing MPI flow.
- Kueue preemption configuration.
