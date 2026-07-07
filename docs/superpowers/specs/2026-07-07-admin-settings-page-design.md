# Admin Settings Page + Cluster Page Rewrite — Design

**Date:** 2026-07-07
**Scope:** Add a new `/admin/settings` page to the frontend admin panel for managing DB-backed runtime settings (logger, CORS, local-auth, GitLab OIDC, JWT expiry). Rewrite the `/admin/cluster` page to use the new DB-managed cluster + pool endpoints (replacing the removed node-based endpoints). Add a "Settings" entry to the admin sub-nav.

## Goals

1. **New `/admin/settings` page** — tabbed form (General / Security / GitLab / CORS) that loads via `GET /admin/settings` and saves each key via `PUT /admin/settings/:key`. Read-only "Boot Facts" card showing listen/storage/jwt_secret_present.
2. **Rewritten `/admin/cluster` page** — cluster-row table (from `GET /admin/clusters`) with create/edit/delete dialogs (including a kubeconfig textarea), a "Reload Clusters" button (`POST /admin/clusters/reload`), and a live pool-status table with pool CRUD (`POST/PUT/DELETE /admin/clusters/:c/pools/:p`) and pause/resume.
3. **Admin sub-nav** — add a 7th "Settings" entry.
4. **Types** — add `ClusterRow` + `ClusterNodePool`; update `ClusterStatusResponse` to the new snapshot shape; remove dead `ConfigNode`/`NodeState`/`NodeDetail`.

## Non-goals

- i18n for the settings page (English-hardcoded, matching the existing admin pages).
- A dedicated "Boot Facts" edit page (those are config.yaml — restart-required).
- Migrating the old `ConfigNode`/`NodeState` types if anything else references them (grep first; remove only if unused).

---

## Components

### `frontend/app/(main)/admin/settings/page.tsx` (new)

- `export default withAdmin(SettingsPage)` — gated by `withAdmin` HOC.
- Renders `<AdminSubNav />` at the top.
- `useSWR('/admin/settings', fetcher)` loads `{ settings: {...}, boot: { listen, storage, jwt_secret_present } }`.
- A `<Tabs>` with 4 tabs:
  - **General** — `logger` setting: `level` (Select: debug/info/warn/error) + `file` (Input). Save button calls `PUT /admin/settings/logger` with `{"value":{"level":"...","file":"..."}}`. Badge: "restart required".
  - **Security** — `auth.local` (Checkbox: enabled) + `auth.jwt.expire_hours` (Input number). Save calls `PUT /admin/settings/auth.local` + `PUT /admin/settings/auth.jwt.expire_hours`. Both live.
  - **GitLab OIDC** — `auth.gitlab` fields: `url`, `client_id`, `client_secret` (password-type Input), `redirect_uri`, `frontend_callback_url`. Save calls `PUT /admin/settings/auth.gitlab` with the full JSON. Live.
  - **CORS** — `cors.allowed_origins`: a Textarea (one origin per line, parsed to `string[]` on save). Save calls `PUT /admin/settings/cors` with `{"value":{"allowed_origins":[...]}}`. Live.
- Read-only **Boot Facts** card above the tabs (always visible): shows `listen`, `storage.database`, `storage.submission_content`, `storage.submission_log`, `storage.user_avatar`, `jwt_secret_present` (boolean badge).
- Each tab's save button: `useToast` for success/failure feedback.

### `frontend/app/(main)/admin/cluster/page.tsx` (rewritten)

- `export default withAdmin(ClusterPage)`.
- `<AdminSubNav />` at top.
- Two sections:
  1. **Cluster Rows** — `useSWR('/admin/clusters', fetcher)` loads `ClusterRow[]`. Table: name, namespace, concurrency, heartbeat_ttl(s). Actions: Edit (dialog with kubeconfig Textarea + all fields), Delete (AlertDialog confirm). "Add Cluster" button opens a create dialog. "Reload Clusters" button calls `POST /admin/clusters/reload` → toast with warnings.
  2. **Live Pool Status** — `useSWR('/admin/clusters/status', fetcher, { refreshInterval: 3000 })` loads `ClusterStatusResponse`. For each cluster: a Card with a table of pools (name, cpu, memory, is_paused badge, used/available). Pool actions: Pause/Resume (PUT `/admin/clusters/:c/pools/:p` with `is_paused`), Edit (dialog: cpu/memory/nodeSelector), Delete (AlertDialog). "Add Pool" button per cluster.

### `frontend/components/layout/admin-sub-nav.tsx`

Add to `routes`:
```ts
{ href: "/admin/settings", label: "Settings", icon: Settings },
```
Import `Settings` from `lucide-react`.

### `frontend/lib/types.ts`

Add:
```ts
export interface ClusterRow {
  name: string;
  kubeconfig: string;
  context: string;
  namespace: string;
  concurrency: number;
  heartbeat_ttl: number;
}

export interface ClusterNodePool {
  cluster_name: string;
  pool_name: string;
  node_selector: Record<string, string>;
  cpu: number;
  memory: number;
  is_paused: boolean;
}
```

Update `ClusterStatusResponse` to match the scheduler's `ClusterStateSnapshot`:
```ts
export interface PoolState {
  Name: string;
  NodeSelector: Record<string, string>;
  CPU: number;
  Memory: number;
  IsPaused: boolean;
}
export interface ClusterStateSnapshot {
  Name: string;
  Namespace: string;
  Pools: Record<string, PoolState>;
  MPIEnabled: boolean;
  QueueLength: number;
  Concurrency: number;
}
export interface ClusterStatusResponse {
  resource_status: Record<string, ClusterStateSnapshot>;
  queue_lengths: Record<string, number>;
}
```

Remove `ConfigNode`, `NodeState`, `NodeDetail` if nothing else references them (grep first; the user-facing pages don't use them — only the admin cluster page did).

---

## Data flow

- **Settings load**: `GET /admin/settings` → `{ settings: { "logger": "{...}", "cors": "{...}", ... }, boot: { listen, storage, jwt_secret_present } }`. Each value is a JSON string; the page parses it into the form fields.
- **Settings save**: `PUT /admin/settings/:key` with body `{"value": <parsed JSON>}`. The backend's `SettingsStore.Set` writes to DB + updates the cache (live for CORS/local/gitlab/jwt; restart-required for logger).
- **Cluster rows load**: `GET /admin/clusters` → `ClusterRow[]` (kubeconfig omitted from list response; included in the edit dialog's GET... actually the list omits kubeconfig for safety. The edit dialog shows the existing fields but the kubeconfig is editable — the admin pastes the full kubeconfig text).
- **Cluster create/update**: `POST /admin/clusters` or `PUT /admin/clusters/:name` with `{ name, kubeconfig, context, namespace, concurrency, heartbeat_ttl }`.
- **Reload**: `POST /admin/clusters/reload` → `{ warnings: [...] }`. Toast shows warnings (if any) + success.
- **Pool status**: `GET /admin/clusters/status` → `{ resource_status: { clusterName: { Pools, MPIEnabled, ... } }, queue_lengths }`. Polled every 3s.
- **Pool CRUD**: `POST /admin/clusters/:c/pools` (create), `PUT /admin/clusters/:c/pools/:p` (update cpu/memory/node_selector/is_paused), `DELETE /admin/clusters/:c/pools/:p` (delete).

## Error handling

- Settings load fail → skeleton + "Failed to load settings."
- Settings save fail → destructive toast with `error.response?.data?.message`.
- Cluster create/update fail → toast. Kubeconfig parse error (returned as a warning from reload) → toast.
- Pool save fail → toast.
- All follow the existing `useToast` + SWR `onError` (global toast) pattern.

## Testing

- `pnpm build` succeeds (static export). 18 pages (the existing 17 + the new settings page).
- Manual: load the settings page, change CORS, verify the next request has CORS headers. Disable local auth, verify `/auth/status` shows `false`. Create a cluster row (fake kubeconfig), reload, verify no crash. Pause/resume a pool, verify the badge updates.

## Out of scope

- i18n for the settings page.
- Editing boot facts (config.yaml) from the UI.
- The user-facing pages (contests/problems/submissions) are unchanged.
