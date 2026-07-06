# Backend-Managed Config & Data — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move contests/problems/announcements/assets/cluster-cpu/links from the filesystem into the database, so the backend manages everything; `config.yaml` shrinks to bootstrap + Docker-connection fields only.

**Architecture:** New GORM models (`Contest`, `Problem`, `Announcement`, `Asset`, `ClusterNode`, `Link`) replace the on-disk YAML/filesystem store. `appState` is rebuilt from the DB via a new `judger.LoadFromDB`. `judger/fs.go` and the disk-loader functions are deleted. The scheduler merges `config.yaml`'s per-node Docker connection with `ClusterNode` rows (cpu/memory) from the DB. Admin CRUD handlers write to the DB; user asset/links handlers read from the DB.

**Tech Stack:** Go 1.24, Gin, GORM/SQLite, golang-jwt/v5.

**Reference spec:** `docs/superpowers/specs/2026-07-05-backend-managed-config-design.md`

**Branch:** `merge-webui-admin` (continues from the WebUI+AdminPanel merge).

---

## File Structure

### Backend (Go)

- **Modify** `internal/database/models/models.go` — add `StringArray` type + `Contest`/`Problem`/`Announcement`/`Asset`/`ClusterNode`/`Link` models
- **Modify** `internal/database/database.go` — `AutoMigrate` the new models
- **Modify** `internal/database/crud.go` — add `Link`/`ClusterNode` CRUD helpers
- **Create** `internal/judger/db.go` — `LoadFromDB` + model↔judger struct converters
- **Modify** `internal/judger/loader.go` — remove `BasePath`/`ProblemDirs`; delete `FindContestDirs`/`LoadAllContestsAndProblems`/`loadContest`/`loadProblem`
- **Delete** `internal/judger/fs.go`
- **Modify** `internal/judger/scheduler.go` — `NewScheduler` merges config-docker + DB-cpu/memory; `NodeState` carries `CPU`/`Memory` directly; new `UpdateNodeResources` method
- **Modify** `internal/config/config.go` — remove `ContestsRoot`/`Links`; remove `CPU`/`Memory` from `Node`
- **Modify** `internal/api/admin/management.go` — `reload` reads from DB
- **Modify** `internal/api/admin/contest.go` — CRUD → DB
- **Modify** `internal/api/admin/problem.go` — CRUD → DB
- **Modify** `internal/api/admin/announcement.go` — CRUD → DB; delete disk helpers
- **Modify** `internal/api/admin/assets.go` — list/upload/delete/serve → DB
- **Modify** `internal/api/admin/cluster.go` — add `updateNode` handler
- **Modify** `internal/api/admin/router.go` — register `links` group + `PUT node` route
- **Create** `internal/api/admin/links.go` — links CRUD
- **Modify** `internal/api/user/contest.go` — `getLinks` reads from DB
- **Modify** `internal/api/user/asset.go` — `serveContestAsset`/`serveProblemAsset` read from DB
- **Modify** `cmd/CSOJ/main.go` — boot calls `LoadFromDB` instead of disk scan
- **Modify** docs: `configuration/main-config.md`, `configuration/contest-config.md`, `configuration/problem-config.md`, `getting-started.md`, `api-reference/admin-api.md`

### Out of scope
- Frontend changes (API shapes preserved).
- Migration of existing on-disk data (fresh start).
- Adding/removing Docker node connections at runtime (config-time only).

---

## Phase A: Data models

### Task A1: Add `StringArray` GORM type + new models

**Files:**
- Modify: `internal/database/models/models.go`

- [ ] **Step 1: Add the `StringArray` helper type**

Append near the top of `internal/database/models/models.go` (after the `JSONMap` type's methods):

```go
// StringArray is a []string stored as a JSON text column.
type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return "[]", nil
	}
	b, err := json.Marshal(a)
	return string(b), err
}

func (a *StringArray) Scan(value interface{}) error {
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, a)
	case string:
		return json.Unmarshal([]byte(v), a)
	}
	return nil
}
```

(Uses the already-imported `encoding/json` and `database/sql/driver`.)

- [ ] **Step 2: Append the new GORM models**

Append at the end of `internal/database/models/models.go`:

```go
// Contest is a backend-managed contest definition.
type Contest struct {
	ID          string     `gorm:"primaryKey" json:"id"`
	Name        string     `json:"name"`
	StartTime   time.Time  `gorm:"index" json:"starttime"`
	EndTime     time.Time  `json:"endtime"`
	Description string     `gorm:"type:text" json:"description"`
	ProblemIDs  StringArray `gorm:"type:text" json:"problem_ids"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// Problem is a backend-managed problem definition.
type Problem struct {
	ID             string  `gorm:"primaryKey" json:"id"`
	ContestID      string  `gorm:"index" json:"contest_id"`
	Name           string  `json:"name"`
	Level          string  `json:"level"`
	StartTime      time.Time `json:"starttime"`
	EndTime        time.Time `json:"endtime"`
	MaxSubmissions int     `json:"max_submissions"`
	Cluster        string  `gorm:"index" json:"cluster"`
	CPU            int     `json:"cpu"`
	Memory         int64   `json:"memory"`
	Upload         JSONMap `gorm:"type:text" json:"upload"`
	Workflow       JSONMap `gorm:"type:text" json:"workflow"`
	Score          JSONMap `gorm:"type:text" json:"score"`
	Description    string  `gorm:"type:text" json:"description"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Announcement is a contest-scoped announcement.
type Announcement struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	ContestID   string    `gorm:"index" json:"contest_id"`
	Title       string    `json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Asset is a contest/problem asset stored as a BLOB.
type Asset struct {
	ID        string    `gorm:"primaryKey" json:"-"`
	OwnerType string    `gorm:"index:idx_asset_owner" json:"-"` // "contest" | "problem"
	OwnerID   string    `gorm:"index:idx_asset_owner" json:"-"`
	Path      string    `gorm:"index:idx_asset_owner" json:"path"` // relative path, forward slashes
	IsDir     bool      `json:"is_dir"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
	Content   []byte    `gorm:"type:blob" json:"-"`
}

// ClusterNode holds the runtime-mutable resource caps for a node.
// The Docker connection (host, TLS) lives in config.yaml and is merged at boot.
type ClusterNode struct {
	ClusterName string `gorm:"primaryKey" json:"cluster_name"`
	NodeName    string `gorm:"primaryKey" json:"node_name"`
	CPU         int    `json:"cpu"`
	Memory      int64  `json:"memory"`
}

// Link is a nav-bar link managed via the admin API.
type Link struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Position int    `json:"position"`
}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/database/models/models.go
git commit -m "feat(db): add Contest/Problem/Announcement/Asset/ClusterNode/Link models"
```

### Task A2: AutoMigrate the new models

**Files:**
- Modify: `internal/database/database.go:30-36`

- [ ] **Step 1: Add the new models to `AutoMigrate`**

In `internal/database/database.go`, replace the `db.AutoMigrate(...)` call:

```go
	err = db.AutoMigrate(
		&models.User{},
		&models.Submission{},
		&models.Container{},
		&models.ContestScoreHistory{},
		&models.UserProblemBestScore{},
		&models.Contest{},
		&models.Problem{},
		&models.Announcement{},
		&models.Asset{},
		&models.ClusterNode{},
		&models.Link{},
	)
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/database/database.go
git commit -m "feat(db): AutoMigrate new backend-managed tables"
```

### Task A3: Add `Link` and `ClusterNode` CRUD helpers

**Files:**
- Modify: `internal/database/crud.go`

- [ ] **Step 1: Append the helper functions**

Append to `internal/database/crud.go`:

```go
// --- Links ---

func GetAllLinks(db *gorm.DB) ([]models.Link, error) {
	var links []models.Link
	err := db.Order("position asc").Find(&links).Error
	return links, err
}

func CreateLink(db *gorm.DB, link *models.Link) error {
	return db.Create(link).Error
}

func UpdateLink(db *gorm.DB, link *models.Link) error {
	return db.Save(link).Error
}

func DeleteLink(db *gorm.DB, id uint) error {
	return db.Delete(&models.Link{}, id).Error
}

// --- Cluster nodes ---

func GetAllClusterNodes(db *gorm.DB) ([]models.ClusterNode, error) {
	var nodes []models.ClusterNode
	err := db.Find(&nodes).Error
	return nodes, err
}

func UpsertClusterNode(db *gorm.DB, node *models.ClusterNode) error {
	return db.Save(node).Error
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/database/crud.go
git commit -m "feat(db): add Link and ClusterNode CRUD helpers"
```

---

## Phase B: Judger loads from DB

### Task B1: Trim `judger.Contest`/`judger.Problem` (remove `BasePath`/`ProblemDirs`)

**Files:**
- Modify: `internal/judger/loader.go:22-32` (Contest struct), `73-88` (Problem struct)

- [ ] **Step 1: Replace the `Contest` struct**

In `internal/judger/loader.go`, replace the `Contest` struct:

```go
type Contest struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	StartTime     time.Time       `json:"starttime"`
	EndTime       time.Time       `json:"endtime"`
	ProblemIDs    []string        `json:"problem_ids"`
	Description   string          `json:"description"`
	Announcements []*Announcement `json:"announcements"`
}
```

(`BasePath`, `ProblemDirs` removed — no longer disk-backed.)

- [ ] **Step 2: Replace the `Problem` struct**

In the same file, replace the `Problem` struct:

```go
type Problem struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Level          string         `json:"level"`
	StartTime      time.Time      `json:"starttime"`
	EndTime        time.Time      `json:"endtime"`
	MaxSubmissions int            `json:"max_submissions"`
	Cluster        string         `json:"cluster"`
	CPU            int            `json:"cpu"`
	Memory         int64          `json:"memory"`
	Upload         UploadLimit    `json:"upload"`
	Workflow       []WorkflowStep `json:"workflow"`
	Score          ScoreConfig    `json:"score"`
	Description    string         `json:"description"`
}
```

(`BasePath` removed. The `yaml` tags are gone too — no longer read from YAML.)

- [ ] **Step 3: Verify the package fails to compile (expected — loaders reference removed fields)**

Run: `go build ./internal/judger/`
Expected: compile errors in `loader.go` (`contest.BasePath`, `contest.ProblemDirs`, `problem.BasePath`) and `fs.go`. These are fixed in B2/B3. Do not commit yet.

### Task B2: Delete `loader.go` disk functions; create `db.go` with `LoadFromDB`

**Files:**
- Modify: `internal/judger/loader.go` (delete disk functions, keep struct/types)
- Create: `internal/judger/db.go`

- [ ] **Step 1: Delete the disk-loader functions from `loader.go`**

In `internal/judger/loader.go`, delete the functions `FindContestDirs`, `LoadAllContestsAndProblems`, `loadContest`, `loadProblem` (lines ~90-202). Keep the type definitions at the top (`Announcement`, `Contest`, `UploadLimit`, `TmpfsOptions`, `Mount`, `WorkflowStep`, `ScoreConfig`, `Problem`).

After deletion, remove now-unused imports (`fmt`, `os`, `path/filepath`, `sort`, `go.uber.org/zap`, `gopkg.in/yaml.v3`) — leave only `time` (used by the structs). The file should now contain only the type definitions and the `time` import.

- [ ] **Step 2: Create `internal/judger/db.go`**

Create `internal/judger/db.go`:

```go
package judger

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"gorm.io/gorm"
)

// LoadFromDB rebuilds the in-memory contest/problem state from the database.
func LoadFromDB(db *gorm.DB) (map[string]*Contest, map[string]*Problem, map[string]*Contest, error) {
	var dbContests []models.Contest
	if err := db.Find(&dbContests).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load contests: %w", err)
	}

	var dbProblems []models.Problem
	if err := db.Find(&dbProblems).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load problems: %w", err)
	}

	var dbAnnouncements []models.Announcement
	if err := db.Find(&dbAnnouncements).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load announcements: %w", err)
	}

	annByContest := make(map[string][]*Announcement)
	for _, a := range dbAnnouncements {
		annByContest[a.ContestID] = append(annByContest[a.ContestID], &Announcement{
			ID:          a.ID,
			Title:       a.Title,
			Description: a.Description,
			CreatedAt:   a.CreatedAt,
			UpdatedAt:   a.UpdatedAt,
		})
	}

	contests := make(map[string]*Contest, len(dbContests))
	for _, c := range dbContests {
		anns := annByContest[c.ID]
		sort.Slice(anns, func(i, j int) bool { return anns[i].CreatedAt.After(anns[j].CreatedAt) })
		contests[c.ID] = &Contest{
			ID:            c.ID,
			Name:          c.Name,
			StartTime:     c.StartTime,
			EndTime:       c.EndTime,
			ProblemIDs:    append([]string(nil), c.ProblemIDs...),
			Description:   c.Description,
			Announcements: anns,
		}
	}

	problems := make(map[string]*Problem, len(dbProblems))
	problemToContest := make(map[string]*Contest)
	for _, p := range dbProblems {
		prob, err := problemFromModel(p)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to parse problem %s: %w", p.ID, err)
		}
		problems[p.ID] = prob
		if parent, ok := contests[p.ContestID]; ok {
			problemToContest[p.ID] = parent
		}
	}

	return contests, problems, problemToContest, nil
}

func problemFromModel(p models.Problem) (*Problem, error) {
	prob := &Problem{
		ID:             p.ID,
		Name:           p.Name,
		Level:          p.Level,
		StartTime:      p.StartTime,
		EndTime:        p.EndTime,
		MaxSubmissions: p.MaxSubmissions,
		Cluster:        p.Cluster,
		CPU:            p.CPU,
		Memory:         p.Memory,
		Description:    p.Description,
	}
	if p.Upload != nil {
		var u UploadLimit
		if err := unmarshalJSONMap(p.Upload, &u); err == nil {
			prob.Upload = u
		}
	}
	if p.Workflow != nil {
		var w []WorkflowStep
		if err := unmarshalJSONMap(p.Workflow, &w); err == nil {
			prob.Workflow = w
		}
	}
	if p.Score != nil {
		var s ScoreConfig
		if err := unmarshalJSONMap(p.Score, &s); err == nil {
			prob.Score = s
		}
	}
	if prob.Score.Mode == "" {
		prob.Score.Mode = "score"
	}
	return prob, nil
}

// ProblemToModel converts a judger.Problem into a models.Problem for DB persistence.
func ProblemToModel(p *Problem, contestID string) (models.Problem, error) {
	upload, err := toJSONMap(p.Upload)
	if err != nil {
		return models.Problem{}, err
	}
	workflow, err := toJSONMap(p.Workflow)
	if err != nil {
		return models.Problem{}, err
	}
	score, err := toJSONMap(p.Score)
	if err != nil {
		return models.Problem{}, err
	}
	return models.Problem{
		ID:             p.ID,
		ContestID:      contestID,
		Name:           p.Name,
		Level:          p.Level,
		StartTime:      p.StartTime,
		EndTime:        p.EndTime,
		MaxSubmissions: p.MaxSubmissions,
		Cluster:        p.Cluster,
		CPU:            p.CPU,
		Memory:         p.Memory,
		Upload:         upload,
		Workflow:       workflow,
		Score:          score,
		Description:    p.Description,
	}, nil
}

func toJSONMap(v interface{}) (models.JSONMap, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m models.JSONMap
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func unmarshalJSONMap(m models.JSONMap, dst interface{}) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// ContestToModel converts a judger.Contest into a models.Contest for DB persistence.
func ContestToModel(c *Contest) models.Contest {
	return models.Contest{
		ID:          c.ID,
		Name:        c.Name,
		StartTime:   c.StartTime,
		EndTime:     c.EndTime,
		Description: c.Description,
		ProblemIDs:  models.StringArray(append([]string(nil), c.ProblemIDs...)),
	}
}
```

- [ ] **Step 3: Verify the judger package compiles**

Run: `go build ./internal/judger/`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/judger/loader.go internal/judger/db.go
git commit -m "feat(judger): load contests/problems from DB; drop disk loaders"
```

### Task B3: Delete `internal/judger/fs.go`

**Files:**
- Delete: `internal/judger/fs.go`

- [ ] **Step 1: Delete the file**

Run: `git rm internal/judger/fs.go`

- [ ] **Step 2: Verify the package compiles**

Run: `go build ./internal/judger/`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git commit -m "refactor(judger): delete disk-based contest/problem fs.go"
```

---

## Phase C: Scheduler merges config + DB node caps

### Task C1: Move `CPU`/`Memory` out of `config.Node`; add to `NodeState`

**Files:**
- Modify: `internal/config/config.go` (Node struct)
- Modify: `internal/judger/scheduler.go` (NodeState/NodeDetail)

- [ ] **Step 1: Trim `config.Node`**

In `internal/config/config.go`, replace the `Node` struct:

```go
type Node struct {
	Name   string       `yaml:"name" json:"name"`
	Docker DockerConfig `yaml:"docker" json:"docker"`
}
```

(`CPU`/`Memory` removed — now in the `ClusterNode` DB table.)

- [ ] **Step 2: Add `CPU`/`Memory` to `NodeState` and `NodeDetail`**

In `internal/judger/scheduler.go`, replace the `NodeState` and `NodeDetail` structs:

```go
type NodeState struct {
	sync.Mutex
	Name   string           `json:"name"`
	Docker config.DockerConfig `json:"docker"`
	CPU    int              `json:"cpu"`
	Memory int64            `json:"memory"`
	UsedMemory int64  `json:"used_memory"`
	UsedCores  []bool `json:"used_cores"`
	IsPaused   bool   `json:"is_paused"`
}

type NodeDetail struct {
	Name   string           `json:"name"`
	Docker config.DockerConfig `json:"docker"`
	CPU    int              `json:"cpu"`
	Memory int64            `json:"memory"`
	UsedMemory int64  `json:"used_memory"`
	UsedCores  []bool `json:"used_cores"`
	IsPaused   bool   `json:"is_paused"`
}
```

- [ ] **Step 3: Verify the package fails to compile (expected)**

Run: `go build ./internal/judger/`
Expected: errors in `scheduler.go` `NewScheduler` (references `node.CPU`/`node.Memory` via the embedded `*config.Node`) and `GetNodeDetails` (copy of `*node.Node`). Fixed in C2.

### Task C2: `NewScheduler` merges config-docker + DB-cpu/memory

**Files:**
- Modify: `internal/judger/scheduler.go` (`NewScheduler`, `GetNodeDetails`)

- [ ] **Step 1: Rewrite `NewScheduler`**

In `internal/judger/scheduler.go`, replace the `NewScheduler` function:

```go
func NewScheduler(cfg *config.Config, db *gorm.DB, appState *AppState) *Scheduler {
	clusters := make(map[string]*ClusterState)
	queues := make(map[string]chan QueuedSubmission)

	// Load runtime-mutable resource caps from the DB, keyed by (cluster, node).
	dbNodes, err := database.GetAllClusterNodes(db)
	if err != nil {
		zap.S().Fatalf("failed to load cluster nodes from DB: %v", err)
	}
	caps := make(map[string]models.ClusterNode, len(dbNodes))
	for _, n := range dbNodes {
		caps[n.ClusterName+"\x00"+n.NodeName] = n
	}

	for i := range cfg.Cluster {
		cluster := cfg.Cluster[i]
		clusterState := &ClusterState{
			Cluster: &cluster,
			Nodes:   make(map[string]*NodeState),
		}
		for j := range cluster.Nodes {
			node := cluster.Nodes[j]
			cap, ok := caps[cluster.Name+"\x00"+node.Name]
			if !ok {
				zap.S().Warnf("node %s/%s has no DB resource caps; skipping", cluster.Name, node.Name)
				continue
			}
			nodeCores := make([]bool, cap.CPU)
			clusterState.Nodes[node.Name] = &NodeState{
				Name:       node.Name,
				Docker:     node.Docker,
				CPU:        cap.CPU,
				Memory:     cap.Memory,
				UsedMemory: 0,
				UsedCores:  nodeCores,
				IsPaused:   false,
			}
		}
		clusters[cluster.Name] = clusterState
		queues[cluster.Name] = make(chan QueuedSubmission, 1024)
	}

	scheduler := &Scheduler{
		cfg:      cfg,
		db:       db,
		clusters: clusters,
		queues:   queues,
		appState: appState,
	}
	scheduler.dispatcher = NewDispatcher(cfg, db, scheduler, appState)
	return scheduler
}
```

Add the imports `"github.com/ZJUSCT/CSOJ/internal/database"` and `"github.com/ZJUSCT/CSOJ/internal/database/models"` to `scheduler.go` if not already present (`models` is already imported; `database` may not be).

- [ ] **Step 2: Update `GetNodeDetails` to use the new fields**

In `internal/judger/scheduler.go`, find `GetNodeDetails` and replace the body that copies `*node.Node`:

```go
	details := &NodeDetail{
		Name:       node.Name,
		Docker:     node.Docker,
		CPU:        node.CPU,
		Memory:     node.Memory,
		UsedMemory: node.UsedMemory,
		IsPaused:   node.IsPaused,
		UsedCores:  append([]bool(nil), node.UsedCores...),
	}
	return details, nil
```

(Delete the `nodeConfigCopy := *node.Node` line.)

- [ ] **Step 3: Find any other `node.CPU`/`node.Memory`/`node.Node` references in judger and fix**

Run: `grep -n "node\.CPU\|node\.Memory\|\.Node\b\|config\.Node" internal/judger/*.go`
Fix each: references to `node.Node` (the embedded `*config.Node`) become `node.Name`/`node.Docker`/`node.CPU`/`node.Memory` directly. The dispatcher (`dispatcher.go`) likely uses `problem.CPU`/`problem.Memory` (from `judger.Problem`, unchanged) and `node.UsedCores`/`node.UsedMemory`/`node.IsPaused` (unchanged). Verify by reading the hits.

- [ ] **Step 4: Verify the judger package compiles**

Run: `go build ./internal/judger/`
Expected: exit 0.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/judger/scheduler.go
git commit -m "refactor(scheduler): merge config docker + DB cpu/memory per node"
```

### Task C3: Add `Scheduler.UpdateNodeResources`

**Files:**
- Modify: `internal/judger/scheduler.go`

- [ ] **Step 1: Add the method**

Append to `internal/judger/scheduler.go`:

```go
// UpdateNodeResources updates the in-memory CPU/Memory caps for a node
// after an admin edits the ClusterNode DB row.
func (s *Scheduler) UpdateNodeResources(clusterName, nodeName string, cpu int, memory int64) error {
	cluster, ok := s.clusters[clusterName]
	if !ok {
		return fmt.Errorf("cluster '%s' not found", clusterName)
	}
	node, ok := cluster.Nodes[nodeName]
	if !ok {
		return fmt.Errorf("node '%s' not found in cluster '%s'", nodeName, clusterName)
	}
	node.Lock()
	defer node.Unlock()
	node.CPU = cpu
	node.Memory = memory
	// Resize the UsedCores slice; preserve existing usage where possible.
	newCores := make([]bool, cpu)
	copy(newCores, node.UsedCores)
	node.UsedCores = newCores
	return nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/judger/`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/judger/scheduler.go
git commit -m "feat(scheduler): UpdateNodeResources for runtime cpu/memory edits"
```

---

## Phase D: Admin contest/problem/announcement CRUD → DB

### Task D1: `reload` reads from DB

**Files:**
- Modify: `internal/api/admin/management.go`

- [ ] **Step 1: Replace the `reload` function body**

In `internal/api/admin/management.go`, replace the `reload` function:

```go
func (h *Handler) reload(c *gin.Context) {
	zap.S().Info("starting reload process...")

	newContests, newProblems, newProblemToContestMap, err := judger.LoadFromDB(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to load contests/problems from DB: %w", err))
		return
	}
	zap.S().Infof("loaded %d contests and %d problems from DB", len(newContests), len(newProblems))

	newProblemIDs := make(map[string]struct{}, len(newProblems))
	for id := range newProblems {
		newProblemIDs[id] = struct{}{}
	}

	// Find submissions whose problems have been deleted
	var allSubmissions []models.Submission
	if err := h.db.Preload("Containers").Find(&allSubmissions).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to get all submissions: %w", err))
		return
	}

	// Atomically update the shared state
	h.appState.Lock()
	h.appState.Contests = newContests
	h.appState.Problems = newProblems
	h.appState.ProblemToContestMap = newProblemToContestMap
	h.appState.Unlock()
	zap.S().Info("app state reloaded successfully")

	util.Success(c, gin.H{
		"contests_loaded": len(newContests),
		"problems_loaded": len(newProblems),
	}, "Reload successful")
}
```

Remove now-unused imports if any (`models` is still used for `models.Submission`). Keep `fmt`, `net/http`, `judger`, `util`, `gin`, `zap`, `models`.

- [ ] **Step 2: Verify the package compiles**

Run: `go build ./internal/api/admin/`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/api/admin/management.go
git commit -m "refactor(admin): reload reads contests/problems from DB"
```

### Task D2: Contest CRUD → DB

**Files:**
- Modify: `internal/api/admin/contest.go`

- [ ] **Step 1: Replace the contest CRUD handlers**

In `internal/api/admin/contest.go`, replace `createContest`, `updateContest`, `handleUpdateContestProblemOrder`, `deleteContest`, `createProblemInContest` with DB-backed versions. The `getAllContests`/`getContest`/`getContestLeaderboard`/`getContestTrend` handlers stay (they read `appState`, unchanged).

Replace `createContest`:

```go
func (h *Handler) createContest(c *gin.Context) {
	var newContest judger.Contest
	if err := c.ShouldBindJSON(&newContest); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	h.appState.RLock()
	_, exists := h.appState.Contests[newContest.ID]
	h.appState.RUnlock()
	if exists {
		util.Error(c, http.StatusConflict, "a contest with this ID already exists")
		return
	}

	mc := judger.ContestToModel(&newContest)
	if err := h.db.Create(&mc).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to create contest: %w", err))
		return
	}
	zap.S().Infof("admin created contest '%s'", newContest.ID)
	h.reload(c)
}
```

Replace `updateContest`:

```go
func (h *Handler) updateContest(c *gin.Context) {
	contestID := c.Param("id")
	var updatedContest judger.Contest
	if err := c.ShouldBindJSON(&updatedContest); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if contestID != updatedContest.ID {
		util.Error(c, http.StatusBadRequest, "contest ID in path does not match contest ID in body")
		return
	}

	h.appState.RLock()
	existing, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	// Preserve the problem list (managed via problem endpoints / order endpoint)
	updatedContest.ProblemIDs = existing.ProblemIDs
	mc := judger.ContestToModel(&updatedContest)
	if err := h.db.Save(&mc).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update contest: %w", err))
		return
	}
	zap.S().Infof("admin updated contest '%s'", updatedContest.ID)
	h.reload(c)
}
```

Replace `handleUpdateContestProblemOrder`:

```go
func (h *Handler) handleUpdateContestProblemOrder(c *gin.Context) {
	contestID := c.Param("id")
	var req struct {
		ProblemIDs []string `json:"problem_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	newSet := make(map[string]struct{})
	for _, pid := range req.ProblemIDs {
		if _, exists := newSet[pid]; exists {
			util.Error(c, http.StatusBadRequest, fmt.Sprintf("duplicate problem ID in request: %s", pid))
			return
		}
		newSet[pid] = struct{}{}
	}
	origSet := make(map[string]struct{})
	for _, pid := range contest.ProblemIDs {
		origSet[pid] = struct{}{}
	}
	if len(newSet) != len(origSet) {
		util.Error(c, http.StatusBadRequest, "number of problems does not match original")
		return
	}
	for pid := range newSet {
		if _, exists := origSet[pid]; !exists {
			util.Error(c, http.StatusBadRequest, fmt.Sprintf("problem ID %s not found in original contest", pid))
			return
		}
	}

	mc := judger.ContestToModel(contest)
	mc.ProblemIDs = models.StringArray(req.ProblemIDs)
	if err := h.db.Model(&models.Contest{}).Where("id = ?", contestID).Update("problem_ids", mc.ProblemIDs).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update contest problem order: %w", err))
		return
	}
	zap.S().Infof("admin updated problem order for contest '%s'", contestID)
	h.reload(c)
}
```

Replace `deleteContest`:

```go
func (h *Handler) deleteContest(c *gin.Context) {
	contestID := c.Param("id")
	if err := h.db.Delete(&models.Contest{}, "id = ?", contestID).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to delete contest: %w", err))
		return
	}
	zap.S().Warnf("admin deleted contest '%s'", contestID)
	h.reload(c)
}
```

Replace `createProblemInContest`:

```go
func (h *Handler) createProblemInContest(c *gin.Context) {
	contestID := c.Param("id")
	var newProblem judger.Problem
	if err := c.ShouldBindJSON(&newProblem); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusNotFound, "parent contest not found")
		return
	}
	_, problemExists := h.appState.Problems[newProblem.ID]
	h.appState.RUnlock()
	if problemExists {
		util.Error(c, http.StatusConflict, "a problem with this ID already exists")
		return
	}

	mp, err := judger.ProblemToModel(&newProblem, contestID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to marshal problem: %w", err))
		return
	}
	if err := h.db.Create(&mp).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to create problem: %w", err))
		return
	}

	// Append the problem ID to the contest's ordered list
	contest.ProblemIDs = append(contest.ProblemIDs, newProblem.ID)
	mc := judger.ContestToModel(contest)
	if err := h.db.Model(&models.Contest{}).Where("id = ?", contestID).Update("problem_ids", mc.ProblemIDs).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update contest problem list: %w", err))
		return
	}
	zap.S().Infof("admin created problem '%s' in contest '%s'", newProblem.ID, contestID)
	h.reload(c)
}
```

- [ ] **Step 2: Update imports in `contest.go`**

Ensure imports include `"github.com/ZJUSCT/CSOJ/internal/database/models"` and `"github.com/ZJUSCT/CSOJ/internal/judger"`. Remove `"github.com/ZJUSCT/CSOJ/internal/database"` if no longer used (it's used by `getContestLeaderboard`'s `database.GetLeaderboard` — keep it).

- [ ] **Step 3: Verify the package compiles**

Run: `go build ./internal/api/admin/`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/api/admin/contest.go
git commit -m "refactor(admin): contest CRUD writes to DB"
```

### Task D3: Problem CRUD → DB

**Files:**
- Modify: `internal/api/admin/problem.go`

- [ ] **Step 1: Replace `updateProblem` and `deleteProblem`**

In `internal/api/admin/problem.go`, replace the two handlers:

```go
func (h *Handler) updateProblem(c *gin.Context) {
	problemID := c.Param("id")
	var updatedProblem judger.Problem
	if err := c.ShouldBindJSON(&updatedProblem); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if problemID != updatedProblem.ID {
		util.Error(c, http.StatusBadRequest, "problem ID in path does not match problem ID in body")
		return
	}

	h.appState.RLock()
	existing, ok := h.appState.Problems[problemID]
	parentContest, _ := h.appState.ProblemToContestMap[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}

	contestID := ""
	if parentContest != nil {
		contestID = parentContest.ID
	}
	updatedProblem.Contest = existing.Contest // preserve cluster assignment (not in body normally)

	mp, err := judger.ProblemToModel(&updatedProblem, contestID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to marshal problem: %w", err))
		return
	}
	if err := h.db.Save(&mp).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update problem: %w", err))
		return
	}
	zap.S().Infof("admin updated problem '%s'", updatedProblem.ID)
	h.reload(c)
}

func (h *Handler) deleteProblem(c *gin.Context) {
	problemID := c.Param("id")

	h.appState.RLock()
	_, ok := h.appState.Problems[problemID]
	parentContest, contestOk := h.appState.ProblemToContestMap[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	if !contestOk || parentContest == nil {
		util.Error(c, http.StatusInternalServerError, "could not find parent contest for problem, state may be inconsistent")
		return
	}

	if err := h.db.Delete(&models.Problem{}, "id = ?", problemID).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to delete problem: %w", err))
		return
	}

	// Remove the problem ID from the parent contest's ordered list
	newIDs := make([]string, 0, len(parentContest.ProblemIDs))
	for _, pid := range parentContest.ProblemIDs {
		if pid != problemID {
			newIDs = append(newIDs, pid)
		}
	}
	mc := judger.ContestToModel(parentContest)
	mc.ProblemIDs = models.StringArray(newIDs)
	if err := h.db.Model(&models.Contest{}).Where("id = ?", parentContest.ID).Update("problem_ids", mc.ProblemIDs).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update contest problem list: %w", err))
		return
	}
	zap.S().Warnf("admin deleted problem '%s' from contest '%s'", problemID, parentContest.ID)
	h.reload(c)
}
```

- [ ] **Step 2: Update imports in `problem.go`**

Replace the import block with:

```go
import (
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/judger"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)
```

- [ ] **Step 3: Verify the package compiles**

Run: `go build ./internal/api/admin/`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/api/admin/problem.go
git commit -m "refactor(admin): problem CRUD writes to DB"
```

### Task D4: Announcement CRUD → DB

**Files:**
- Modify: `internal/api/admin/announcement.go`

- [ ] **Step 1: Replace the whole file**

Replace `internal/api/admin/announcement.go` with:

```go
package admin

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (h *Handler) handleGetContestAnnouncements(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	util.Success(c, contest.Announcements, "Announcements retrieved successfully")
}

func (h *Handler) handleCreateContestAnnouncement(c *gin.Context) {
	contestID := c.Param("id")
	var req struct {
		Title       string `json:"title" binding:"required"`
		Description string `json:"description" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	now := time.Now()
	ann := models.Announcement{
		ID:          uuid.NewString(),
		ContestID:   contestID,
		Title:       req.Title,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.db.Create(&ann).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to create announcement: %w", err))
		return
	}
	zap.S().Infof("admin created announcement '%s' in contest '%s'", ann.ID, contestID)
	h.reload(c)
}

func (h *Handler) handleUpdateContestAnnouncement(c *gin.Context) {
	contestID := c.Param("id")
	announcementID := c.Param("announcementId")
	var req struct {
		Title       string `json:"title" binding:"required"`
		Description string `json:"description" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	res := h.db.Model(&models.Announcement{}).Where("id = ? AND contest_id = ?", announcementID, contestID).
		Updates(map[string]interface{}{"title": req.Title, "description": req.Description, "updated_at": time.Now()})
	if res.Error != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update announcement: %w", res.Error))
		return
	}
	if res.RowsAffected == 0 {
		util.Error(c, http.StatusNotFound, "announcement not found")
		return
	}
	zap.S().Infof("admin updated announcement '%s' in contest '%s'", announcementID, contestID)
	h.reload(c)
}

func (h *Handler) handleDeleteContestAnnouncement(c *gin.Context) {
	contestID := c.Param("id")
	announcementID := c.Param("announcementId")

	res := h.db.Where("id = ? AND contest_id = ?", announcementID, contestID).Delete(&models.Announcement{})
	if res.Error != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to delete announcement: %w", res.Error))
		return
	}
	if res.RowsAffected == 0 {
		util.Error(c, http.StatusNotFound, "announcement not found")
		return
	}
	zap.S().Warnf("admin deleted announcement '%s' from contest '%s'", announcementID, contestID)
	h.reload(c)
}
```

- [ ] **Step 2: Verify the package compiles**

Run: `go build ./internal/api/admin/`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/api/admin/announcement.go
git commit -m "refactor(admin): announcement CRUD writes to DB"
```

---

## Phase E: Admin assets → DB

### Task E1: Rewrite `assets.go` to use the DB

**Files:**
- Modify: `internal/api/admin/assets.go`

- [ ] **Step 1: Replace the whole file**

Replace `internal/api/admin/assets.go` with:

```go
package admin

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type AssetInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// listAssetsFromDB lists asset rows for an owner.
func listAssetsFromDB(db *gorm.DB, ownerType, ownerID string) ([]AssetInfo, error) {
	var rows []models.Asset
	if err := db.Find(&rows, "owner_type = ? AND owner_id = ?", ownerType, ownerID).Error; err != nil {
		return nil, err
	}
	out := make([]AssetInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, AssetInfo{
			Name:    path.Base(r.Path),
			Path:    r.Path,
			IsDir:   r.IsDir,
			Size:    r.Size,
			ModTime: r.ModTime,
		})
	}
	return out, nil
}
```

// cleanAssetPath normalizes a user-supplied relative path and rejects traversal.
func cleanAssetPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	cleaned := path.Clean(p)
	if cleaned == "." {
		return "", fmt.Errorf("empty asset path")
	}
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "../") {
		return "", fmt.Errorf("path traversal attempt detected")
	}
	return strings.ReplaceAll(cleaned, "\\", "/"), nil
}

// ensureDirRows upserts directory rows for all ancestors of relPath.
func ensureDirRows(db *gorm.DB, ownerType, ownerID, relPath string) error {
	dir := path.Dir(relPath)
	if dir == "." || dir == "/" {
		return nil
	}
	parts := strings.Split(dir, "/")
	current := ""
	now := time.Now()
	for _, p := range parts {
		if p == "" {
			continue
		}
		if current == "" {
			current = p
		} else {
			current = current + "/" + p
		}
		row := models.Asset{
			ID:        uuid.NewString(),
			OwnerType: ownerType,
			OwnerID:   ownerID,
			Path:      current,
			IsDir:     true,
			ModTime:   now,
		}
		if err := db.Where("owner_type = ? AND owner_id = ? AND path = ?",
			ownerType, ownerID, current).Assign(row).FirstOrCreate(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) handleListContestAssets(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	assets, err := listAssetsFromDB(h.db, "contest", contestID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to list assets: %w", err))
		return
	}
	util.Success(c, assets, "Assets listed successfully")
}

func (h *Handler) handleListProblemAssets(c *gin.Context) {
	problemID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	assets, err := listAssetsFromDB(h.db, "problem", problemID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to list assets: %w", err))
		return
	}
	util.Success(c, assets, "Assets listed successfully")
}

// handleUploadAsset reads multipart files and stores them as Asset rows.
func (h *Handler) handleUploadAsset(c *gin.Context, ownerType, ownerID string) {
	form, err := c.MultipartForm()
	if err != nil {
		util.Error(c, http.StatusBadRequest, fmt.Errorf("failed to parse multipart form: %w", err))
		return
	}
	files := form.File["files"]
	subdir := ""
	if v := form.Value["path"]; len(v) > 0 {
		subdir = v[0]
	}

	count := 0
	now := time.Now()
	for _, file := range files {
		rel, err := cleanAssetPath(strings.TrimPrefix(subdir, "/") + "/" + file.Filename)
		if err != nil {
			util.Error(c, http.StatusBadRequest, err)
			return
		}
		src, err := file.Open()
		if err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to open uploaded file: %w", err))
			return
		}
		content, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to read uploaded file: %w", err))
			return
		}
		if err := ensureDirRows(h.db, ownerType, ownerID, rel); err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to create dir rows: %w", err))
			return
		}
		row := models.Asset{
			ID:        uuid.NewString(),
			OwnerType: ownerType,
			OwnerID:   ownerID,
			Path:      rel,
			IsDir:     false,
			Size:      int64(len(content)),
			ModTime:   now,
			Content:   content,
		}
		// Replace any existing row at the same path.
		h.db.Where("owner_type = ? AND owner_id = ? AND path = ?", ownerType, ownerID, rel).Delete(&models.Asset{})
		if err := h.db.Create(&row).Error; err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to store file: %w", err))
			return
		}
		count++
	}
	util.Success(c, gin.H{"files_uploaded": count}, "Files uploaded successfully")
}

func (h *Handler) handleUploadContestAssets(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	h.handleUploadAsset(c, "contest", contestID)
}

func (h *Handler) handleUploadProblemAssets(c *gin.Context) {
	problemID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	h.handleUploadAsset(c, "problem", problemID)
}

func (h *Handler) handleDeleteAsset(c *gin.Context, ownerType, ownerID string) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	cleaned, err := cleanAssetPath(req.Path)
	if err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	// Delete the row at this path AND any rows under it (subtree).
	prefix := cleaned + "/"
	res := h.db.Where(
		"owner_type = ? AND owner_id = ? AND (path = ? OR path LIKE ?)",
		ownerType, ownerID, cleaned, prefix+"%",
	).Delete(&models.Asset{})
	if res.Error != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to delete asset: %w", res.Error))
		return
	}
	if res.RowsAffected == 0 {
		util.Error(c, http.StatusNotFound, "asset not found")
		return
	}
	zap.S().Warnf("admin deleted asset at '%s'", cleaned)
	util.Success(c, nil, "Asset deleted successfully")
}

func (h *Handler) handleDeleteContestAsset(c *gin.Context) {
	contestID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	h.handleDeleteAsset(c, "contest", contestID)
}

func (h *Handler) handleDeleteProblemAsset(c *gin.Context) {
	problemID := c.Param("id")
	h.appState.RLock()
	_, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	h.handleDeleteAsset(c, "problem", problemID)
}

// serveAssetDB looks up a single asset row by exact path and writes its bytes.
func (h *Handler) serveAssetDB(c *gin.Context, ownerType, ownerID, assetPath string) {
	cleaned, err := cleanAssetPath(assetPath)
	if err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	var row models.Asset
	if err := h.db.Where("owner_type = ? AND owner_id = ? AND path = ?", ownerType, ownerID, cleaned).First(&row).Error; err != nil {
		util.Error(c, http.StatusNotFound, "asset not found")
		return
	}
	if row.IsDir {
		util.Error(c, http.StatusBadRequest, "cannot serve a directory")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(cleaned)))
	c.Data(http.StatusOK, contentTypeFor(cleaned), row.Content)
}

func (h *Handler) serveContestAsset(c *gin.Context) {
	contestID := c.Param("id")
	assetPath := c.Param("assetpath")
	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}
	h.serveAssetDB(c, "contest", contestID, assetPath)
}

func (h *Handler) serveProblemAsset(c *gin.Context) {
	problemID := c.Param("id")
	assetPath := c.Param("assetpath")
	h.appState.RLock()
	_, ok := h.appState.Problems[problemID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	h.serveAssetDB(c, "problem", problemID, assetPath)
}

func contentTypeFor(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".md":
		return "text/markdown; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".json":
		return "application/json"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}
```

Add `"io"` to the imports (used by `io.ReadAll`).

- [ ] **Step 2: Verify the package compiles**

Run: `go build ./internal/api/admin/`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/api/admin/assets.go
git commit -m "refactor(admin): assets stored as BLOB rows in the DB"
```

---

## Phase F: Cluster node + Links admin endpoints

### Task F1: `PUT /admin/clusters/:c/nodes/:n` handler

**Files:**
- Modify: `internal/api/admin/cluster.go`, `internal/api/admin/router.go`

- [ ] **Step 1: Add the `updateNode` handler**

Append to `internal/api/admin/cluster.go`:

```go
func (h *Handler) updateNode(c *gin.Context) {
	clusterName := c.Param("clusterName")
	nodeName := c.Param("nodeName")
	var req struct {
		CPU    int   `json:"cpu" binding:"required"`
		Memory int64 `json:"memory" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if req.CPU <= 0 || req.Memory <= 0 {
		util.Error(c, http.StatusBadRequest, "cpu and memory must be positive")
		return
	}

	// Verify the node exists in the scheduler (i.e. its docker connection is in config.yaml).
	if _, err := h.scheduler.GetNodeDetails(clusterName, nodeName); err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}

	node := models.ClusterNode{
		ClusterName: clusterName,
		NodeName:    nodeName,
		CPU:         req.CPU,
		Memory:      req.Memory,
	}
	if err := database.UpsertClusterNode(h.db, &node); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("failed to update node: %w", err))
		return
	}
	if err := h.scheduler.UpdateNodeResources(clusterName, nodeName, req.CPU, req.Memory); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	zap.S().Infof("admin updated node %s/%s resources: cpu=%d memory=%d", clusterName, nodeName, req.CPU, req.Memory)
	util.Success(c, node, "Node resources updated")
}
```

Add imports to `cluster.go`: `"fmt"`, `"github.com/ZJUSCT/CSOJ/internal/database"`, `"github.com/ZJUSCT/CSOJ/internal/database/models"`, `"net/http"`, `"github.com/ZJUSCT/CSOJ/internal/util"`, `"go.uber.org/zap"`, `"github.com/gin-gonic/gin"` (some may already be present — keep the union).

- [ ] **Step 2: Register the route**

In `internal/api/admin/router.go`, inside the `clusters := adminV1.Group("/clusters")` block, add after the `resume` route:

```go
			clusters.PUT("/:clusterName/nodes/:nodeName", h.updateNode)
```

- [ ] **Step 3: Verify the package compiles**

Run: `go build ./internal/api/admin/`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/api/admin/cluster.go internal/api/admin/router.go
git commit -m "feat(admin): PUT /admin/clusters/:c/nodes/:n for cpu/memory"
```

### Task F2: Links CRUD

**Files:**
- Create: `internal/api/admin/links.go`, modify `internal/api/admin/router.go`

- [ ] **Step 1: Create `internal/api/admin/links.go`**

```go
package admin

import (
	"net/http"
	"strconv"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

func (h *Handler) listLinks(c *gin.Context) {
	links, err := database.GetAllLinks(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, links, "Links retrieved")
}

func (h *Handler) createLink(c *gin.Context) {
	var link models.Link
	if err := c.ShouldBindJSON(&link); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if link.Position == 0 {
		// Default to the end of the list.
		existing, _ := database.GetAllLinks(h.db)
		link.Position = len(existing)
	}
	if err := database.CreateLink(h.db, &link); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, link, "Link created")
}

func (h *Handler) updateLink(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.Error(c, http.StatusBadRequest, "invalid link id")
		return
	}
	var link models.Link
	if err := c.ShouldBindJSON(&link); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	link.ID = uint(id)
	if err := database.UpdateLink(h.db, &link); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, link, "Link updated")
}

func (h *Handler) deleteLink(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.Error(c, http.StatusBadRequest, "invalid link id")
		return
	}
	if err := database.DeleteLink(h.db, uint(id)); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "Link deleted")
}
```

- [ ] **Step 2: Register the routes**

In `internal/api/admin/router.go`, inside the `adminV1` group block (after the `clusters` group, for example), add:

```go
		links := adminV1.Group("/links")
		{
			links.GET("", h.listLinks)
			links.POST("", h.createLink)
			links.PUT("/:id", h.updateLink)
			links.DELETE("/:id", h.deleteLink)
		}
```

- [ ] **Step 3: Verify the package compiles**

Run: `go build ./internal/api/admin/`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/api/admin/links.go internal/api/admin/router.go
git commit -m "feat(admin): links CRUD endpoints"
```

---

## Phase G: User-facing handlers read from DB

### Task G1: `getLinks` reads from the DB

**Files:**
- Modify: `internal/api/user/contest.go`

- [ ] **Step 1: Replace `getLinks`**

In `internal/api/user/contest.go`, replace `getLinks`:

```go
func (h *Handler) getLinks(c *gin.Context) {
	links, err := database.GetAllLinks(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	if links == nil {
		links = []models.Link{}
	}
	util.Success(c, links, "Links retrieved successfully")
}
```

Add imports `"github.com/ZJUSCT/CSOJ/internal/database"` and `"github.com/ZJUSCT/CSOJ/internal/database/models"` to `contest.go` if not present. Remove any now-unused imports.

- [ ] **Step 2: Verify the package compiles**

Run: `go build ./internal/api/user/`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/api/user/contest.go
git commit -m "refactor(user): getLinks reads from the DB"
```

### Task G2: User asset serve reads from the DB

**Files:**
- Modify: `internal/api/user/asset.go`

- [ ] **Step 1: Replace `serveContestAsset` and `serveProblemAsset`**

In `internal/api/user/asset.go`, replace both functions:

```go
func (h *Handler) serveContestAsset(c *gin.Context) {
	contestID := c.Param("id")
	assetPath := c.Param("assetpath")

	h.appState.RLock()
	_, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()
	if !ok {
		util.Error(c, http.StatusNotFound, "contest not found")
		return
	}

	var row models.Asset
	if err := h.db.Where("owner_type = ? AND owner_id = ? AND path = ?", "contest", contestID, cleanUserAssetPath(assetPath)).First(&row).Error; err != nil {
		util.Error(c, http.StatusNotFound, "asset not found")
		return
	}
	if row.IsDir {
		util.Error(c, http.StatusBadRequest, "cannot serve a directory")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(row.Path)))
	c.Data(http.StatusOK, contentTypeFor(row.Path), row.Content)
}

func (h *Handler) serveProblemAsset(c *gin.Context) {
	problemID := c.Param("id")
	assetPath := c.Param("assetpath")

	h.appState.RLock()
	problem, ok := h.appState.Problems[problemID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusNotFound, "problem not found")
		return
	}
	parentContest, ok := h.appState.ProblemToContestMap[problemID]
	if !ok {
		h.appState.RUnlock()
		util.Error(c, http.StatusInternalServerError, "internal server error: problem has no parent contest")
		return
	}
	now := time.Now()
	if now.Before(parentContest.StartTime) {
		h.appState.RUnlock()
		util.Error(c, http.StatusForbidden, "contest has not started yet")
		return
	}
	if now.Before(problem.StartTime) {
		h.appState.RUnlock()
		util.Error(c, http.StatusForbidden, "problem has not started yet")
		return
	}
	h.appState.RUnlock()

	var row models.Asset
	if err := h.db.Where("owner_type = ? AND owner_id = ? AND path = ?", "problem", problemID, cleanUserAssetPath(assetPath)).First(&row).Error; err != nil {
		util.Error(c, http.StatusNotFound, "asset not found")
		return
	}
	if row.IsDir {
		util.Error(c, http.StatusBadRequest, "cannot serve a directory")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(row.Path)))
	c.Data(http.StatusOK, contentTypeFor(row.Path), row.Content)
}
```

- [ ] **Step 2: Add shared helpers + imports**

Add to `internal/api/user/asset.go` (the `cleanUserAssetPath` and `contentTypeFor` helpers — small local copies so the `user` package doesn't import `admin`):

```go
func cleanUserAssetPath(p string) string {
	p = strings.TrimPrefix(p, "/")
	cleaned := path.Clean(p)
	if strings.HasPrefix(cleaned, "..") {
		return ""
	}
	return strings.ReplaceAll(cleaned, "\\", "/")
}

func contentTypeFor(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".md":
		return "text/markdown; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".json":
		return "application/json"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}
```

Replace the import block of `internal/api/user/asset.go` with:

```go
import (
	"crypto/hmac"
	"crypto/sha512"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)
```

(`os` and `path/filepath` are no longer used — `serveAvatar` still uses `filepath`, so KEEP `path/filepath` and `os` if `serveAvatar` remains in this file. Read `serveAvatar`: it uses `filepath.Base`, `filepath.Join`, `os.Stat`, `c.File` — so keep `os` and `path/filepath`.)

Final imports:

```go
import (
	"crypto/hmac"
	"crypto/sha512"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)
```

- [ ] **Step 3: Verify the package compiles**

Run: `go build ./internal/api/user/`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/api/user/asset.go
git commit -m "refactor(user): serveContestAsset/serveProblemAsset read from DB"
```

---

## Phase H: Config trim + boot rewrite

### Task H1: Remove `ContestsRoot`/`Links` from `Config`

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Remove the fields**

In `internal/config/config.go`, in the `Config` struct, delete the lines:

```go
	ContestsRoot string    `yaml:"contests_root"`
	Links        []Link    `yaml:"links"`
```

Keep the `Link` struct definition (it's still used by the `models.Link`? No — `config.Link` and `models.Link` are separate. `config.Link` is no longer referenced anywhere after `getLinks` moves to DB. Delete the `Link` struct too, and the unused import if any.)

Delete the `type Link struct { ... }` block from `config.go`.

- [ ] **Step 2: Verify the package compiles**

Run: `go build ./internal/config/`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "refactor(config): remove contests_root and links (now DB-backed)"
```

### Task H2: `main.go` boot uses `LoadFromDB`

**Files:**
- Modify: `cmd/CSOJ/main.go`

- [ ] **Step 1: Replace the boot-time contest loading**

In `cmd/CSOJ/main.go`, find the block that calls `judger.FindContestDirs` / `LoadAllContestsAndProblems` and builds `problemToContestMap`. Replace it with:

```go
	// contests and problems (loaded from the DB)
	contests, problems, problemToContestMap, err := judger.LoadFromDB(db)
	if err != nil {
		zap.S().Fatalf("failed to load contests and problems: %v", err)
	}
	appState.Contests = contests
	appState.Problems = problems
	appState.ProblemToContestMap = problemToContestMap
	zap.S().Infof("loaded %d contests and %d problems", len(contests), len(problems))
```

- [ ] **Step 2: Verify the whole project builds**

Run: `go build ./...`
Expected: exit 0.

Run: `go vet ./...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add cmd/CSOJ/main.go
git commit -m "refactor(main): boot loads contests/problems from DB"
```

---

## Phase I: Docs

### Task I1: Update config + getting-started docs

**Files:**
- Modify: `docs/configuration/main-config.md`, `docs/configuration/contest-config.md`, `docs/configuration/problem-config.md`, `docs/getting-started.md`

- [ ] **Step 1: Update `main-config.md`**

In `docs/configuration/main-config.md`:
- Remove `contests_root` from the field reference and the sample config.
- Remove `links` from the field reference and the sample config.
- Remove `cpu`/`memory` from each `cluster.node` entry in the sample config (keep `name` + `docker.*`); add a note: "Node `cpu`/`memory` are managed at runtime via the admin API and stored in the database."
- Add a section explaining that contests, problems, announcements, assets, cluster node caps, and links are all stored in the database and managed via the admin API — not on disk.

- [ ] **Step 2: Update `contest-config.md`**

Rewrite `docs/configuration/contest-config.md` to describe the contest as a DB record (created via `POST /api/v1/admin/contests`), not a `contest.yaml` file. Keep the field reference (id, name, starttime, endtime, problems, description) as the JSON shape. Remove all references to the on-disk directory layout.

- [ ] **Step 3: Update `problem-config.md`**

Rewrite `docs/configuration/problem-config.md` similarly: a problem is a DB record created via `POST /api/v1/admin/contests/:id/problems`. Keep the field reference. Remove the on-disk layout.

- [ ] **Step 4: Update `getting-started.md`**

In `docs/getting-started.md`:
- Remove the `contests_root` and `links` keys from the sample `config.yaml`.
- Remove `cpu`/`memory` from the cluster node in the sample (keep `name` + `docker.host`).
- Add a note: "Contests, problems, and other runtime data are managed via the admin API and stored in the database. Use the admin UI (or the admin REST API) to create them after first boot."
- Remove any reference to a `contests/` directory on disk.

- [ ] **Step 5: Commit**

```bash
git add docs/
git commit -m "docs: reflect DB-backed config and contest/problem storage"
```

### Task I2: Update admin API reference

**Files:**
- Modify: `docs/api-reference/admin-api.md`

- [ ] **Step 1: Add the new endpoints**

In `docs/api-reference/admin-api.md`, add endpoint entries for:
- `GET /api/v1/admin/links`, `POST /api/v1/admin/links`, `PUT /api/v1/admin/links/:id`, `DELETE /api/v1/admin/links/:id` — manage nav links.
- `PUT /api/v1/admin/clusters/:clusterName/nodes/:nodeName` — update a node's `cpu`/`memory` (body `{"cpu":N,"memory":N}`).

- [ ] **Step 2: Note the storage change**

In the intro of `docs/api-reference/admin-api.md`, add a line: "All admin-managed data (contests, problems, announcements, assets, links, cluster node caps) is persisted in the database."

- [ ] **Step 3: Commit**

```bash
git add docs/api-reference/admin-api.md
git commit -m "docs(admin-api): document links and node-caps endpoints; note DB storage"
```

---

## Phase J: End-to-end verification

### Task J1: Build everything

**Files:** (no file changes — verification only)

- [ ] **Step 1: Build the frontend + backend**

Run: `make build`
Expected: `CSOJ` binary produced, no errors.

### Task J2: Smoke-test the DB-backed system

**Files:** (no file changes — verification only)

- [ ] **Step 1: Start with a fresh DB**

Create `/tmp/csoj-db-smoke/configs/config.yaml`:
```yaml
listen: ":18080"
logger: { level: "debug" }
storage:
  database: "data/csoj.db"
  user_avatar: "data/avatars"
  submission_content: "data/submissions"
  submission_log: "data/logs"
auth:
  jwt: { secret: "smoke-test-secret", expire_hours: 72 }
  local: { enabled: true }
  gitlab:
    app: ""
    url: "https://gitlab.com"
    client_id: ""
    client_secret: ""
    redirect_uri: ""
    frontend_callback_url: ""
cors: { allowed_origins: [] }
cluster:
  - name: "default-cluster"
    node:
      - name: "local-node"
        docker: { host: "tcp://127.0.0.1:2375" }
```

(Note: NO `contests_root`, NO `links`, NO `cpu`/`memory` on the node.)

Start the server:
```bash
cd /tmp/csoj-db-smoke && /home/yihao/develop/CSOJ/CSOJ -c configs/config.yaml
```
Expected: server logs `loaded 0 contests and 0 problems` (no fatal). A `cluster_nodes` row does NOT yet exist for `local-node`, so the scheduler logs `node default-cluster/local-node has no DB resource caps; skipping`.

- [ ] **Step 2: Register first user → superadmin; set node caps**

```bash
curl -s localhost:18080/api/v1/auth/local/register -H 'Content-Type: application/json' \
  -d '{"username":"root","password":"rootpass","nickname":"Root"}'
TOKEN=$(curl -s localhost:18080/api/v1/auth/local/login -H 'Content-Type: application/json' \
  -d '{"username":"root","password":"rootpass"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])")
# Set the node's cpu/memory (creates the ClusterNode row → node now active in scheduler)
curl -s -X PUT localhost:18080/api/v1/admin/clusters/default-cluster/nodes/local-node \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"cpu":4,"memory":4096}'
# Confirm cluster status shows the node with cpu=4
curl -s localhost:18080/api/v1/admin/clusters/status -H "Authorization: Bearer $TOKEN" | python3 -m json.tool | grep -A3 local-node
```
Expected: node `local-node` appears with `cpu: 4`, `memory: 4096`.

- [ ] **Step 3: Create a contest + problem via the admin API**

```bash
curl -s -X POST localhost:18080/api/v1/admin/contests -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"id":"c1","name":"Contest 1","starttime":"2026-01-01T00:00:00Z","endtime":"2027-01-01T00:00:00Z","description":"# C1"}'
curl -s localhost:18080/api/v1/contests | python3 -m json.tool | head
curl -s -X POST localhost:18080/api/v1/admin/contests/c1/problems -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"id":"p1","name":"Problem 1","level":"easy","starttime":"2026-01-01T00:00:00Z","endtime":"2027-01-01T00:00:00Z","max_submissions":3,"cluster":"default-cluster","cpu":1,"memory":256,"upload":{"max_num":1,"max_size":1024,"upload_form":true,"upload_files":[],"editor":false,"editor_files":[]},"workflow":[],"score":{"mode":"score","max_performance_score":100},"description":"# P1"}'
curl -s localhost:18080/api/v1/contests/c1 | python3 -m json.tool | grep -E "id|problem_ids"
```
Expected: contest `c1` appears with `problem_ids: ["p1"]`; problem `p1` appears via `GET /api/v1/problems/p1`.

- [ ] **Step 4: Upload + fetch an asset**

```bash
echo "hello" > /tmp/hello.txt
curl -s -X POST localhost:18080/api/v1/admin/contests/c1/assets -H "Authorization: Bearer $TOKEN" \
  -F "files=@/tmp/hello.txt"
# List
curl -s localhost:18080/api/v1/admin/contests/c1/assets -H "Authorization: Bearer $TOKEN" | python3 -m json.tool
# Fetch via signed URL
URL=$(curl -s "localhost:18080/api/v1/assets/query_url?asset=/api/v1/assets/contests/c1/hello.txt" -H "Authorization: Bearer $TOKEN" | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['url'])")
curl -s "localhost:18080${URL}" ; echo
```
Expected: list shows `hello.txt`; the signed-URL fetch returns `hello`.

- [ ] **Step 5: Link CRUD**

```bash
curl -s -X POST localhost:18080/api/v1/admin/links -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Source","url":"https://github.com/ZJUSCT/CSOJ"}'
curl -s localhost:18080/api/v1/links | python3 -m json.tool
```
Expected: `GET /api/v1/links` returns the link.

- [ ] **Step 6: Restart → data persists**

Stop the server, restart it, `curl localhost:18080/api/v1/contests`. Expected: contest `c1` still present (DB is the source of truth).

- [ ] **Step 7: No-commit verification gate**

If any step fails, debug with the superpowers:systematic-debugging skill before declaring done. On success, stop the server and clean up `/tmp/csoj-db-smoke`.

### Task J3: Run the `verify` skill

- [ ] **Step 1: Invoke verify**

Use the `verify` skill to drive the affected flows end-to-end: boot with empty DB, first-user→superadmin, set node caps, create contest+problem, upload+fetch asset, link CRUD, restart-persists. Capture the result.

- [ ] **Step 2: Commit any fixes**

If verification found bugs, fix and commit. If clean, no commit.

---

## Notes for the implementer

- **Order:** Phase A → B → C → D → E → F → G → H → I → J. Within Phase D, do D1 (reload) first so the build stays green-ish, then D2/D3/D4.
- **`config.Node` no longer has `CPU`/`Memory`** — any code reading `node.CPU`/`node.Memory` via the embedded `*config.Node` must move to `NodeState.CPU`/`NodeState.Memory`. Grep `internal/judger/` for `.CPU`/`.Memory` after C1.
- **`judger.Contest.ProblemDirs` is gone** — only `ProblemIDs` remains. The order endpoint simplifies to sorting `ProblemIDs`.
- **Assets: BLOB column.** `Asset.Content []byte` with `gorm:"type:blob"`. SQLite stores it inline; large files will bloat the DB — acceptable per the spec (fresh start, no migration).
- **No frontend changes** in this plan — the admin UI already calls these endpoints with the same shapes.
- **The `data/` dir** (SQLite file) is created at runtime under the configured `storage.database` path; it's gitignored via `storage`.
