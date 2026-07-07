# Per-Step Resources + Scheduling Constraints — Design

**Date:** 2026-07-07
**Scope:** Move CPU/memory resources and K8s scheduling constraints (nodeSelector, nodeAffinity, tolerations, priorityClassName, runtimeClassName) from the problem level to each workflow step. Each step independently controls its own Pod's resources and scheduling. Remove the problem-level `cpu`/`memory` fields entirely.

## Goals

1. **Per-step `StepResources`** — each workflow step specifies its own CPU request/limit + memory request/limit as K8s resource strings (`"2"`, `"500m"`, `"1Gi"`, `"256Mi"`). Empty fields default to `"1"` CPU / `"256Mi"` memory. Separate request and limit allow both Guaranteed QoS (request==limit, for CPU Manager static pinning) and Burstable QoS (request<limit).
2. **Per-step `StepScheduling`** — each step specifies nodeSelector overrides, nodeAffinity terms, tolerations, priorityClassName, and runtimeClassName. Empty → inherits the pool's nodeSelector (current behavior).
3. **Remove `Problem.CPU` and `Problem.Memory`** — these fields no longer exist. The problem form no longer has problem-level CPU/memory inputs; each step's Resources panel is the only place to set them.
4. **Frontend structured editor** — the workflow step editor gains a "Resources" panel (4 fields + "Make Guaranteed" button) and a "Scheduling" panel (nodeSelector key-value pairs, affinity terms, tolerations, priorityClass, runtimeClass) inside each step card.

## Non-goals

- kubelet-level config (cpuManagerPolicy, topologyManagerPolicy) — node-level, not per-Pod. Admin controls this via node setup + RuntimeClass.
- scheduler profiles (binpack/spread) — cluster-level.
- LimitRange / ResourceQuota — namespace-level.
- Pod-level resources (K8s 1.34+ beta) — use container-level (more universal).
- Migration of existing problems (fresh start; steps without `resources` use defaults).

---

## Data model

### Go (`internal/judger/loader.go`)

`WorkflowStep` gains two optional fields:
```go
type WorkflowStep struct {
    // ... existing fields ...
    Resources  *StepResources  `json:"resources,omitempty"`
    Scheduling *StepScheduling `json:"scheduling,omitempty"`
}

type StepResources struct {
    CPURequest    string `json:"cpu_request,omitempty"`
    CPULimit      string `json:"cpu_limit,omitempty"`
    MemoryRequest string `json:"memory_request,omitempty"`
    MemoryLimit   string `json:"memory_limit,omitempty"`
}

type StepScheduling struct {
    NodeSelector      map[string]string  `json:"node_selector,omitempty"`
    NodeAffinity     []NodeAffinityTerm `json:"node_affinity,omitempty"`
    Tolerations       []Toleration       `json:"tolerations,omitempty"`
    PriorityClassName string             `json:"priority_class_name,omitempty"`
    RuntimeClassName  string             `json:"runtime_class_name,omitempty"`
}

type NodeAffinityTerm struct {
    Key      string   `json:"key"`
    Operator string   `json:"operator"`
    Values   []string `json:"values,omitempty"`
}

type Toleration struct {
    Key      string `json:"key"`
    Operator string `json:"operator"`
    Value    string `json:"value,omitempty"`
    Effect   string `json:"effect,omitempty"`
}
```

`Problem` loses `CPU` and `Memory`:
```go
type Problem struct {
    ID             string         `json:"id"`
    Name           string         `json:"name"`
    Level          string         `json:"level"`
    StartTime      time.Time      `json:"starttime"`
    EndTime        time.Time      `json:"endtime"`
    MaxSubmissions int            `json:"max_submissions"`
    Cluster        string         `json:"cluster"`
    // CPU and Memory REMOVED — per-step in WorkflowStep.Resources
    Upload         UploadLimit    `json:"upload"`
    Workflow       []WorkflowStep `json:"workflow"`
    Score          ScoreConfig    `json:"score"`
    Description    string         `json:"description"`
}
```

### DB model (`internal/database/models/models.go`)

`models.Problem` loses `CPU` and `Memory` fields. `Workflow` stays `RawJSON` — the new `StepResources`/`StepScheduling` are embedded in the workflow JSON. No schema migration needed (the DB column just stores whatever JSON).

### TS (`frontend/lib/types.ts`)

`WorkflowStep` gains `resources` and `scheduling`:
```ts
export interface WorkflowStep {
  name: string;
  image?: string;
  root?: boolean;
  timeout?: number;
  show: boolean;
  steps: string[][];
  mounts?: any[];
  network?: boolean;
  mpi?: { enabled: boolean; worker_replicas: number; slots_per_worker: number; launcher_cmd: string[] } | null;
  resources?: StepResources;
  scheduling?: StepScheduling;
}

export interface StepResources {
  cpu_request?: string;
  cpu_limit?: string;
  memory_request?: string;
  memory_limit?: string;
}

export interface StepScheduling {
  node_selector?: Record<string, string>;
  node_affinity?: NodeAffinityTerm[];
  tolerations?: Toleration[];
  priority_class_name?: string;
  runtime_class_name?: string;
}

export interface NodeAffinityTerm {
  key: string;
  operator: string;
  values?: string[];
}

export interface Toleration {
  key: string;
  operator: string;
  value?: string;
  effect?: string;
}
```

`Problem` loses `cpu` and `memory`.

---

## Pod spec builder (`internal/judger/podspec/mpijob.go`)

`PodSpecInput` changes:
- Remove `CPU int` and `MemoryMi int64`.
- Add `CPURequest string`, `CPULimit string`, `MemoryRequest string`, `MemoryLimit string`.
- Add `NodeAffinity []NodeAffinityTerm`, `Tolerations []Toleration`, `PriorityClassName string`, `RuntimeClassName string`.

`buildPodSpec`:
- Resources: `resource.MustParse(in.CPURequest)` with fallback `"1"`; same for limit, memory request (`"256Mi"` fallback), memory limit.
- NodeAffinity: build `corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: [...]}}`.
- Tolerations: build `[]corev1.Toleration`.
- `pod.Spec.PriorityClassName = in.PriorityClassName`.
- `pod.Spec.RuntimeClassName = &corev1.RuntimeClass{Name: in.RuntimeClassName}` (if non-empty).
- `pod.Spec.NodeSelector` stays from `in.NodeSel` (pool-level base).

`buildMPIJobSpec`: launcher + worker containers both get the same `StepResources`-derived resource list. `podTemplate` also gets the affinity/tolerations/priority/runtimeClass.

---

## Dispatcher (`internal/judger/dispatcher.go`)

`runPodStep` reads `flow.Resources` and `flow.Scheduling`:
```go
res := flow.Resources    // *StepResources (may be nil)
sched := flow.Scheduling  // *StepScheduling (may be nil)

cpuReq, cpuLim, memReq, memLim := stepResourceDefaults(res)
pod := podspec.BuildPodSpec(podspec.PodSpecInput{
    // ...
    CPURequest:       cpuReq,
    CPULimit:         cpuLim,
    MemoryRequest:    memReq,
    MemoryLimit:      memLim,
    NodeSel:          pool.NodeSelector,
    NodeAffinity:     nodeAffinityTerms(sched),
    Tolerations:      tolerations(sched),
    PriorityClassName: sched.GetPriorityClassName(),
    RuntimeClassName:  sched.GetRuntimeClassName(),
    TimeoutSec:       int64(flow.Timeout),
})
```

`stepResourceDefaults(res *StepResources)`:
- If `res == nil` → `"1", "1", "256Mi", "256Mi"`.
- For each field: if empty string → use the default; otherwise use the provided value.

`runMPIStep`: same — both launcher and worker get the same resources/scheduling.

---

## Scheduler (`internal/judger/scheduler.go`)

`findAvailablePool` no longer checks `problem.CPU`/`problem.Memory`:
```go
func (s *Scheduler) findAvailablePool(cluster *ClusterState) *PoolState {
    cluster.Lock()
    defer cluster.Unlock()
    for _, p := range cluster.pools {
        if p.IsPaused || p.CPU == 0 || p.Memory == 0 {
            continue
        }
        return p  // K8s scheduler handles actual resource fit
    }
    return nil
}
```

K8s's kube-scheduler is the real resource arbiter — if a Pod's resource requests exceed node capacity, it stays Pending and the `activeDeadlineSeconds` timeout catches it.

---

## Frontend

### Types (`frontend/lib/types.ts`)

Add `StepResources`, `StepScheduling`, `NodeAffinityTerm`, `Toleration` interfaces. Remove `cpu` and `memory` from `Problem`.

### Problem form (`frontend/components/admin/problem-actions.tsx`)

1. Remove "CPU Cores" and "Memory (MB)" inputs from the problem form (they're no longer on the problem).
2. Remove `cpu`/`memory` from the zod schema + form defaults.
3. `WorkflowStepEntry` gains `resources?: StepResources` and `scheduling?: StepScheduling`.
4. `workflowStepsToEntries` reads `resources`/`scheduling` from the workflow JSON.
5. `entriesToWorkflowSteps` outputs `resources`/`scheduling` if non-empty.

### Step card (`StepCard` component)

New panels inside each step (between Mounts and MPI):

**`ResourcesEditor`**:
- CPU Request (Input, placeholder `"2"` / `"500m"`)
- CPU Limit (Input, placeholder `"2"`)
- Memory Request (Input, placeholder `"1Gi"` / `"256Mi"`)
- Memory Limit (Input, placeholder `"1Gi"`)
- Badge hint: "Set request == limit for Guaranteed QoS + CPU pinning"
- "Make Guaranteed" button: sets limit = request for all 4 fields

**`SchedulingEditor`**:
- Node Selector: key-value pair list (add/remove rows, like the mounts editor)
- Node Affinity: term list (key + operator Select: In/NotIn/Exists/DoesNotExist + values comma-separated)
- Tolerations: term list (key + operator Select: Equal/Exists + value + effect Select: NoSchedule/NoExecute/PreferNoSchedule)
- Priority Class Name (Input, placeholder `"latency-critical"`)
- Runtime Class Name (Input, placeholder `"kata"` / `"runc"`)

---

## Testing

- **`podspec` unit tests**: update `TestBuildPodSpec_Basic` for the new `CPURequest`/`CPULimit`/`MemoryRequest`/`MemoryLimit` string fields. Add tests for Guaranteed QoS (request==limit), Burstable (request<limit), empty→defaults, nodeAffinity mapping, tolerations mapping, priorityClassName, runtimeClassName.
- **`buildMPIJobSpec` tests**: launcher + worker containers get the same resources.
- **Frontend build**: `pnpm build` passes, 18 pages.
- **Manual**: create a problem with per-step resources (CPU `"2"`/`"2"`, memory `"1Gi"`/`"1Gi"`) + scheduling (nodeSelector + toleration). Verify the JSON round-trips correctly via `GET /api/v1/problems/:id`.

## Breaking change

`Problem.CPU` and `Problem.Memory` are removed. Existing problem DB records have dead `cpu`/`memory` columns (harmless — GORM ignores unknown columns on read, and the `Workflow` RawJSON stores per-step resources). Steps without `resources` default to `"1"` CPU / `"256Mi"` memory.

## Out of scope

- kubelet CPU Manager / Topology Manager config (node-level).
- scheduler profiles (cluster-level).
- LimitRange / ResourceQuota (namespace-level).
- Pod-level resources (K8s 1.34+ beta — using container-level).
