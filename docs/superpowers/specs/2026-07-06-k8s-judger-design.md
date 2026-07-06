# Kubernetes Judger Rewrite — Design

**Date:** 2026-07-06
**Scope:** Replace the Docker-based judger with a Kubernetes-based one. Support multiple K8s clusters (two-level `Cluster → NodePool`), per-cluster concurrency, and MPI-operator-based multi-node judging. Keep the existing pubsub log-streaming + on-disk NDJSON model and the admin/frontend API shapes (with one documented break: the score contract).

Builds on the merged single-repo state on branch `merge-webui-admin`, after the backend-managed-config migration.

## Goals

1. **One Pod per `WorkflowStep`** with a generated shell entrypoint (the step's `Steps [][]string` serialized into a `sh -c` script). Sequential steps share a per-submission RWX PVC at `/mnt/work`.
2. **Multi-cluster**: each `cluster` config entry is a K8s cluster (kubeconfig + context + namespace + concurrency + heartbeat_ttl + declared node-pool names). Per-cluster K8s clientset; one queue per cluster; N-way concurrency per cluster (configurable semaphore).
3. **Node-pools** (two-level `Cluster → NodePool`): a node-pool is a `nodeSelector` + cpu/memory caps, stored in the DB, admin-mutable. The dispatcher sets `nodeSelector` on the Pod; kube-scheduler places it. No in-process core-bitmap — K8s is the scheduler.
4. **MPI-operator multi-node**: an optional `WorkflowStep.MPI` field makes that step an MPIJob (launcher + worker replicas); the launcher runs `mpirun <launcherCmd>` and writes the result file. Non-MPI steps are plain Pods.
5. **Recovery**: HA heartbeat + TTL grace period; on claim, label-select and delete all `csoj-*` pods/MPIJobs, mark `Running` submissions `Failed`, re-enqueue `Queued`.
6. **Score contract change**: the final step writes `/mnt/work/.csoj/result.json` (`{score, performance, info}`) on the PVC; the dispatcher reads the file. Breaks existing problems that print JSON to stdout.

## Non-goals

- A log backend (Loki/ES) — keep pubsub + on-disk NDJSON.
- Namespace isolation within a pool — one `csoj-judger` namespace per cluster.
- Explicit GPU resource modeling — `nodeSelector` + standard device plugins suffice.
- Priority classes / fair queuing — FIFO cluster queue.
- Admin-frontend URL update (node→pool) — separate follow-up; this spec is backend.
- Migrating the on-disk Docker judger state — fresh start.
- Preemption of running submissions on pool pause.

---

## Architecture

### Current state (recap, post backend-managed-config)

- `config.Cluster` is `Name + Nodes []Node` where `Node` is `Name + Docker` (Docker connection in config.yaml); cpu/memory caps are in the DB `ClusterNode` table.
- `judger.Scheduler` holds `clusters map[string]*ClusterState`; each `ClusterState` has `Nodes map[string]*NodeState` with `UsedCores []bool` + `UsedMemory`. One worker goroutine per cluster, **serial** processing.
- `judger.Dispatcher` creates a Docker volume per submission, runs each `WorkflowStep` as a Docker container (`docker exec` for each `Steps` entry), captures stdout/stderr via `stdcopy` into a pubsub topic + NDJSON buffer, parses the **last step's stdout** as JSON for the score.
- `judger.DockerManager` wraps the Docker SDK: `CreateContainer`/`StartContainer`/`ExecInContainer`/`CleanupContainer`/`CopyToContainer`/`CreateVolume`/`RemoveVolume`.
- `internal/pubsub.Broker`: in-memory, per-container/per-submission topics, history-replay on subscribe.
- Recovery: `RecoverAndCleanup` builds a Docker client per node config, `CleanupContainer`s orphaned containers; marks `Running` submissions `Failed`.
- Submission content: API writes to `cfg.Storage.SubmissionContent/<subID>` on local disk; the dispatcher `CopyToContainer`s it into the container at step 0.

### Target state

- `config.Cluster` is `Name + Kubeconfig + Context + Namespace + Concurrency + HeartbeatTTL + NodePools []NodePool{Name}` (names only). The K8s connection (kubeconfig) is boot-time; node-pool caps are in the DB `cluster_node_pools` table.
- `judger.Scheduler` holds `clusters map[string]*ClusterState`; each `ClusterState` has `k8s kubernetes.Interface`, `dynamic dynamic.Interface`, `pools map[string]*PoolState`, `sem chan struct{}` (size `concurrency`), `queue chan QueuedSubmission`, `mpiEnabled bool`. One worker goroutine per cluster, N-way concurrency.
- `judger.Dispatcher` builds a Pod spec (or MPIJob spec) per `WorkflowStep`, creates it via the K8s clientset/dynamic, tails pod logs into the pubsub, reads `/mnt/work/.csoj/result.json` on completion, deletes the pod.
- `judger.k8s.go` (`KubeManager`) replaces `DockerManager`: `CreatePod`/`GetPodStatus`/`StreamPodLogs`/`DeletePod`/`CreateMPIJob`/`GetMPIJobStatus`/`DeleteMPIJob`/`ListJudgerPods` (label-select).
- Submission content: a shared **RWX PVC** mounted at `/mnt/work` on both the API server and every judging pod. The API writes uploads to `<pvc-mount>/<subID>/`; the pod's `/mnt/work` IS `<pvc-mount>/<subID>` (per-submission subpath via a `subPath` volumeMount). No copy.
- `UsedCores`/`UsedMemory` core-bitmap removed. Dashboard "used" numbers read live from the K8s node/metrics API.

---

## Cluster model & config

### `config.yaml` (boot + K8s connection only)

```yaml
cluster:
  - name: "gpu-cluster"
    kubeconfig: "/etc/csoj/kubeconfigs/gpu.yaml"
    context: ""
    namespace: "csoj-judger"
    concurrency: 4
    heartbeat_ttl: 30s
    node_pools:
      - name: "gpu-pool"
      - name: "cpu-pool"
```

`config.Cluster`:
```go
type Cluster struct {
    Name         string     `yaml:"name"`
    Kubeconfig   string     `yaml:"kubeconfig"`
    Context      string     `yaml:"context"`
    Namespace    string     `yaml:"namespace"`
    Concurrency  int        `yaml:"concurrency"`
    HeartbeatTTL Duration  `yaml:"heartbeat_ttl"`
    NodePools    []NodePool `yaml:"node_pools"`
}
type NodePool struct {
    Name string `yaml:"name"`
}
```

`DockerConfig` and the Docker `Node` are removed. `config.Load` unchanged (read-only).

### DB: `cluster_node_pools` (rename of `ClusterNode`)

```go
type ClusterNodePool struct {
    ClusterName  string    `gorm:"primaryKey" json:"cluster_name"`
    PoolName     string    `gorm:"primaryKey" json:"pool_name"`
    NodeSelector JSONMap   `gorm:"type:text" json:"node_selector"`
    CPU          int       `json:"cpu"`
    Memory       int64     `json:"memory"`
    IsPaused     bool      `gorm:"default:false" json:"is_paused"`
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

Old `ClusterNode` table dropped (fresh start). At boot, for each `config.Cluster`, the scheduler upserts a `cluster_node_pools` row for each declared `node_pools[].name` with zeroed caps if none exists (admin must `PUT` to enable) — mirrors today's "register disabled" behavior.

### DB: `judger_heartbeats` (HA)

```go
type Heartbeat struct {
    ClusterName string    `gorm:"primaryKey" json:"cluster_name"`
    InstanceID  string    `json:"instance_id"`
    LastBeatAt  time.Time `json:"last_beat_at"`
}
```

---

## Judger Pod lifecycle (one Pod per WorkflowStep)

For a non-MPI `WorkflowStep`:

**Pod spec:**
- `name = <subID>-<step>`; `namespace = cluster.Namespace`; labels `app=csoj-judger`, `csoj-submission=<subID>`, `csoj-step=<i>`.
- `image = flow.Image`; `imagePullPolicy: IfNotPresent`.
- `command = ["/bin/sh","-c", <generated script>]`. The generated script:
  ```sh
  #!/bin/sh
  set -e
  echo '--- Executing Command 1 ---'
  eval "<steps[0]>"
  ec=$?
  echo '--- Exit Code: $ec ---'
  [ $ec -ne 0 ] && exit $ec
  echo '--- Executing Command 2 ---'
  eval "<steps[1]>"
  ec=$?
  echo '--- Exit Code: $ec ---'
  exit $ec
  ```
  (Each `Steps` entry is `eval`'d; non-zero exit aborts. The `---` markers mirror today's pubsub info messages.)
- `env`: `CSOJ_SUBMIT_DIR=/mnt/work`, `CSOJ_USERNAME=<username>`.
- `securityContext`: non-root steps → `runAsUser: 1000, runAsGroup: 1000`; root steps omit.
- `resources`: `requests` and `limits` both `{cpu: <problem.CPU>, memory: <problem.Memory>Mi}`.
- `nodeSelector`: `pool.NodeSelector`.
- `volumes`/`volumeMounts`: submission PVC at `/mnt/work` via `subPath: <subID>` (so each submission sees its own dir on the shared RWX PVC). `flow.Mounts` translated: `"tmpfs"` → `emptyDir` with `medium: Memory` (+ size from `TmpfsOptions.SizeBytes`); `"volume"` → a `PVC` reference; `"bind"` → `hostPath` (with the same caveats as today).
- `activeDeadlineSeconds = flow.Timeout` (K8s enforces per-step timeout; a Pending pod that can't be scheduled within the deadline is also failed).
- Network: `flow.Network == false` → the pod gets label `csoj-net=isolated`; a cluster-level default-deny `NetworkPolicy` selects that label. (Best-effort; documented.)

**Step 0 population:** none — the submission PVC is already populated by the API (`/mnt/work` IS the submission's directory via `subPath`). No `CopyToContainer` equivalent.

**Score extraction:** on pod `Succeeded`/`Failed`, the dispatcher reads `/mnt/work/.csoj/result.json` → `{score, performance, info}`. Missing file → `failSubmission` ("result.json not found"). The file is on the PVC, readable by the API server (which also mounts the PVC) — the dispatcher reads it via the API server's own PVC mount, not via K8s API.

**Log streaming:** while the pod is `Running`, the dispatcher calls `corev1.PodInterface.GetLogs(name, &corev1.PodLogOptions{Follow:true, Container:"main"})`. The returned `io.ReadCloser` is read in chunks; each chunk is published to `cont.ID` pubsub topic (streamType `"stdout"`, with `"info"` for the `---` markers already embedded in the stream by the entrypoint). The chunks are also appended to an in-memory NDJSON buffer — same shape as today's `outputCallback`. On completion the buffer flushes to `cfg.Storage.SubmissionLog/<subID>_<uuid>.log`; post-completion WS reads from disk.

**Cleanup:** in `Dispatch`'s defer, the dispatcher `DeleteCollection`s pods labeled `csoj-submission=<subID>` and `CloseTopic`s. The concurrency semaphore is released.

---

## Multi-node judging via MPI operator

`judger.WorkflowStep` gains:
```go
type WorkflowStep struct {
    // ...existing
    MPI *MPIConfig `json:"mpi,omitempty"`
}
type MPIConfig struct {
    Enabled        bool     `json:"enabled"`
    WorkerReplicas int      `json:"worker_replicas"`
    SlotsPerWorker int      `json:"slots_per_worker"`
    LauncherCmd    []string `json:"launcher_cmd"`
}
```

When `MPI.Enabled`:
- Build an **MPIJob** CRD (`kubeflow.org/v2beta1`) via the dynamic client (`dynamic.Resource("mpijobs.kubeflow.org")`).
- `MPIReplicaSpecs`: `Launcher` (1 replica) + `Worker` (`WorkerReplicas` replicas).
- **Launcher**: image=`flow.Image`, `command=["/bin/sh","-c","mpirun -np <total ranks> <launcherCmd joined>"]`. (mpi-operator's launcher template provides `mpirun` + hostfile; we pass `launcherCmd` as the tail.) `total ranks = WorkerReplicas * SlotsPerWorker`.
- **Worker**: image=`flow.Image`, `command=["sleep","infinity"]`. Workers provide MPI ranks; mpi-operator handles passwordless SSH.
- Both replicas: same `resources`, `nodeSelector`, `imagePullPolicy`, `securityContext`, env, and the submission PVC at `/mnt/work` (`subPath: <subID>`).
- `Launcher.activeDeadlineSeconds = flow.Timeout`.
- MPIJob `name = <subID>-<step>`; labels `app=csoj-judger`, `csoj-submission=<subID>`, `csoj-step=<i>`.

**Score/logs:** the launcher writes `/mnt/work/.csoj/result.json`. While running, the dispatcher tails the **launcher pod's** logs (`GetLogs` with `container=launcher`) into pubsub — same surface as a plain pod. Worker logs are not streamed (debug-only via `kubectl`).

**CRD probe:** at boot, the scheduler probes `mpijobs.kubeflow.org` in each cluster's API; if absent, logs a warning and marks the cluster `mpiEnabled=false`. An MPI step submitted to an MPI-disabled cluster → `failSubmission` ("cluster does not support MPI").

**Non-MPI steps** (`MPI == nil` or `MPI.Enabled == false`) → plain Pod (previous section). Mixed workflows allowed (a compile Pod step then an MPIJob step, sharing `/mnt/work`).

---

## Log streaming & recovery

### Logs (pubsub unchanged)

- `internal/pubsub.Broker` unchanged: in-memory, per-`Container.ID` and per-`Submission.ID` topics, history-replay on subscribe.
- Live: dispatcher tails pod logs (`GetLogs Follow:true`) → publishes chunks to `cont.ID` topic + appends to NDJSON buffer.
- Post-completion: buffer flushes to `cfg.Storage.SubmissionLog/<subID>_<uuid>.log`; WS reads from disk.
- `WsMessage{Stream,Data}` shape and `streamType` (`stdout`/`info`/`error`) unchanged. `stderr` is folded into `stdout` (K8s pod logs aren't stream-multiplexed); the `---` markers remain `info`.
- `CloseTopic` on completion/failure/interrupt — unchanged.

### Recovery (HA heartbeat + TTL grace + label-delete)

1. **Heartbeat:** a goroutine writes `Heartbeat{ClusterName, InstanceID, LastBeatAt=now}` every `HeartbeatTTL/2` (e.g. 15s) while the judger runs. `InstanceID` is a UUID generated at boot.
2. **Boot claim:** at startup, for each cluster, the new instance waits `HeartbeatTTL`. It reads the `Heartbeat` row; if `LastBeatAt` is within TTL (another live instance), it aborts startup for that cluster (only one judger owns a cluster). If `LastBeatAt` is older than TTL (or absent), the new instance writes its own `InstanceID` + `now` — **claim** — and proceeds.
3. **Recover:** after claiming, label-select all pods/MPIJobs (`app=csoj-judger`) in the cluster namespace, `DeleteCollection` them; in a single DB transaction mark all `Running` submissions `Failed` (`info.error="System interrupted during execution"`), `CloseTopic` their pubsub; re-enqueue `Queued` submissions. Idempotent.

### Interrupt (user/admin)

`interruptSubmission`: delete pods/MPIJobs labeled `csoj-submission=<subID>`, publish `"error"` to `sub.ID`, `CloseTopic`, mark `Failed`. Endpoint shape unchanged.

---

## Admin surface, config trim, data model

### Admin API

| Endpoint | Change |
|---|---|
| `GET /api/v1/admin/clusters/status` | In-memory pool state + live K8s node state (used/available cpu+memory, node count). |
| `PUT /api/v1/admin/clusters/:c/pools/:p` | Update `cluster_node_pools` (cpu/memory/node_selector/is_paused); refresh in-memory. Replaces `PUT /admin/clusters/:c/nodes/:n`. |
| `POST /api/v1/admin/clusters/:c/pools` | Create a node-pool. |
| `DELETE /api/v1/admin/clusters/:c/pools/:p` | Delete a node-pool. |
| `PUT /api/v1/admin/clusters/:c/concurrency` | Set the cluster's concurrency; resize the semaphore. |
| pause/resume node | Removed; pool `is_paused` DB flag (dispatcher skips paused pools). |

### Config trim

`config.Cluster` → `Name + Kubeconfig + Context + Namespace + Concurrency + HeartbeatTTL + NodePools []NodePool{Name}`. `DockerConfig`, Docker `Node`, `ClusterNode` removed. `config.Storage`: `submission_content` is now the RWX PVC mount path (shared by API + judger pods); `submission_log` stays local disk; `database` stays SQLite.

### Data model

- `models.ClusterNode` → `models.ClusterNodePool` (rename + `NodeSelector JSONMap`, `IsPaused bool`). Fresh start — old rows dropped.
- `models.Heartbeat` (new).
- `models.Container.DockerID` → reused as `PodName` (string); `LogFilePath` unchanged.
- `models.Submission`: drop `AllocatedCores`; keep `Cluster`, `Node` (Node = pool name).
- `models.Problem.Workflow` (RawJSON) unchanged; the Go `judger.WorkflowStep` gains `MPI *MPIConfig`.

### Boot flow (`main.go`)

1. `database.Init` (AutoMigrate new tables).
2. Heartbeat claim + TTL grace per cluster.
3. Label-select + delete `csoj-*` pods/MPIJobs, mark `Running` → `Failed`, re-enqueue `Queued`.
4. `LoadFromDB` → appState.
5. `NewScheduler(cfg, db, appState)`: build K8s clientsets + dynamic clients per cluster, per-cluster semaphores, probe MPI CRD.
6. Start heartbeat goroutine (write every `ttl/2`).
7. `go scheduler.Run()`.

### Docker removal

`internal/judger/docker.go` deleted. `internal/judger/dispatcher.go` rewritten to call `internal/judger/k8s.go` (`KubeManager`). `go.mod`: remove `github.com/docker/docker`; add `k8s.io/client-go`, `k8s.io/api`, `k8s.io/apimachinery`. MPIJob handled via the dynamic client (no `kubeflow.org` Go type dependency) so the CRD version isn't pinned.

---

## Error handling

- Pod create fails → `failSubmission`, release semaphore, `CloseTopic`.
- Pod stuck `Pending` → `activeDeadlineSeconds` times out → Pod `Failed` → `failContainer`.
- MPI CRD absent at boot → cluster `mpiEnabled=false`; MPI submissions `failSubmission` ("cluster does not support MPI").
- `result.json` missing → `failSubmission` ("result.json not found").
- K8s API server unreachable → retry with backoff; if it exceeds the per-step timeout, the step fails. Doesn't crash the cluster.
- Heartbeat DB write fails → log + retry; if no write for >TTL, ownership is lost (safe fail-open for HA).

## Testing / verification

- **Unit (table-driven, pure funcs):** entrypoint-script generation (escaping, exit-code markers, multi-command); MPIJob spec building; pool→nodeSelector translation; semaphore acquire/release.
- **k8s.go against a `kind` cluster:** CreatePod/StreamLogs/DeletePod; CreateMPIJob (with mpi-operator installed).
- **End-to-end (kind):** first-user→superadmin; claim cluster; create node-pool; submit a 1-step problem that writes `result.json`; assert pod runs, score read, pod deleted, WS streams logs. MPI problem (skip if CRD absent). Recovery: kill judger with a pod running, restart → pod deleted, submission `Failed`.
- **HA:** two judger binaries against one cluster; second waits TTL then claims; only one runs.

## Migration

Fresh start. `ClusterNode` rows dropped (`cluster_node_pools` replaces). No contest/problem/asset data loss (all DB-backed). **Breaking score contract:** problems must write `/mnt/work/.csoj/result.json` instead of printing JSON to stdout. Documented in `docs/configuration/problem-config.md` + `getting-started.md`; migration note: redirect the final step's stdout to the file.

## Out of scope

- Log backend (Loki/ES) — keep pubsub+disk.
- Namespace isolation within a pool.
- Explicit GPU resource modeling (nodeSelector + device plugins suffice).
- Priority classes / fair queuing.
- Admin-frontend node→pool URL update (separate follow-up).
- Preemption on pool pause.
