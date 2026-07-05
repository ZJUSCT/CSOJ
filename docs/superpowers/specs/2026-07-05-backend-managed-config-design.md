# Backend-Managed Config & Data — Design

**Date:** 2026-07-05
**Scope:** Move all runtime-mutable configuration and contest/problem/asset data from the filesystem into the database, so the CSOJ backend manages and modifies everything. `config.yaml` shrinks to pure bootstrap + connection fields. No on-disk `contests_root` tree; no `contest.yaml`/`problem.yaml`/`announcements.yaml` writes.

This builds on the merged single-repo state on branch `merge-webui-admin`.

## Goals

1. **Contest/problem/announcement metadata → DB.** New GORM models `Contest`, `Problem`, `Announcement`. Admin CRUD writes to the DB; `appState` is rebuilt from the DB, not from disk.
2. **Assets → DB.** Asset bytes live in a BLOB column (`assets` table). The `index.assets/` filesystem tree is gone.
3. **Cluster node resource caps + links → DB, admin-mutable.** New `cluster_nodes` table (`cpu`/`memory`) and `links` table. The Docker connection per node (`docker.host`, TLS) stays in `config.yaml` because the judger needs it before the DB is up.
4. **`config.yaml` = bootstrap + connect only.** `listen`, `logger`, `storage` (paths), `auth` (jwt/gitlab/local), `cors`, and `cluster[].node[].docker.*` (+ `name`). Nothing else. No `contests_root`, no `links`, no cluster `cpu`/`memory`.
5. **No migration path.** Fresh start — the DB starts empty; admins recreate contests via the admin UI. Existing on-disk `contests/` dirs are ignored.

## Non-goals

- Migrating existing on-disk contests/problems/assets into the DB (explicitly declined; fresh start).
- Splitting the `Node` struct's `docker.*` from `cpu`/`memory` at the config-yaml *level* beyond what's needed — `config.yaml` keeps `name` + `docker.*` per node; `cpu`/`memory` move to the DB.
- Changing the judger's Docker execution model (workflow steps, mounts, scheduling).
- Changing user-facing API shapes (the `/api/v1/contests`, `/problems`, etc. responses stay the same — only their source changes from disk to DB).
- Touching the frontend for this phase (the admin UI already calls the same endpoints; it keeps working).

---

## Architecture

### Current state (recap)

- `config.yaml` read once at boot (`config.Load`), shared read-only. Fields: `listen`, `logger`, `storage`, `auth`, `cors`, `cluster` (full node block incl. cpu/memory/docker), `links`, `contests_root`.
- Contests/problems/announcements: 100% file-based. `judger/loader.go` reads `<contests_root>/<id>/contest.yaml` + `index.md` + `announcements.yaml` + `<problem>/problem.yaml` into `appState.Contests`/`Problems`/`ProblemToContestMap`. `judger/fs.go` writes them on admin CRUD. `reload` re-scans disk.
- Assets: pure filesystem under `<base>/index.assets/`. No DB record.
- `cluster`: `cfg.Cluster` read at boot; `NewScheduler` builds `ClusterState`/`NodeState` from it. `PauseNode`/`ResumeNode` flip an in-memory flag only.
- `links`: `cfg.Links`, returned read-only by `GET /api/v1/links`. No admin endpoint.
- DB models: `User`, `Submission`, `Container`, `ContestScoreHistory`, `UserProblemBestScore`. No `Contest`/`Problem`/`Announcement`/`Asset`/`ClusterNode`/`Link` models.

### Target state

- `config.yaml` fields: `listen`, `logger`, `storage` (incl. a new optional `assets` path, unused for bytes but kept for future), `auth`, `cors`, `cluster` (name + nodes with name + docker.* only). Removed: `contests_root`, `links`, and `cpu`/`memory` from each node.
- New GORM models in `internal/database/models/models.go`:
  - `Contest` — id, name, starttime, endtime, description, problem_ids (JSON array), created_at, updated_at, deleted_at.
  - `Problem` — id, contest_id (FK), name, level, starttime, endtime, max_submissions, cluster, cpu, memory, upload (JSON), workflow (JSON), score (JSON), description, created_at, updated_at.
  - `Announcement` — id, contest_id (FK), title, description, created_at, updated_at.
  - `Asset` — id, owner_type ("contest"|"problem"), owner_id, path (relative), is_dir, size, mod_time, content (BLOB, nullable for dirs).
  - `ClusterNode` — cluster_name, node_name, cpu, memory (composite PK cluster_name+node_name). Joined with `config.yaml`'s docker.* at boot.
  - `Link` — id, name, url, position (int, for ordering).
- `appState` is rebuilt from the DB (not disk) by a new `judger.LoadFromDB(db)` (or a `reload` that reads DB). `Contest.BasePath`/`Problem.BasePath` disappear — assets are resolved via `Asset` rows.
- `judger/fs.go` is deleted (no disk writes for contest/problem). `judger/loader.go`'s `FindContestDirs`/`LoadAllContestsAndProblems` are deleted; replaced by DB queries.
- Asset serving: `serveContestAsset`/`serveProblemAsset` read the `Asset` BLOB from the DB and write it to the response (with `Content-Disposition`). `handleListContestAssets` lists `Asset` rows. Upload/delete insert/delete `Asset` rows.
- Cluster: `NewScheduler` reads `cfg.Cluster` (docker.* per node) AND `ClusterNode` rows (cpu/memory) from the DB, merging by `(cluster_name, node_name)`. New admin endpoints mutate `ClusterNode` (cpu/memory) — but **cannot add a node's docker connection** (that's config-time). Adding a *new* node requires editing config.yaml and restarting; the admin UI can only edit cpu/memory of existing nodes.
- Links: new admin CRUD endpoints under `/api/v1/admin/links`; `GET /api/v1/links` reads from the DB.

---

## Data model (new GORM models)

`internal/database/models/models.go` — append:

```go
type Contest struct {
	ID          string `gorm:"primaryKey" json:"id"`
	Name        string `json:"name"`
	StartTime   time.Time `gorm:"index" json:"starttime"`
	EndTime     time.Time `json:"endtime"`
	Description string `gorm:"type:text" json:"description"`
	ProblemIDs  StringArray `gorm:"type:text" json:"problem_ids"` // ordered list of problem IDs
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`

	Announcements []Announcement `gorm:"foreignKey:ContestID;constraint:OnDelete:CASCADE" json:"-"`
}

type Problem struct {
	ID            string `gorm:"primaryKey" json:"id"`
	ContestID     string `gorm:"index" json:"contest_id"`
	Name          string `json:"name"`
	Level         string `json:"level"`
	StartTime     time.Time `json:"starttime"`
	EndTime       time.Time `json:"endtime"`
	MaxSubmissions int `json:"max_submissions"`
	Cluster       string `gorm:"index" json:"cluster"`
	CPU           int `json:"cpu"`
	Memory        int64 `json:"memory"`
	Upload        JSONMap `gorm:"type:text" json:"upload"`
	Workflow      JSONMap `gorm:"type:text" json:"workflow"`
	Score         JSONMap `gorm:"type:text" json:"score"`
	Description   string `gorm:"type:text" json:"description"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Announcement struct {
	ID          string `gorm:"primaryKey" json:"id"`
	ContestID   string `gorm:"index" json:"contest_id"`
	Title       string `json:"title"`
	Description string `gorm:"type:text" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Asset struct {
	ID         string `gorm:"primaryKey" json:"-"`
	OwnerType  string `gorm:"index:idx_owner" json:"-"` // "contest" | "problem"
	OwnerID    string `gorm:"index:idx_owner" json:"-"`  // contest_id or problem_id
	Path       string `gorm:"index:idx_owner" json:"path"` // relative path within the asset tree
	IsDir      bool `json:"is_dir"`
	Size       int64 `json:"size"`
	ModTime    time.Time `json:"mod_time"`
	Content    []byte `gorm:"type:blob" json:"-"`
}

type ClusterNode struct {
	ClusterName string `gorm:"primaryKey" json:"cluster_name"`
	NodeName    string `gorm:"primaryKey" json:"node_name"`
	CPU         int `json:"cpu"`
	Memory      int64 `json:"memory"`
}

type Link struct {
	ID       uint `gorm:"primaryKey" json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Position int `json:"position"`
}
```

`StringArray` — a helper type over `[]string` with `Value`/`Scan` (comma-separated or JSON in a text column), mirroring the existing `JSONMap` pattern. Defined once in `models.go`.

`Problem.Upload`/`Workflow`/`Score` use the existing `JSONMap` type (already defined) — they're stored as JSON text and (un)marshalled by the judger when building the in-memory `judger.Problem`. Alternatively, dedicated `UploadLimit`/`WorkflowStep` types with `Value`/`Scan` — but `JSONMap` is simpler and the judger already re-parses these from the in-memory struct. Decision: store as `JSONMap`; the loader marshals/unmarshales to the typed `judger.Problem` struct. (This avoids a schema change if the workflow shape evolves.)

### Migration of `appState`

`judger.AppState` keeps `Contests`/`Problems`/`ProblemToContestMap` maps, but they're now `[]*judger.Contest`/`[]*judger.Problem` built from DB rows (not disk). A new `judger.LoadFromDB(db) (contests, problems, problemToContestMap, error)` replaces `LoadAllContestsAndProblems`. The `judger.Contest`/`judger.Problem` structs keep their shape (the judger consumes them) but `BasePath` is removed (unused — assets come from the DB now).

---

## Config changes

### `internal/config/config.go`

```go
type Config struct {
	Cluster []Cluster `yaml:"cluster"`
	Logger  Logger    `yaml:"logger"`
	Storage Storage   `yaml:"storage"`
	Auth    Auth      `yaml:"auth"`
	Listen  string    `yaml:"listen"`
	CORS    CORS      `yaml:"cors"`
	// Removed: ContestsRoot, Links
}

type Cluster struct {
	Name  string `yaml:"name" json:"name"`
	Nodes []Node `yaml:"node" json:"node"`
}

type Node struct {
	Name   string       `yaml:"name" json:"name"`     // matches ClusterNode.NodeName
	Docker DockerConfig `yaml:"docker" json:"docker"` // connection only
	// Removed: CPU, Memory (now in ClusterNode DB table)
}
```

`config.Load` unchanged (read-only). `contests_root` and `links` removed from the struct; a leftover `contests_root:`/`links:` key in a yaml is silently ignored by `yaml.Unmarshal` (safe).

`main.go` no longer calls `judger.FindContestDirs`/`LoadAllContestsAndProblems`. It calls `judger.LoadFromDB(db)` to populate `appState` after the DB is up.

### Scheduler rebuild

`NewScheduler(cfg, db, appState)` now:
1. Reads `cfg.Cluster` (docker.* per node).
2. Reads `ClusterNode` rows from the DB (cpu/memory).
3. Merges: for each `cfg.Cluster` node, find the matching `ClusterNode` by `(cluster.Name, node.Name)`; build `NodeState` with `cpu`/`memory` from the DB row and `docker.*` from config. If no DB row exists, skip the node (log) — the node isn't "enabled" until an admin adds its resource caps.
4. Builds `ClusterState`/`queues` as today.

A new admin endpoint `PUT /api/v1/admin/clusters/:clusterName/nodes/:nodeName` updates the `ClusterNode` row (cpu/memory) and refreshes the in-memory `NodeState`. `PauseNode`/`ResumeNode` unchanged (in-memory flag). No endpoint to add/remove a node's docker connection (config-time only).

---

## Judger changes

### `internal/judger/loader.go`

- Delete `FindContestDirs`, `LoadAllContestsAndProblems`, `loadContest`, `loadProblem`.
- Add `LoadFromDB(db *gorm.DB) (map[string]*Contest, map[string]*Problem, map[string]*Problem, error)`:
  - `db.Find(&allContests)` → build `judger.Contest` structs (translate `ProblemIDs` from `StringArray`).
  - `db.Find(&allProblems)` → build `judger.Problem` structs (unmarshal `Upload`/`Workflow`/`Score` from `JSONMap` into the typed fields).
  - Build `ProblemToContestMap` from `Problem.ContestID`.
- `judger.Contest`/`judger.Problem`: remove `BasePath` field (no longer used). `Contest.Announcements` is now loaded eagerly from the `Announcement` rows (or on demand — but eager is simpler and matches today's behavior).

### `internal/judger/fs.go`

- **Delete the file entirely.** `CreateContest`/`UpdateContest`/`DeleteContest`/`CreateProblem`/`UpdateProblem`/`DeleteProblem` are gone — admin handlers now write to the DB directly.

---

## Admin handler changes

### `internal/api/admin/management.go` (`reload`)

Now reads from the DB:
```go
newContests, newProblems, newProblemToContestMap, err := judger.LoadFromDB(h.db)
// ... swap appState under lock ...
```
No more `FindContestDirs`. Still cleans up submissions whose problems were deleted (same DB query as today).

### `internal/api/admin/contest.go`

- `createContest`: `db.Create(&models.Contest{...})`, then `reload`.
- `updateContest`: load, mutate, `db.Save`, then `reload`.
- `deleteContest`: `db.Delete`, then `reload`.
- `createProblemInContest`: `db.Create(&models.Problem{ContestID: ...})`, append to contest's `ProblemIDs` (as a `StringArray`), `db.Save` the contest, then `reload`.
- `handleUpdateContestProblemOrder`: reorder `ProblemIDs` on the `Contest` row, `db.Save`, then `reload`.

The handlers no longer touch `judger/fs.go`. They translate between the JSON API shape and the GORM models (or, simplest path: have the API bind directly to the GORM model where shapes match — but keep the existing request/response shapes to avoid a frontend change).

### `internal/api/admin/problem.go`

- `updateProblem`/`deleteProblem`: `db.Save`/`db.Delete`, then `reload`. The parent contest's `ProblemIDs` is updated on delete (filter the array, `db.Save`).

### `internal/api/admin/announcement.go`

- Delete `readAnnouncementsFile`/`writeAnnouncementsFile` (disk helpers).
- `handleGetContestAnnouncements`: `db.Where("contest_id = ?", id).Find(&announcements)`.
- `handleCreateContestAnnouncement`: `db.Create(&models.Announcement{...})`.
- `handleUpdateContestAnnouncement`/`handleDeleteContestAnnouncement`: `db.Save`/`db.Delete`.

### `internal/api/admin/assets.go`

- `handleListContestAssets`/`handleListProblemAssets`: `db.Where("owner_type = ? AND owner_id = ? AND path LIKE ?", ...).Find(&assets)` — list `Asset` rows.
- `handleUploadContestAssets`/`handleUploadProblemAssets`: for each uploaded file, `db.Create(&models.Asset{OwnerType, OwnerID, Path, Content: bytes, ...})`. Subdirectory rows (`is_dir=true`) created as needed (or the `path` field encodes the full relative path and dirs are inferred at list time — simpler: store every file as a row with its full relative path; list by walking the `path` prefix and grouping).
- `handleDeleteContestAsset`/`handleDeleteProblemAsset`: `db.Where("owner_type=? AND owner_id=? AND path LIKE ?", ...).Delete(&models.Asset{})` (delete a file or a subtree via path prefix).
- `serveContestAsset`/`serveProblemAsset` (admin): `db.First(&asset, ...)` → `c.Data(contentType, asset.Content)` with `Content-Disposition`.

### `internal/api/admin/cluster.go`

- `getClusterStatus`/`getNodeDetails`/`pauseNode`/`resumeNode` unchanged (read in-memory state).
- **New** `PUT /api/v1/admin/clusters/:clusterName/nodes/:nodeName`: update `ClusterNode` (cpu/memory) row, then refresh the in-memory `NodeState` (and re-init the `UsedCores` slice to the new length).

### `internal/api/admin/links.go` (new file)

- `GET /api/v1/admin/links` — list all `Link` rows ordered by `position`.
- `POST /api/v1/admin/links` — create.
- `PUT /api/v1/admin/links/:id` — update.
- `DELETE /api/v1/admin/links/:id` — delete.

### `internal/api/admin/router.go`

Register the new `links` group and the `PUT cluster node` route under `adminV1`.

---

## User-facing handler changes

### `internal/api/user/contest.go`

- `getLinks`: `db.Find(&links)` instead of `h.cfg.Links`.
- `getAllContests`/`getContest`: read from `appState` (already in-memory; source is now DB, but the handler code is unchanged).
- `getContestAnnouncements`: `appState.Contests[id].Announcements` (still in-memory, now sourced from DB).

### `internal/api/user/asset.go`

- `serveContestAsset`/`serveProblemAsset`: replace the `os.ReadFile(contest.BasePath/...)` with a DB lookup: `db.Where("owner_type=? AND owner_id=? AND path=?", ...).First(&asset)` → `c.Data(...)`. The HMAC signed-URL flow (`queryAssetURL`) is unchanged — the signed path still points at `/api/v1/assets/contests/:id/*assetpath`, and `AssetsAuthMiddleware` + the handler resolve it to a DB row.
- The start-time authorization check on `serveProblemAsset` is unchanged (uses `appState` start times).

---

## Data flow

- **Boot:** `config.Load` → `database.Init` (AutoMigrate creates the new tables) → `judger.LoadFromDB(db)` populates `appState` → `judger.NewScheduler(cfg, db, appState)` merges config-docker + DB-cpu/memory → server starts.
- **Admin creates a contest:** `POST /api/v1/admin/contests` → `db.Create` → `reload` (re-reads DB into `appState`).
- **Admin uploads an asset:** `POST /api/v1/admin/contests/:id/assets` → `db.Create(&Asset{...})` for each file → list endpoint sees it.
- **User fetches an asset:** `GET /api/v1/assets/contests/:id/<path>?token=...` → `AssetsAuthMiddleware` → `serveContestAsset` → `db.First(&Asset, ...)` → `c.Data(content, bytes)`.
- **Admin edits a node's cpu:** `PUT /api/v1/admin/clusters/:c/nodes/:n` → `db.Save(&ClusterNode)` → refresh in-memory `NodeState` → judger uses new cpu count for future submissions.

## Error handling

- `LoadFromDB` failure at boot is fatal (same as today's disk-scan failure).
- A `Contest` row with a `ProblemIDs` entry pointing at a missing `Problem` is logged and skipped (mirrors today's "failed to load problem" warning).
- Asset DB lookup miss → 404 (same as today's `os.IsNotExist`).
- `ClusterNode` row missing for a config-defined node → node is skipped at boot (logged); admin can add it via `PUT` (which `Upsert`s).

## Testing / verification

No existing Go test suite. Manual verification:
1. Fresh DB → `make build` → run → `appState` empty, no errors.
2. Admin creates a contest + problem via the API → DB rows created → `GET /api/v1/contests` returns it.
3. Admin uploads an asset → `Asset` row created → user fetches it via signed URL → bytes match.
4. Admin edits a node's cpu via `PUT` → in-memory `NodeState` updated → next submission uses the new cpu count.
5. Admin creates/edits a link → `GET /api/v1/links` reflects it.
6. `config.yaml` with a leftover `contests_root:`/`links:` key → ignored (no error).
7. Restart the server → all data persists (DB is the source of truth).

A `verify` pass will exercise: contest create, problem create, asset upload + fetch, node cpu edit, link CRUD.

---

## Out of scope

- Migration of existing on-disk data (fresh start).
- Frontend changes (admin UI already calls these endpoints; shapes preserved).
- Splitting `docker.*` further (it stays as one block in config.yaml per node).
- Adding new nodes at runtime (requires config.yaml edit + restart — the docker connection is config-time).
- Changing the judger's workflow/mount/scheduling model.
- Multi-node Docker connection discovery (still static in config.yaml).
