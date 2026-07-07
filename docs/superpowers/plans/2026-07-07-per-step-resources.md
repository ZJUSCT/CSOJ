# Per-Step Resources + Scheduling Constraints — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move CPU/memory resources and K8s scheduling constraints (nodeSelector, nodeAffinity, tolerations, priorityClassName, runtimeClassName) from the problem level to each workflow step. Each step independently controls its Pod's resources and scheduling.

**Architecture:** `WorkflowStep` gains optional `Resources` and `Scheduling` structs. `buildPodSpec` switches from `int` CPU/memory to K8s resource strings (`"2"`, `"500m"`, `"1Gi"`). `Problem.CPU`/`Memory` are removed. The frontend workflow editor gains Resources + Scheduling panels per step.

**Tech Stack:** Go 1.24, k8s.io/api, Next.js 14, React 18, TypeScript, shadcn/ui.

**Reference spec:** `docs/superpowers/specs/2026-07-07-per-step-resources-design.md`

**Branch:** `merge-webui-admin`.

---

## Task 1: Go types — add StepResources/StepScheduling, remove Problem.CPU/Memory

**Files:**
- Modify: `internal/judger/loader.go`
- Modify: `internal/database/models/models.go`

- [ ] **Step 1: Add new types to `loader.go`**

In `internal/judger/loader.go`, add after the `MPIConfig` type:

```go
type StepResources struct {
	CPURequest    string `json:"cpu_request,omitempty"`
	CPULimit      string `json:"cpu_limit,omitempty"`
	MemoryRequest string `json:"memory_request,omitempty"`
	MemoryLimit   string `json:"memory_limit,omitempty"`
}

type StepScheduling struct {
	NodeSelector      map[string]string  `json:"node_selector,omitempty"`
	NodeAffinity      []NodeAffinityTerm `json:"node_affinity,omitempty"`
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

Add `Resources` and `Scheduling` fields to `WorkflowStep`:
```go
type WorkflowStep struct {
	Name    string     `json:"name"`
	Image   string     `json:"image"`
	Root    bool       `json:"root"`
	Timeout int        `json:"timeout"`
	Show    bool       `json:"show"`
	Steps   [][]string `json:"steps"`
	Mounts  []Mount    `json:"mounts"`
	Network bool       `json:"network"`
	MPI     *MPIConfig  `json:"mpi,omitempty"`
	Resources  *StepResources  `json:"resources,omitempty"`
	Scheduling *StepScheduling `json:"scheduling,omitempty"`
}
```

Remove `CPU` and `Memory` from `Problem`:
```go
type Problem struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Level          string         `json:"level"`
	StartTime      time.Time      `json:"starttime"`
	EndTime        time.Time      `json:"endtime"`
	MaxSubmissions int            `json:"max_submissions"`
	Cluster        string         `json:"cluster"`
	Upload         UploadLimit    `json:"upload"`
	Workflow       []WorkflowStep `json:"workflow"`
	Score          ScoreConfig    `json:"score"`
	Description    string         `json:"description"`
}
```

- [ ] **Step 2: Remove CPU/Memory from `models.Problem`**

In `internal/database/models/models.go`, in the `Problem` struct, delete the `CPU` and `Memory` lines:
```go
	CPU            int       `json:"cpu"`       // DELETE
	Memory         int64     `json:"memory"`    // DELETE
```

- [ ] **Step 3: Remove CPU/Memory from `db.go` problemFromModel + ProblemToModel**

In `internal/judger/db.go`, in `problemFromModel`:
- Delete `CPU: p.CPU,` and `Memory: p.Memory,` (lines ~80-81).

In `ProblemToModel`:
- Delete `CPU: p.CPU,` and `Memory: p.Memory,` (lines ~135-136).

- [ ] **Step 4: Verify the judger package compiles (will fail on dispatcher.go + podspec — expected)**

Run: `go build ./internal/judger/loader.go ./internal/judger/db.go 2>&1 | head` — these two files should be clean. The package will fail to build because `dispatcher.go` + `podspec/mpijob.go` reference the old `Problem.CPU`/`Memory` and `PodSpecInput.CPU`/`MemoryMi` — that's fixed in Tasks 2-3.

- [ ] **Step 5: Commit**

```bash
git add internal/judger/loader.go internal/judger/db.go internal/database/models/models.go
git commit -m "refactor(judger): add StepResources/StepScheduling; remove Problem.CPU/Memory"
```

---

## Task 2: Update podspec builder — string resources + scheduling

**Files:**
- Modify: `internal/judger/podspec/mpijob.go`
- Modify: `internal/judger/podspec/mpijob_test.go`

- [ ] **Step 1: Update `PodSpecInput`**

In `internal/judger/podspec/mpijob.go`, replace `CPU int` and `MemoryMi int64` with string resource fields, and add scheduling fields:

```go
type PodSpecInput struct {
	Name             string
	Namespace        string
	Image            string
	Script           string
	CPURequest       string
	CPULimit         string
	MemoryRequest    string
	MemoryLimit      string
	NodeSel          map[string]string
	NodeAffinity     []NodeAffinityTerm
	Tolerations      []Toleration
	PriorityClassName string
	RuntimeClassName  string
	Env              []corev1.EnvVar
	SubID            string
	Step             int
	AsRoot           bool
	Network          bool
	TimeoutSec       int64
}
```

Define `NodeAffinityTerm` and `Toleration` types in `mpijob.go` (they mirror the judger types but in the `podspec` package):
```go
type NodeAffinityTerm struct {
	Key      string
	Operator string
	Values   []string
}

type Toleration struct {
	Key      string
	Operator string
	Value    string
	Effect   string
}
```

- [ ] **Step 2: Add a resource helper**

Add a helper that parses a resource string with a fallback:
```go
func parseResource(s, fallback string) resource.Quantity {
	if s == "" {
		return resource.MustParse(fallback)
	}
	return resource.MustParse(s)
}
```

- [ ] **Step 3: Update `buildPodSpec` (now `BuildPodSpec`)**

Replace the resource block in `BuildPodSpec`:
```go
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    parseResource(in.CPURequest, "1"),
				corev1.ResourceMemory: parseResource(in.MemoryRequest, "256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    parseResource(in.CPULimit, "1"),
				corev1.ResourceMemory: parseResource(in.MemoryLimit, "256Mi"),
			},
		},
```

Add scheduling fields to the `PodSpec`:
```go
		// Node affinity
		Affinity: buildNodeAffinity(in.NodeAffinity),
		// Tolerations
		Tolerations: buildTolerations(in.Tolerations),
		// Priority
		PriorityClassName: in.PriorityClassName,
		// RuntimeClass
		RuntimeClassName: runtimeClassNamePtr(in.RuntimeClassName),
```

Add helper functions:
```go
func buildNodeAffinity(terms []NodeAffinityTerm) *corev1.Affinity {
	if len(terms) == 0 {
		return nil
	}
	var nodeSelectorTerms []corev1.NodeSelectorTerm
	for _, t := range terms {
		var values []corev1.NodeSelectorRequirement
		vals := t.Values
		if vals == nil {
			vals = []string{}
		}
		values = append(values, corev1.NodeSelectorRequirement{
			Key:      t.Key,
			Operator: corev1.NodeSelectorOperator(t.Operator),
			Values:   vals,
		})
		nodeSelectorTerms = append(nodeSelectorTerms, corev1.NodeSelectorTerm{
			MatchExpressions: values,
		})
	}
	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: nodeSelectorTerms,
			},
		},
	}
}

func buildTolerations(tols []Toleration) []corev1.Toleration {
	if len(tols) == 0 {
		return nil
	}
	var result []corev1.Toleration
	for _, t := range tol {
		result = append(result, corev1.Toleration{
			Key:      t.Key,
			Operator: corev1.TolerationOperator(t.Operator),
			Value:    t.Value,
			Effect:   corev1.TaintEffect(t.Effect),
		})
	}
	return result
}

func runtimeClassNamePtr(name string) *string {
	if name == "" {
		return nil
	}
	return &name
}
```

- [ ] **Step 4: Update `MPIJobSpecInput` + `buildMPIJobSpec`**

Replace `CPU int` / `MemoryMi int64` in `MPIJobSpecInput` with the same string fields + scheduling:
```go
type MPIJobSpecInput struct {
	Name              string
	Namespace         string
	Image             string
	LauncherScript    string
	WorkerReplicas    int
	CPURequest        string
	CPULimit          string
	MemoryRequest     string
	MemoryLimit       string
	NodeSel           map[string]string
	NodeAffinity      []NodeAffinityTerm
	Tolerations       []Toleration
	PriorityClassName string
	RuntimeClassName  string
	SubID             string
	Step              int
}
```

Update `mpiContainerTemplates` to use `parseResource` instead of `int64` quantities. Update `podTemplate` to include affinity/tolerations/priority/runtimeClass.

- [ ] **Step 5: Update the tests**

In `internal/judger/podspec/mpijob_test.go`, update `TestBuildPodSpec_Basic`:
- Replace `CPU: 2, MemoryMi: 512` with `CPURequest: "2", CPULimit: "2", MemoryRequest: "512Mi", MemoryLimit: "512Mi"`.
- Add assertions for nodeAffinity, tolerations, priorityClassName, runtimeClassName.

Add a new test `TestBuildPodSpec_Burstable`:
```go
func TestBuildPodSpec_Burstable(t *testing.T) {
	pod := BuildPodSpec(PodSpecInput{
		Name: "test", Namespace: "ns", Image: "img", Script: "echo",
		CPURequest: "500m", CPULimit: "2",
		MemoryRequest: "256Mi", MemoryLimit: "1Gi",
		SubID: "s1", Step: 0, TimeoutSec: 30,
	})
	req := pod.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	if req.Cmp(resource.MustParse("500m")) != 0 {
		t.Errorf("cpu request: %s", req.String())
	}
	lim := pod.Spec.Containers[0].Resources.Limits[corev1.ResourceCPU]
	if lim.Cmp(resource.MustParse("2")) != 0 {
		t.Errorf("cpu limit: %s", lim.String())
	}
}
```

Add a test for defaults:
```go
func TestBuildPodSpec_Defaults(t *testing.T) {
	pod := BuildPodSpec(PodSpecInput{
		Name: "test", Namespace: "ns", Image: "img", Script: "echo",
		SubID: "s1", Step: 0, TimeoutSec: 30,
	})
	req := pod.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	if req.Cmp(resource.MustParse("1")) != 0 {
		t.Errorf("default cpu request: %s", req.String())
	}
}
```

Update `TestBuildMPIJobSpec_LauncherWorker` to use the new string fields.

- [ ] **Step 6: Verify**

Run: `go test ./internal/judger/podspec/ -v`
Expected: all tests pass.

Run: `go build ./internal/judger/podspec/`
Expected: exit 0.

- [ ] **Step 7: Commit**

```bash
git add internal/judger/podspec/mpijob.go internal/judger/podspec/mpijob_test.go
git commit -m "refactor(podspec): string resources + scheduling constraints per step"
```

---

## Task 3: Update dispatcher + scheduler

**Files:**
- Modify: `internal/judger/dispatcher.go`
- Modify: `internal/judger/scheduler.go`

- [ ] **Step 1: Update `runPodStep` in `dispatcher.go`**

Replace the `BuildPodSpec` call:
```go
	res := flow.Resources
	sched := flow.Scheduling

	cpuReq := ""
	cpuLim := ""
	memReq := ""
	memLim := ""
	if res != nil {
		cpuReq = res.CPURequest
		cpuLim = res.CPULimit
		memReq = res.MemoryRequest
		memLim = res.MemoryLimit
	}

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

	pod := podspec.BuildPodSpec(podspec.PodSpecInput{
		Name:              podName,
		Namespace:         km.ns,
		Image:             flow.Image,
		Script:            script,
		CPURequest:        cpuReq,
		CPULimit:          cpuLim,
		MemoryRequest:     memReq,
		MemoryLimit:       memLim,
		NodeSel:           pool.NodeSelector,
		NodeAffinity:      nodeAff,
		Tolerations:       tols,
		PriorityClassName: priorityClass,
		RuntimeClassName:  runtimeClass,
		Env:               env,
		SubID:             sub.ID,
		Step:              step,
		AsRoot:            flow.Root,
		Network:           flow.Network,
		TimeoutSec:        int64(flow.Timeout),
	})
```

- [ ] **Step 2: Update `runMPIStep` in `dispatcher.go`**

Same pattern — read `flow.Resources` and `flow.Scheduling`, pass to `BuildMPIJobSpec`.

- [ ] **Step 3: Update `findAvailablePool` in `scheduler.go`**

Remove `cpu`/`mem` parameters — K8s scheduler handles resource fit:
```go
func (s *Scheduler) findAvailablePool(cluster *ClusterState) *PoolState {
	cluster.Lock()
	defer cluster.Unlock()
	for _, p := range cluster.pools {
		if p.IsPaused || p.CPU == 0 || p.Memory == 0 {
			continue
		}
		return p // K8s scheduler checks actual resource fit
	}
	return nil
}
```

Update the call site in `clusterWorker`:
```go
	pool := s.findAvailablePool(cluster)
```
(was `s.findAvailablePool(cluster, job.Problem.CPU, job.Problem.Memory)`)

- [ ] **Step 4: Verify**

Run: `go build ./...`
Expected: exit 0.

Run: `go vet ./...`
Expected: clean.

Run: `go test ./internal/judger/podspec/ ./internal/config/`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/judger/dispatcher.go internal/judger/scheduler.go
git commit -m "refactor(judger): dispatcher reads per-step resources/scheduling; pool selection simplified"
```

---

## Task 4: Frontend types + workflow editor

**Files:**
- Modify: `frontend/lib/types.ts`
- Modify: `frontend/components/admin/problem-actions.tsx`

- [ ] **Step 1: Update TS types**

In `frontend/lib/types.ts`:

Remove `cpu` and `memory` from `Problem`.

Add to `WorkflowStep`:
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
```

Add the new interfaces:
```ts
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

Also remove `cpu`/`memory` from the admin `Problem` type at line ~199 (the one with `max_submissions`).

- [ ] **Step 2: Update `problem-actions.tsx`**

In `frontend/components/admin/problem-actions.tsx`:

1. Remove `cpu` and `memory` from the zod schema, form defaults, and form reset.
2. Remove the `FormField` for `cpu` and `memory` (lines ~283-284).
3. Add `resources` and `scheduling` to `WorkflowStepEntry`:
```ts
interface WorkflowStepEntry {
    name: string;
    image: string;
    root: boolean;
    timeout: number;
    show: boolean;
    network: boolean;
    steps: string[][];
    mounts: MountEntry[];
    mpi?: MPIConfig | null;
    resources: { cpu_request: string; cpu_limit: string; memory_request: string; memory_limit: string };
    scheduling: {
        node_selector: { key: string; value: string }[];
        node_affinity: { key: string; operator: string; values: string }[];
        tolerations: { key: string; operator: string; value: string; effect: string }[];
        priority_class_name: string;
        runtime_class_name: string;
    };
}
```

4. Update `emptyStep()` to include default `resources` and `scheduling`:
```ts
function emptyStep(): WorkflowStepEntry {
    return {
        name: '', image: '', root: false, timeout: 30, show: true, network: false,
        steps: [], mounts: [], mpi: null,
        resources: { cpu_request: '', cpu_limit: '', memory_request: '', memory_limit: '' },
        scheduling: { node_selector: [], node_affinity: [], tolerations: [], priority_class_name: '', runtime_class_name: '' },
    };
}
```

5. Update `workflowStepsToEntries` to read `resources`/`scheduling` from the workflow JSON.

6. Update `entriesToWorkflowSteps` to output `resources`/`scheduling`.

7. Add `ResourcesEditor` component inside `StepCard` (between Mounts and MPI):
```tsx
function ResourcesEditor({ res, onChange }: { res: any, onChange: (r: any) => void }) {
    return (
        <div className="space-y-2">
            <div className="flex items-center justify-between">
                <Label className="flex items-center gap-1"><Cpu className="h-3 w-3" /> Resources</Label>
                <Button type="button" variant="ghost" size="sm" onClick={() => onChange({
                    ...res, cpu_limit: res.cpu_request, memory_limit: res.memory_request
                })}>Make Guaranteed</Button>
            </div>
            <div className="grid grid-cols-2 gap-2 pl-4">
                <div>
                    <Label className="text-xs">CPU Request</Label>
                    <Input className="text-xs font-mono" value={res.cpu_request} onChange={e => onChange({ ...res, cpu_request: e.target.value })} placeholder="2" />
                </div>
                <div>
                    <Label className="text-xs">CPU Limit</Label>
                    <Input className="text-xs font-mono" value={res.cpu_limit} onChange={e => onChange({ ...res, cpu_limit: e.target.value })} placeholder="2" />
                </div>
                <div>
                    <Label className="text-xs">Memory Request</Label>
                    <Input className="text-xs font-mono" value={res.memory_request} onChange={e => onChange({ ...res, memory_request: e.target.value })} placeholder="1Gi" />
                </div>
                <div>
                    <Label className="text-xs">Memory Limit</Label>
                    <Input className="text-xs font-mono" value={res.memory_limit} onChange={e => onChange({ ...res, memory_limit: e.target.value })} placeholder="1Gi" />
                </div>
            </div>
            <p className="text-xs text-muted-foreground pl-4">Set request == limit for Guaranteed QoS + CPU pinning.</p>
        </div>
    );
}
```

8. Add `SchedulingEditor` component inside `StepCard`:
```tsx
function SchedulingEditor({ sched, onChange }: { sched: any, onChange: (s: any) => void }) {
    // Node selector: key-value pairs (add/remove)
    // Node affinity: terms (key + operator Select + values)
    // Tolerations: terms (key + operator Select + value + effect Select)
    // Priority Class Name (Input)
    // Runtime Class Name (Input)
    // Follows the same pattern as MountsEditor
}
```

9. Render both inside `StepCard` between the `MountsEditor` and `MPIEditor`:
```tsx
<Separator />
<ResourcesEditor res={step.resources} onChange={(r) => updateStep(i, { resources: r })} />
<Separator />
<SchedulingEditor sched={step.scheduling} onChange={(s) => updateStep(i, { scheduling: s })} />
```

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: 18 pages, succeeds.

- [ ] **Step 4: Commit**

```bash
git add frontend/lib/types.ts frontend/components/admin/problem-actions.tsx
git commit -m "feat(frontend): per-step resources + scheduling editor in workflow"
```

---

## Task 5: Build + verify

**Files:** (no changes — verification)

- [ ] **Step 1: Build everything**

Run: `go build ./... && go vet ./... && go test ./internal/judger/podspec/ ./internal/config/`
Expected: clean, all tests pass.

Run: `cd frontend && pnpm build`
Expected: 18 pages.

Run: `make build`
Expected: binary produced.

- [ ] **Step 2: Manual smoke test**

Start the server. Create a problem with a workflow step that has:
- CPU Request=`"2"`, CPU Limit=`"2"`, Memory Request=`"1Gi"`, Memory Limit=`"1Gi"`
- Node affinity: key=`cpu-manager`, operator=`In`, values=`static`
- Toleration: key=`dedicated`, operator=`Equal`, value=`cpu-pinning`, effect=`NoSchedule`
- PriorityClassName=`latency-critical`
- RuntimeClassName=`runc`

Verify `GET /api/v1/problems/:id` returns the workflow JSON with `resources` and `scheduling` fields.

- [ ] **Step 3: No commit**

---

## Self-Review notes

- `podspec.NodeAffinityTerm` and `podspec.Toleration` are separate types from `judger.NodeAffinityTerm` and `judger.Toleration` (different packages). The dispatcher converts between them.
- `parseResource` uses `resource.MustParse` which panics on invalid strings. This is acceptable — the frontend validates inputs and the admin is responsible for valid K8s resource strings.
- The `findAvailablePool` no longer takes `cpu`/`mem` args — the clusterWorker call site must be updated.
- `models.Problem` losing `CPU`/`Memory` means the DB column still exists (GORM won't drop it) but Go won't read/write it. Harmless.
- The frontend `WorkflowStepEntry.resources` uses flat string fields (not optional) — empty string means "use default".
- The `SchedulingEditor` follows the same add/remove-row pattern as `MountsEditor` — refer to that component for the UI pattern.
