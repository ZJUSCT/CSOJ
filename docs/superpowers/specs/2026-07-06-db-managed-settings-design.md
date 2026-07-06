# Move Runtime Config to DB + Admin Panel — Design

**Date:** 2026-07-06
**Scope:** Shrink `config.yaml` to deployment facts only (`listen`, `storage.*`, `auth.jwt.secret`). Move everything else — logger, CORS, local-auth toggle, GitLab OIDC, JWT expiry, and ALL cluster config (kubeconfig text, namespace, concurrency, heartbeat_ttl) — into the database, managed via the admin panel. Runtime settings hot-reload live (CORS/local/gitlab/jwt-expire per-request; logger restart-required); cluster changes apply via a "Reload Clusters" admin button that rebuilds K8s clientsets without a server restart.

Builds on the K8s-judger-rewrite state on branch `merge-webui-admin`.

## Goals

1. **`config.yaml` = boot facts only.** `listen`, `storage.{database,user_avatar,submission_content,submission_log}`, `auth.jwt.secret`. Nothing else. These are the irreducible "what disk/mount/crypto-key did I boot with" facts.
2. **Scalar runtime settings → `settings` DB table.** A generic key/value store (`logger`, `cors`, `auth.local`, `auth.gitlab`, `auth.jwt.expire_hours`) with JSON-encoded values, read through an in-memory `SettingsStore` cache. Live-reload for CORS/local/gitlab/jwt-expire; logger restart-required.
3. **Clusters → `clusters` DB table.** Each K8s cluster is a row: `name`, `kubeconfig` (full YAML text), `context`, `namespace`, `concurrency`, `heartbeat_ttl` (seconds). Node-pool caps stay in the existing `cluster_node_pools` table. The scheduler reads clusters from the DB at boot and on a "Reload Clusters" admin action.
4. **`SettingsStore` is the runtime source of truth** for the moved settings — consumers re-read it per-request (CORS/local/gitlab) or per-token-issue (jwt-expire), not a stale `cfg`.
5. **"Reload Clusters" admin button** rebuilds the scheduler's in-memory `clusters` map (new clientsets, re-probe MPI CRD) without a restart.

## Non-goals

- Logger hot-reload (`zap.ReplaceGlobals` swap) — restart-required; flagged in the panel.
- Admin-frontend settings page UI — backend provides the API; the frontend page is a separate follow-up (documented).
- Cluster gitlab-OIDC connection pooling — handler rebuilds per-request (cheap).
- Migrating the existing `podspec` unit tests — unchanged.
- HA-aware cluster add (heartbeat claim races on a brand-new cluster row) — best-effort; logged.

---

## Data model

### New `settings` table (generic key/value)

`internal/database/models/models.go`:
```go
type Setting struct {
	Key       string    `gorm:"primaryKey" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`   // JSON-encoded (string, []string, bool, int, struct)
	UpdatedAt time.Time `json:"updated_at"`
}
```

Keys and value shapes:
- `logger` → `{"level":"debug","file":"csoj.log"}`
- `cors` → `{"allowed_origins":["https://oj.example.com"]}`
- `auth.local` → `{"enabled":true}`
- `auth.gitlab` → `{"app":"","url":"https://gitlab.com","client_id":"","client_secret":"","redirect_uri":"","frontend_callback_url":""}`
- `auth.jwt.expire_hours` → `72`

### New `clusters` table (replaces `config.Cluster`)

```go
type Cluster struct {
	Name         string    `gorm:"primaryKey" json:"name"`
	Kubeconfig   string    `gorm:"type:text" json:"kubeconfig"`   // full kubeconfig YAML text
	Context      string    `json:"context"`
	Namespace    string    `json:"namespace"`
	Concurrency  int       `json:"concurrency"`
	HeartbeatTTL int       `json:"heartbeat_ttl"`                  // seconds
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
```

`cluster_node_pools` (already exists) is unchanged — pool caps/selector/pause per cluster.

### Removed from `config.go`

`Logger`, `CORS`, `Cluster`, `NodePool`, `Duration`, `Auth.Local`, `Auth.GitLab`. `Auth.JWT.ExpireHours` stays in the struct but is overridden by `settings` at runtime; only `Auth.JWT.Secret` is the boot fact.

Final `Config`:
```go
type Config struct {
	Listen  string  `yaml:"listen"`
	Storage Storage `yaml:"storage"`
	Auth    Auth    `yaml:"auth"`
}
type Auth struct {
	JWT JWT `yaml:"jwt"`
}
type JWT struct {
	Secret      string `yaml:"secret"`
	ExpireHours int    `yaml:"expire_hours"` // overridden by settings at runtime
}
```

---

## `SettingsStore` (runtime read-through cache)

`internal/config/settings.go`:
```go
type SettingsStore struct {
	db    *gorm.DB
	mu    sync.RWMutex
	cache map[string]string  // key -> JSON value
}
```

- `NewSettingsStore(db)` — loads all `settings` rows into `cache`.
- `Get(key string, dst interface{})` — RLock → cache lookup → `json.Unmarshal`. Missing key = zero-value `dst`, no error.
- `Set(key string, value interface{})` — marshal → `db.Save(&Setting{Key, Value})` → Lock → cache update.
- `ReloadAll()` — re-read the whole table into cache (used after bulk admin edits + on "Reload Clusters").

The `SettingsStore` is injected into consumers that need the moved settings; `*config.Config` no longer carries them.

---

## Consumer rewires

| Consumer | Today | After |
|---|---|---|
| `main.go` logger | `cfg.Logger` once | Boot: `settings.Get("logger", &loggerCfg)` → build zap. **Restart-required** to change (no live `zap.ReplaceGlobals` swap). |
| `CORSMiddleware` | `cfg.CORS` once | `api.CORSMiddleware(settingsStore)` — reads `settings.Get("cors", &cors)` per-request. Live. |
| `getAuthStatus` (local toggle) | `cfg.Auth.Local.Enabled` | `settings.Get("auth.local", &local)` per-request. Live. |
| `NewGitLabHandler` | built once at boot with `cfg.Auth.GitLab` | `GitLabHandler` reads `settings.Get("auth.gitlab", &gl)` on each `Login`/`Callback`. Live. |
| `GenerateJWT` / `ValidateJWT` / asset HMAC | `cfg.Auth.JWT.Secret`/`ExpireHours` | `cfg.Auth.JWT.Secret` stays in config.yaml (restart to rotate). `ExpireHours` reads `settings.Get("auth.jwt.expire_hours")`. Live for new tokens. |
| `scheduler` clusters | `cfg.Cluster` at boot | `database.GetAllClusters(db)` at boot + on "Reload Clusters". |
| `recovery.RecoverAndCleanup` | iterates `cfg.Cluster` | `database.GetAllClusters(db)`. Signature drops `cfg`. |
| `main.go` heartbeat loop | iterates `cfg.Cluster` | iterates `database.GetAllClusters(db)`. |

---

## Admin API

`internal/api/admin/settings.go` (new):
- `GET /api/v1/admin/settings` → all `settings` rows + boot-fact values (listen, storage paths, jwt-secret presence) for display.
- `PUT /api/v1/admin/settings/:key` → body is the JSON value; writes via `settingsStore.Set`. Triggers the live side-effect (CORS/local/gitlab per-request; logger flagged restart-required). Returns `{restart_required: bool}`.

`internal/api/admin/cluster.go` (extend):
- `GET /api/v1/admin/clusters` → list `clusters` rows (without kubeconfig text, for safety).
- `POST /api/v1/admin/clusters` → create a cluster row (body: name, kubeconfig, context, namespace, concurrency, heartbeat_ttl).
- `PUT /api/v1/admin/clusters/:name` → update (including kubeconfig text).
- `DELETE /api/v1/admin/clusters/:name` → delete.
- `POST /api/v1/admin/clusters/reload` → `scheduler.ReloadClusters()`; returns counts + per-cluster warnings.

The existing pool endpoints (`GET/POST/PUT/DELETE /admin/clusters/:c/pools/:p`) stay; they CRUD `cluster_node_pools`. The cluster row's `name` is the parent key.

---

## Boot flow (`cmd/CSOJ/main.go`)

1. `config.Load(path)` → `cfg` (boot facts only).
2. `database.Init(cfg.Storage.Database)` → AutoMigrate `settings` + `clusters`.
3. `settingsStore := config.NewSettingsStore(db)` → load cache.
4. Logger build from `settingsStore.Get("logger")`.
5. `judger.RecoverAndCleanup(db, instanceID)` — reads `clusters` from DB.
6. `judger.LoadFromDB(db)` → appState.
7. `scheduler := judger.NewScheduler(db, settingsStore, appState)` — reads `clusters` + pools from DB, `heartbeat_ttl` from settings.
8. Heartbeat goroutines per DB cluster row.
9. `judger.RequeuePendingSubmissions(db, scheduler, appState)`.
10. `go scheduler.Run()`.
11. Gin: `r.Use(api.CORSMiddleware(settingsStore))`; `user.RegisterRoutes(r, cfg, settingsStore, db, scheduler, appState)`; `admin.RegisterRoutes(r, cfg, settingsStore, db, scheduler, appState)`.
12. `r.Run(cfg.Listen)`.

### `Scheduler.ReloadClusters()`

```go
func (s *Scheduler) ReloadClusters() ([]string, error) {
    dbClusters, err := database.GetAllClusters(s.db)
    // ... build new map[string]*ClusterState (clientsets + pools) ...
    //   skip clusters whose kubeconfig fails to parse (collect warnings)
    //   re-probe MPI CRD per cluster
    s.Lock()
    s.clusters = newClusters
    s.Unlock()
    // re-claim heartbeats for new clusters; stop heartbeats for removed
    return warnings, nil
}
```

In-flight dispatch goroutines hold the old `ClusterState` pointer; the map swap doesn't preempt them. Per-cluster queues persist (a cluster removed from the DB keeps draining its existing queue, then the queue is dropped).

---

## Error handling

- `SettingsStore.Get` miss → zero-value `dst`, no error (admin hasn't configured → defaults: CORS no header, local-auth disabled, gitlab handler returns 500 "gitlab not configured" on login, jwt-expire uses `cfg.Auth.JWT.ExpireHours` fallback).
- `SettingsStore.Set` DB write fail → return error, cache unchanged (admin sees 500). Consistency preserved.
- `ReloadClusters` kubeconfig parse fail → that cluster skipped with a warning; others reload. Returns the warnings list.
- GitLab handler rebuild fail (OIDC provider unreachable) → 500 with error; config not corrupted; next request retries.
- `POST /admin/clusters/reload` → idempotent (re-reads DB). New-cluster heartbeat claim fail (another instance owns it within TTL) → cluster loads with a warning, heartbeat goroutine doesn't start; admin can retry.

## Testing

- **Unit (table-driven, in-memory sqlite or gorm mock):** `SettingsStore.Get`/`Set` JSON round-trip; `settings.LoadInto(cfg, db)` overlay precedence; `database.GetAllClusters`/`UpsertCluster`/`DeleteCluster` CRUD round-trip.
- **`podspec` tests:** unchanged (6 pass).
- **Manual (UI test instance on :18080):**
  1. `PUT /admin/settings/cors` → verify CORS applied.
  2. `PUT /admin/settings/auth.local` → `{"enabled":false}`; verify `/auth/status` + `/auth/local/login` 404.
  3. `POST /admin/clusters` (fake kubeconfig) → `POST /admin/clusters/reload` → no crash, warning logged, server healthy.
  4. `PUT /admin/settings/logger` → restart-required badge.
  5. Restart → DB settings persist.

## Docs

- `docs/configuration/main-config.md` — rewrite: config.yaml shrinks to `listen`+`storage`+`auth.jwt.secret`. Add a "Runtime Settings (database)" section listing the admin-managed keys + endpoints. Remove the `cluster` config block docs.
- `docs/getting-started.md` — sample config.yaml shrinks; note "after first boot, configure logger/CORS/GitLab/clusters via the admin panel".
- `docs/api-reference/admin-api.md` — add `GET/PUT /admin/settings`, `GET/POST/PUT/DELETE /admin/clusters`, `POST /admin/clusters/reload`.

## Out of scope

- Logger hot-reload (restart-required).
- Admin-frontend settings page UI (separate follow-up; documented).
- Cluster gitlab-OIDC connection pooling.
- `podspec` unit-test migration (unchanged).
