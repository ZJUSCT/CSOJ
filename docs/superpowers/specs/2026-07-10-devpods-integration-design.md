# DevPods Integration — Design

**Date:** 2026-07-10
**Scope:** Add a top-level "DevPods" tab to CSOJ. Admins define fixed-recipe templates (cluster, cpu/memory, NUMA + cpuset binding policy). Users create DevPod instances from templates with per-template quota. Each instance shows an SSH login command. Storage and lifecycle are owned by the external devpods system; CSOJ is a thin creator/viewer.

**Branch:** `devpods-integration`

## Context

CSOJ (this repo) is an online judge. A separate project, `~/develop/devpods`, is a Kubernetes-native multi-tenant remote-dev platform: a developer writes a `DevPod` custom resource (`devpod.io/v1alpha1`) and gets an SSH-reachable dev environment via a single gateway (`ssh <owner>+<podname>@<gateway>`). DevPod supports persistence, hibernation (`spec.running: false`), collaborators, LDAP pubkey auth, and KubeVirt VMs.

This spec connects the two: CSOJ users get a self-service DevPods tab; CSOJ drives devpods by creating DevPod CRs over the Kubernetes API. CSOJ owns templates and quota; devpods owns identity, storage, SSH, and pod lifecycle.

## Decisions (confirmed in brainstorming)

- **Identity / SSH:** CSOJ does not touch SSH keys or authentication. DevPod `owner` is set to the CSOJ **username** (not nickname). DevPods authenticates via its own User CRD / LDAP. CSOJ lazily creates an empty-pubkeys devpods `User` CR on first DevPod creation so LDAP-only users resolve.
- **Connection:** devpods is installed in the same cluster(s) CSOJ already manages. CSOJ reuses the existing `Cluster` table (the kubeconfig each row stores) — one of those rows points at the devpods cluster. No new kubeconfig store.
- **Storage:** CSOJ stores no DevPod instance records. Quota counting and listings are live `List` calls against DevPod CRs filtered by label. Storage/PVC is a devpods concern (set via `spec.persistence`).
- **Quota:** both per-user×template and global×template limits, set per template.
- **NUMA/cpuset:** CPU = integer cores with `requests==limits` (Guaranteed QoS → kubelet static CPU manager → cpuset cgroup). NUMA = a `nodeSelector` entry (e.g. `numa-node: "0"`) requiring pre-labeled nodes.
- **SSH command:** admin configures a fixed gateway address in a setting; CSOJ concatenates `ssh <username>+<podname>@<host> -p <port>`.
- **Visibility:** DevPods tab is visible to all logged-in users; template management is admin-only.
- **Path:** CSOJ talks to devpods CRDs directly via the dynamic client (no devpods REST API, no mirrored record table).

## Non-goals

- CSOJ storing DevPod instance records (no `devpods` table) or PVCs.
- CSOJ managing SSH keys / User pubkeys (LDAP/devpods owns this).
- Per-user quota overrides table (`DevPodQuotaOverride`) — deferred (YAGNI).
- DevPods websocket / live log streaming — 5s SWR polling suffices.
- VM-backed DevPods (only `spec.pod`; the `spec.vm` path is out of scope).
- Collaborators UI (devpod's `spec.collaborators` left empty; admin/devops can edit the CR directly).
- Changes to CSOJ's user registration (username format is validated at DevPod create time, not at registration).

---

## 1. Data model (CSOJ side)

CSOJ stores only **templates**. DevPod instances are not mirrored.

### `DevPodTemplate` (new DB model)

| Field | Type | Notes |
|---|---|---|
| `ID` | string | primary key, lowercase, `[a-z0-9-]{1,6}` (kept short — fits DevPod name budget). e.g. `gpu-8c`. |
| `Name` | string | display name. |
| `ClusterName` | string | references `Cluster.name` (the eval cluster table). CSOJ uses that row's kubeconfig + namespace to talk to devpods. |
| `Image` | string | default container image, e.g. `ubuntu:24.04`. |
| `Shell` | string | `bash`\|`zsh`\|`fish`, empty = devpods supervisor fallback. |
| `Cores` | int | integer CPU cores. Becomes `resources.requests/limits.cpu`. |
| `Memory` | int64 | bytes. Becomes `resources.requests/limits.memory`. |
| `NodeSelector` | JSONMap (text) | fixed `nodeSelector` map; carries NUMA label e.g. `{"numa-node":"0"}`. |
| `Tolerations` | JSONMap (text, optional) | fixed `tolerations` array. |
| `DefaultPerUser` | int | per-user×template cap. Default 1. |
| `DefaultGlobal` | int | global×template cap. Default 5. |
| `PersistenceSize` | string (optional) | if set, enables `spec.persistence` (e.g. `"20Gi"`). |
| `CreatedAt`/`UpdatedAt` | time.Time | |

### Settings

- `devpods.gateway` = `{"host":"devpod.example.com","port":2222}` — used to build the SSH command. Stored in the existing `settings` table, managed via the existing `PUT /admin/settings/:key`.

CSOJ's `User` model is **unchanged** (no pubkey field).

### DB migration

Add `&models.DevPodTemplate{}` to `database.Init`'s `AutoMigrate`. No destructive change; new table.

---

## 2. Backend

### `internal/devpods` package (CRD client)

Reuses the `KubeManager` pattern (`internal/judger/k8s.go` already holds a `kubernetes.Interface` + `dynamic.Interface` per cluster). A new thin client wraps the dynamic client for the two GVRs:

```go
// internal/devpods/client.go
type Client struct {
    dyn dynamic.Interface
    ns  string // = referenced Cluster.Namespace
}

const (
    gvrDevPod = schema.GroupVersionResource{Group:"devpod.io", Version:"v1alpha1", Resource:"devpods"}
    gvrUser   = schema.GroupVersionResource{Group:"devpod.io", Version:"v1alpha1", Resource:"users"}
)

// RenderDevPod builds an unstructured DevPod CR from a template.
func RenderDevPod(owner, podName string, tpl *models.DevPodTemplate) (*unstructured.Unstructured, error)

// CRUD + helpers:
//   EnsureUser(owner)                 // idempotent create empty-pubkeys User CR
//   CreateDevPod(u *unstructured.Unstructured) error
//   GetDevPod(name) (*unstructured.Unstructured, error)
//   ListDevPods(ownerLabel, templateLabel string) ([]unstructured.Unstructured, error)
//   PatchRunning(name string, running bool) error
//   DeleteDevPod(name) error
```

### DevPod CR shape (rendered)

```yaml
apiVersion: devpod.io/v1alpha1
kind: DevPod
metadata:
  name: <username>-<tplid>-<rand4>      # owner-scoped; ≤22 chars (CEL)
  namespace: <Cluster.Namespace>
  labels:
    devpod.io/owner: <username>         # devpods' own label
    csoj.io/template: <tplid>            # CSOJ quota-counting label
spec:
  owner: <username>
  running: true
  shell: <tpl.Shell>                     # may be empty
  pod:
    spec:
      containers:
        - name: dev
          image: <tpl.Image>
          resources:
            requests: {cpu: <Cores>, memory: <Memory>}
            limits:   {cpu: <Cores>, memory: <Memory>}   # == requests → Guaranteed QoS → cpuset
      nodeSelector: <tpl.NodeSelector>    # includes numa-node: "0"
      tolerations: <tpl.Tolerations>
  # spec.persistence: {size: <tpl.PersistenceSize>, mountPath: /home/<username>}  when set
```

`shareProcessNamespace` is **not** set by CSOJ — devpods' own render decides (its v2 layout runs supervisor+sshd in one container; the field is moot).

### Owner source

`c.GetString("userID")` is a UUID. Handlers call `database.GetUserByID(h.db, userID)` to read `.Username`; that username is the DevPod owner, label value, lazy User CR name, and SSH login user.

### Cluster access

`Scheduler` already holds a `clusters map[string]*ClusterState`, each with a cached `k8s kubernetes.Interface` and `dyn dynamic.Interface` plus `Namespace` (built in `buildClusterState`). Add an accessor — `Scheduler.DynamicClientForCluster(name) (dynamic.Interface, namespace string, error)` — that RLocks the map, returns the cached dynamic client + namespace for a cluster name (or an error if the cluster isn't loaded). DevPod handlers construct a `devpods.Client` per request from it. Unexported `ClusterState.dyn`/`k8s` fields are read via this accessor (or expose a getter on `ClusterState`).

### API routes

**User-facing** (`internal/api/user/`, mounted under the existing `authed` group):

| Method + path | Body / result |
|---|---|
| `GET /devpods` | `{items:[{name,template,phase,endpoint,created_at,ssh_command}], gateway:{host,port}}` — list DevPod CRs whose `devpod.io/owner` label == caller's username. |
| `POST /devpods` | body `{template_id}` → validate quota → `EnsureUser` → render → `CreateDevPod`. 409 on quota. |
| `GET /devpods/:name` | single DevPod detail + ssh command; 403 if owner label != caller. |
| `POST /devpods/:name/start` | patch `spec.running=true`. owner check. |
| `POST /devpods/:name/stop` | patch `spec.running=false`. owner check. |
| `DELETE /devpods/:name` | delete DevPod CR. owner check. |

**Admin-facing** (`internal/api/admin/`, under `AdminMiddleware`):

| Method + path | |
|---|---|
| `GET /admin/devpod-templates` | list templates |
| `POST /admin/devpod-templates` | create |
| `PUT /admin/devpod-templates/:id` | update |
| `DELETE /admin/devpod-templates/:id` | delete |

### Quota counting

`ListDevPods` filtered by labels:
- **per-user×template:** `devpod.io/owner=<username>` + `csoj.io/template=<tplid>`. Reject if `len >= DefaultPerUser`.
- **global×template:** `csoj.io/template=<tplid>` only. Reject if `len >= DefaultGlobal`.

No caching — DevPod instance counts are expected in the low hundreds.

### Validation

- **Username format:** DevPod/User CEL requires `^[a-z0-9-]{1,32}$`, no `+`. At `POST /devpods`, if the caller's username fails this regex → 400 `"username not valid for devpod naming"`. Registration is not changed.
- **Template ID:** `[a-z0-9-]{1,6}` (enforced on template CRUD).
- **Name budget:** `len(username) + 1 + len(tplid) + 1 + 4 ≤ 22`. If exceeded → 400 `"username too long for this template; use a shorter username"`.
- **Cluster existence:** template CRUD + DevPod create check the referenced `Cluster` row exists; missing → 400/502.

### Error handling

- Quota exceeded → 409 `{"message":"per-template limit reached (N/M)"}`.
- Cluster kubeconfig bad / cluster row missing → 502 with cluster name.
- DevPod CR create failure (name collision, devpods admission reject) → 500 with raw error.
- Owner mismatch on `GET/POST/DELETE /devpods/:name` → 403.

---

## 3. Frontend

### Top-level tab

`frontend/components/layout/main-nav.tsx` `allRoutes` gains `{ href:"/devpods", label:t("devpods") }` (after `contests`). Visible to all logged-in users (the `(main)` layout already requires auth via `withAuth`).

New route group `frontend/app/(main)/devpods/` with `page.tsx`:

```
DevPods page
├─ Template card grid (each: name / cores·mem / NUMA / per-user limit / used N/M)
│   [Create] button per card (disabled when user's count >= per-user limit)
└─ "My DevPods" table
    columns: name | template | phase | ssh command (+ copy button) | created
    row actions: start/stop | delete
```

### Admin templates page

New `frontend/app/(admin)/admin/devpod-templates/` + an entry in `admin-sub-nav.tsx` (`{href:"/admin/devpod-templates", label:"DevPod Templates", icon:Boxes}`). Table CRUD; create/edit form: name, cluster (select from `/admin/clusters`), image, shell, cores, memory, NUMA node, nodeSelector (key-value editor), tolerations, per-user/global limits, persistence size.

### Data fetching

- `GET /devpods` via SWR, 5s refresh interval (phase Pending→Running drift).
- `GET /admin/devpod-templates` via SWR for the admin page.
- Mutations (`POST /devpods`, start/stop/delete) invalidate the `/devpods` SWR key.

### SSH command

Built client-side from the `gateway` field returned by `GET /devpods`:
`ssh {username}+{podname}@{gateway.host} -p {gateway.port}` (only shown once `phase==Running` and endpoint non-empty; otherwise show phase + "preparing…").

### i18n

Add `devpods.*` and `admin.devpodTemplates.*` keys to the existing next-intl message files (zh + en).

---

## 4. Security & boundaries

- **Owner enforcement:** every user-facing handler reads the DevPod CR's `metadata.labels["devpod.io/owner"]` and 403s if it != caller's username. Owner-scoped naming (CEL `name.startsWith(owner+'-')`) + the label are a double check.
- **No CSOJ RBAC for devpod CRs:** CSOJ's own ServiceAccount (the one it uses to talk to the devpods cluster) must have RBAC on `devpods` + `users` resources in the devpods namespace. This is an install/deploy concern, documented in the spec but not enforced in code.
- **Template management:** admin-only via existing `AdminMiddleware`.
- **SSH keys:** never stored or proxied by CSOJ.

---

## 5. Testing

- **Unit:** `RenderDevPod` produces a DevPod spec with correct resources/nodeSelector/labels/persistence (table-driven over templates with/without persistence, NUMA, tolerations). Quota check rejects at per-user and global limits (mock the list).
- **Backend e2e (manual):** with a devpods cluster wired into CSOJ's `Cluster` table, create a template, create a DevPod as a user, observe phase→Running, verify ssh command, stop/start/delete.
- **Frontend:** `pnpm build` passes; DevPods tab renders for a normal user; admin templates page CRUD works.
- **No devpods cluster available:** template whose `ClusterName` is missing → 400 at create.

## Out of scope (deferred)

- `DevPodQuotaOverride` (per-user quota overrides).
- VM-backed DevPods.
- DevPod collaborators UI.
- Websocket/live status.
- CSOJ registration username format tightening.
- Snapshotting DevPods from CSOJ (use devpods' `DevPodSnapshot` directly if needed).
