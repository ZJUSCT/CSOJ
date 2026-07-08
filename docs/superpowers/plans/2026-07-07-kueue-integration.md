# Kueue Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-cluster `queue_mode` config (`"channel"` | `"kueue"`). When `"kueue"`, the judger creates K8s Jobs with Kueue labels instead of bare Pods, bypassing the in-process channel + semaphore.

**Architecture:** `Cluster.QueueMode` DB field selects the mode. `BuildJobSpec` in podspec wraps the existing `BuildPodSpec` into a `batchv1.Job`. `KubeManager` gains Job lifecycle methods. `Dispatcher.runPodStep` branches on `cluster.queueMode`. `Submit` skips the channel for Kueue mode. Recovery deletes Jobs. Frontend gains a `queue_mode` Select.

**Tech Stack:** Go 1.24, k8s.io/api (batchv1), k8s.io/client-go, Next.js 14.

**Reference spec:** `docs/superpowers/specs/2026-07-07-kueue-integration-design.md`

**Branch:** `merge-webui-admin`.

---

## Task 1: Backend — Cluster.QueueMode + CRD probe + ClusterState.queueMode

**Files:**
- Modify: `internal/database/models/models.go`
- Modify: `internal/judger/scheduler.go`
- Modify: `internal/judger/k8s.go`

- [ ] **Step 1: Add `QueueMode` to `models.Cluster`**

In `internal/database/models/models.go`, in the `Cluster` struct, add after `HeartbeatTTL`:
```go
	QueueMode  string    `gorm:"default:channel" json:"queue_mode"`
```

- [ ] **Step 2: Add `queueMode` to `ClusterState` + `ClusterStateSnapshot`**

In `internal/judger/scheduler.go`:
- In `ClusterState`, add: `queueMode string`
- In `ClusterStateSnapshot`, add: `QueueMode string`
- In `GetClusterStates`, set `QueueMode: c.queueMode` in the snapshot.

- [ ] **Step 3: Read `QueueMode` in `buildClusterState` + CRD probe**

In `internal/judger/scheduler.go`, in `buildClusterState`, after building the `ClusterState`:
```go
	cluster.queueMode = cc.QueueMode
	if cluster.queueMode == "kueue" {
		if !probeKueue(cs) {
			zap.S().Warnf("cluster %s: queue_mode=kueue but Kueue CRD not found; falling back to channel", cc.Name)
			cluster.queueMode = "channel"
		}
	}
```

- [ ] **Step 4: Add `probeKueue` to `k8s.go`**

In `internal/judger/k8s.go`, add:
```go
func probeKueue(cs kubernetes.Interface) bool {
	apiRes, err := cs.Discovery().ServerResourcesForGroupVersion("kueue.x-k8s.io/v1")
	if err != nil {
		return false
	}
	for _, r := range apiRes.APIResources {
		if r.Kind == "Workload" {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Update `ReloadClusters` to read `QueueMode`**

In `scheduler.go`, in `ReloadClusters`, the `buildClusterState` call already reads `cc.QueueMode` (since `buildClusterState` was updated in Step 3). No additional change needed — verify this.

- [ ] **Step 6: Verify**

Run: `go build ./internal/database/... ./internal/judger/`
Expected: exit 0.

- [ ] **Step 7: Commit**

```bash
git add internal/database/models/models.go internal/judger/scheduler.go internal/judger/k8s.go
git commit -m "feat(db): Cluster.QueueMode + Kueue CRD probe + ClusterState.queueMode"
```

---

## Task 2: Podspec — `BuildJobSpec` + tests

**Files:**
- Modify: `internal/judger/podspec/mpijob.go`
- Modify: `internal/judger/podspec/mpijob_test.go`

- [ ] **Step 1: Add `JobSpecInput` + `BuildJobSpec` + `ptrInt32`**

In `internal/judger/podspec/mpijob.go`, add:
```go
import batchv1 "k8s.io/api/batch/v1"

type JobSpecInput struct {
	PodSpecInput
	QueueName string
}

func BuildJobSpec(in JobSpecInput) *batchv1.Job {
	pod := BuildPodSpec(in.PodSpecInput)
	labels := make(map[string]string)
	for k, v := range pod.Labels {
		labels[k] = v
	}
	if in.QueueName != "" {
		labels["kueue.x-k8s.io/queue-name"] = in.QueueName
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.Name,
			Namespace: in.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: pod.Labels,
				},
				Spec: pod.Spec,
			},
			BackoffLimit:            ptrInt32(0),
			TTLSecondsAfterFinished: ptrInt32(60),
			ActiveDeadlineSeconds:   ptrInt64(in.TimeoutSec),
		},
	}
}

func ptrInt32(v int32) *int32 { return &v }
```

- [ ] **Step 2: Add test**

In `internal/judger/podspec/mpijob_test.go`, add:
```go
func TestBuildJobSpec_Basic(t *testing.T) {
	job := BuildJobSpec(JobSpecInput{
		PodSpecInput: PodSpecInput{
			Name: "sub1-0", Namespace: "csoj-judger", Image: "gcc:13",
			Script: "echo hi", CPURequest: "1", CPULimit: "1",
			MemoryRequest: "256Mi", MemoryLimit: "256Mi",
			SubID: "sub1", Step: 0, TimeoutSec: 30,
		},
		QueueName: "test-queue",
	})
	if job.Name != "sub1-0" {
		t.Errorf("name: %s", job.Name)
	}
	if job.Labels["kueue.x-k8s.io/queue-name"] != "test-queue" {
		t.Errorf("queue label: %v", job.Labels)
	}
	if job.Labels["app"] != "csoj-judger" {
		t.Errorf("app label: %v", job.Labels)
	}
	if *job.Spec.BackoffLimit != 0 {
		t.Errorf("backoffLimit: %d", *job.Spec.BackoffLimit)
	}
	if *job.Spec.TTLSecondsAfterFinished != 60 {
		t.Errorf("ttl: %d", *job.Spec.TTLSecondsAfterFinished)
	}
	if job.Spec.Template.Spec.Containers[0].Image != "gcc:13" {
		t.Errorf("image: %s", job.Spec.Template.Spec.Containers[0].Image)
	}
}
```

- [ ] **Step 3: Verify**

Run: `go test ./internal/judger/podspec/ -v -run TestBuildJobSpec`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/judger/podspec/mpijob.go internal/judger/podspec/mpijob_test.go
git commit -m "feat(podspec): BuildJobSpec wraps PodSpec into a batchv1.Job"
```

---

## Task 3: KubeManager — Job lifecycle methods

**Files:**
- Modify: `internal/judger/k8s.go`

- [ ] **Step 1: Add Job methods**

In `internal/judger/k8s.go`, add the `batchv1` import and these methods:

```go
func (k *KubeManager) CreateJob(ctx context.Context, job *batchv1.Job) error {
	_, err := k.cs.BatchV1().Jobs(k.ns).Create(ctx, job, metav1.CreateOptions{})
	return err
}

func (k *KubeManager) WaitForJob(ctx context.Context, jobName string) (batchv1.JobConditionType, error) {
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		job, err := k.cs.BatchV1().Jobs(k.ns).Get(ctx, jobName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		for _, cond := range job.Status.Conditions {
			if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
				return batchv1.JobComplete, nil
			}
			if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
				return batchv1.JobFailed, nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (k *KubeManager) StreamJobLogs(ctx context.Context, jobName string, onChunk func(string)) error {
	// Find the pod created by this job
	pods, err := k.cs.CoreV1().Pods(k.ns).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil || len(pods.Items) == 0 {
		return fmt.Errorf("no pods found for job %s", jobName)
	}
	return k.StreamPodLogs(ctx, pods.Items[0].Name, onChunk)
}

func (k *KubeManager) DeleteJob(ctx context.Context, jobName string) error {
	return k.cs.BatchV1().Jobs(k.ns).Delete(ctx, jobName, metav1.DeleteOptions{
		GracePeriodSeconds: ptrInt64(0),
	})
}

func (k *KubeManager) DeleteSubmissionJobs(ctx context.Context, subID string) error {
	return k.cs.BatchV1().Jobs(k.ns).DeleteCollection(ctx, metav1.DeleteOptions{
		GracePeriodSeconds: ptrInt64(0),
	}, metav1.ListOptions{LabelSelector: fmt.Sprintf("csoj-submission=%s", subID)})
}

func (k *KubeManager) DeleteAllJudgerJobs(ctx context.Context) error {
	return k.cs.BatchV1().Jobs(k.ns).DeleteCollection(ctx, metav1.DeleteOptions{
		GracePeriodSeconds: ptrInt64(0),
	}, metav1.ListOptions{LabelSelector: "app=csoj-judger"})
}
```

Add `"k8s.io/api/batch/v1"` to imports (as `batchv1`).

- [ ] **Step 2: Verify**

Run: `go build ./internal/judger/`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/judger/k8s.go
git commit -m "feat(judger): KubeManager Job lifecycle methods"
```

---

## Task 4: Dispatcher + Scheduler — Kueue mode flow

**Files:**
- Modify: `internal/judger/dispatcher.go`
- Modify: `internal/judger/scheduler.go`

- [ ] **Step 1: Add `runJobStep` to dispatcher**

In `internal/judger/dispatcher.go`, add after `runPodStep`:
```go
func (d *Dispatcher) runJobStep(km *KubeManager, sub *models.Submission, prob *Problem, flow WorkflowStep, cluster *ClusterState, env []corev1.EnvVar, step int) error {
	jobName := fmt.Sprintf("%s-%d", sub.ID, step)
	script := podspec.GenerateEntrypointScript(flow.Steps)

	res := flow.Resources
	sched := flow.Scheduling
	cpuReq, cpuLim, memReq, memLim := stepResourceDefaults(res)
	var nodeAff []podspec.NodeAffinityTerm
	var tols []podspec.Toleration
	var priorityClass, runtimeClass string
	if sched != nil {
		for _, a := range sched.NodeAffinity {
			nodeAff = append(nodeAff, podspec.NodeAffinityTerm{Key: a.Key, Operator: a.Operator, Values: a.Values})
		}
		for _, t := range sched.Tolerations {
			tols = append(tols, podspec.Toleration{Key: t.Key, Operator: t.Operator, Value: t.Value, Effect: t.Effect})
		}
		priorityClass = sched.PriorityClassName
		runtimeClass = sched.RuntimeClassName
	}

	job := podspec.BuildJobSpec(podspec.JobSpecInput{
		PodSpecInput: podspec.PodSpecInput{
			Name: jobName, Namespace: km.ns, Image: flow.Image, Script: script,
			CPURequest: cpuReq, CPULimit: cpuLim, MemoryRequest: memReq, MemoryLimit: memLim,
			NodeSel: nil, // Kueue handles scheduling
			NodeAffinity: nodeAff, Tolerations: tols,
			PriorityClassName: priorityClass, RuntimeClassName: runtimeClass,
			Env: env, SubID: sub.ID, Step: step, AsRoot: flow.Root, Network: flow.Network,
			TimeoutSec: int64(flow.Timeout),
		},
		QueueName: cluster.Name + "-queue",
	})

	cont := d.newContainerRecord(sub, flow.Image, step)
	cont.PodName = jobName
	database.CreateContainer(d.db, cont)
	defer pubsub.GetBroker().CloseTopic(cont.ID)

	ctx, cancel := context.WithTimeout(context.Background(), durationWithDefault(flow.Timeout, 30*60))
	defer cancel()

	if err := km.CreateJob(ctx, job); err != nil {
		d.failContainer(cont, -1, fmt.Sprintf("failed to create job: %v", err))
		return err
	}

	logCtx, logCancel := context.WithCancel(ctx)
	go func() {
		_ = km.StreamJobLogs(logCtx, jobName, func(chunk string) {
			pubsub.GetBroker().Publish(cont.ID, pubsub.FormatMessage("stdout", chunk))
		})
	}()

	cond, err := km.WaitForJob(ctx, jobName)
	logCancel()
	if err != nil && cond == "" {
		d.failContainer(cont, -1, fmt.Sprintf("timeout/error: %v", err))
		return err
	}
	if cond == batchv1.JobFailed {
		d.failContainer(cont, -1, "job failed")
		return fmt.Errorf("job %s failed", jobName)
	}
	cont.Status = models.StatusSuccess
	cont.FinishedAt = time.Now()
	database.UpdateContainer(d.db, cont)
	return nil
}
```

Add `"k8s.io/api/batch/v1"` import (as `batchv1`).

- [ ] **Step 2: Branch in `runPodStep`**

In `internal/judger/dispatcher.go`, in `runPodStep`, add at the top:
```go
	if cluster.queueMode == "kueue" {
		return d.runJobStep(km, sub, prob, flow, cluster, env, step)
	}
```

The `runPodStep` signature needs `cluster *ClusterState` added (currently it takes `pool *PoolState` but not `cluster`). Read the current signature and add `cluster *ClusterState` as a parameter. Update the call site in `Dispatch`.

- [ ] **Step 3: Update `Dispatch` defer for Kueue mode**

In `Dispatch`'s defer, add:
```go
		_ = km.DeleteSubmissionJobs(ctx, sub.ID)
```
And skip `ReleaseSlot` when `cluster.queueMode == "kueue"`:
```go
	if cluster.queueMode != "kueue" {
		d.scheduler.ReleaseSlot(cluster.Name)
	}
```

- [ ] **Step 4: Update `Submit` for Kueue mode**

In `internal/judger/scheduler.go`, in `Submit`, add before the channel write:
```go
	if cluster.queueMode == "kueue" {
		submission.Node = ""
		submission.Status = models.StatusRunning
		database.UpdateSubmission(s.db, submission)
		go s.dispatcher.Dispatch(submission, problem, cluster, nil)
		return
	}
```

- [ ] **Step 5: Verify**

Run: `go build ./...`
Expected: exit 0.

Run: `go vet ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add internal/judger/dispatcher.go internal/judger/scheduler.go
git commit -m "feat(judger): Kueue mode — Job creation + Submit bypass + cleanup"
```

---

## Task 5: Recovery — delete Jobs

**Files:**
- Modify: `internal/judger/recovery.go`

- [ ] **Step 1: Add Job deletion to recovery**

In `internal/judger/recovery.go`, in the cluster cleanup loop, add after `km.DeleteAllJudgerMPIJobs`:
```go
			if err := km.DeleteAllJudgerJobs(ctx); err != nil {
				zap.S().Warnf("cluster %s: failed to delete judger Jobs: %v", cc.Name, err)
			}
```

- [ ] **Step 2: Verify**

Run: `go build ./...`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/judger/recovery.go
git commit -m "feat(judger): recovery deletes Kueue Jobs"
```

---

## Task 6: Frontend — types + cluster form + status

**Files:**
- Modify: `frontend/lib/types.ts`
- Modify: `frontend/app/(main)/admin/cluster/page.tsx`

- [ ] **Step 1: Update types**

In `frontend/lib/types.ts`:
- `ClusterRow` gains `queue_mode: string`
- `ClusterStateSnapshot` gains `QueueMode: string`

- [ ] **Step 2: Update cluster form**

In `frontend/app/(main)/admin/cluster/page.tsx`, in the `ClusterFormDialog`:
- Add `queue_mode` to the form state (default `"channel"`)
- Add a Select: `channel` / `kueue`
- When `kueue` selected, show a hint: "Requires Kueue operator. Jobs use queue name `{name}-queue`."
- Include `queue_mode` in the create/update payload.

- [ ] **Step 3: Update cluster status display**

In the `PoolStatusSection`, show the cluster's `QueueMode` badge (e.g., "Channel" or "Kueue").

- [ ] **Step 4: Verify**

Run: `cd frontend && pnpm build`
Expected: 19 pages, succeeds.

- [ ] **Step 5: Commit**

```bash
git add frontend/lib/types.ts "frontend/app/(main)/admin/cluster/page.tsx"
git commit -m "feat(frontend): cluster queue_mode select + status badge"
```

---

## Task 7: Build + verify

- [ ] **Step 1: Full build**

Run: `go build ./... && go vet ./... && cd frontend && pnpm build`
Expected: Go clean, 19 pages.

Run: `make build`
Expected: binary produced.

- [ ] **Step 2: Manual smoke test**

Create a cluster with `queue_mode="kueue"` (fake kubeconfig). Reload clusters. Verify the CRD probe logs a warning and falls back to `channel`. Verify the admin UI shows the `queue_mode` select + badge.

- [ ] **Step 3: No commit**

---

## Self-Review notes

- `runPodStep` currently takes `(km, sub, prob, flow, pool, env, step)` — it needs `cluster *ClusterState` added so it can check `cluster.queueMode`. Update the call site in `Dispatch` to pass `cluster`.
- `Dispatch` currently receives `cluster *ClusterState` — it's already available. The `runPodStep` call just needs to pass it through.
- `runJobStep` does NOT call `findAvailablePool` — Kueue handles scheduling. `pool` is `nil` in Kueue mode (the `Dispatch` call from `Submit` passes `nil`).
- The `ptrInt32` function is defined in `mpijob.go` (podspec package). The `ptrInt64` is already defined in `k8s.go` (judger package). No conflict.
- `stepResourceDefaults` is already defined in `dispatcher.go` (added in the per-step resources task). Reuse it.
- The `batchv1` import alias must be consistent: `batchv1 "k8s.io/api/batch/v1"`.
- `runMPIStep` does NOT branch on `queueMode` — MPIJob CRDs already work with Kueue (Kueue supports MPIJob admission). The MPIJob already gets a `kueue.x-k8s.io/queue-name` label if added. This is out of scope for this plan (MPI + Kueue is a follow-up).
- `durationWithDefault` is already defined in `dispatcher.go`.
