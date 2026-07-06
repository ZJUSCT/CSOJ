# DB-Managed Runtime Settings + Clusters — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Shrink `config.yaml` to boot facts only (`listen`, `storage.*`, `auth.jwt.secret`); move logger, CORS, local-auth, GitLab OIDC, JWT expiry, and all cluster config (kubeconfig text) into DB tables managed via the admin panel, with a `SettingsStore` read-through cache for live reload and a "Reload Clusters" admin button.

**Architecture:** New `settings` (key/value, JSON) and `clusters` (kubeconfig-as-text rows) DB tables. A `SettingsStore` is injected into handlers; CORS/local/gitlab read it per-request; logger + clusters are boot-or-reload-applied. `Scheduler.ReloadClusters()` rebuilds K8s clientsets from the DB without a restart.

**Tech Stack:** Go 1.24, Gin, GORM/SQLite, `k8s.io/client-go`, `coreos/go-oidc`.

**Reference spec:** `docs/superpowers/specs/2026-07-06-db-managed-settings-design.md`

**Branch:** `merge-webui-admin`.

---

## File Structure

- **Modify** `internal/config/config.go` — `Config` shrinks (drop `Logger`/`CORS`/`Cluster`/`NodePool`/`Duration`/`Auth.Local`/`Auth.GitLab`); add `NewSettingsStore` + `SettingsStore` type
- **Create** `internal/config/settings.go` — `SettingsStore` (read-through cache: `NewSettingsStore`, `Get`, `Set`, `ReloadAll`)
- **Create** `internal/config/settings_test.go` — table-driven round-trip tests
- **Modify** `internal/database/models/models.go` — add `Setting` + `Cluster` (DB cluster) models; remove `ClusterNodePool`? NO — keep `ClusterNodePool` (pools stay)
- **Modify** `internal/database/database.go` — AutoMigrate `Setting` + `Cluster`
- **Modify** `internal/database/crud.go` — add `GetAllClusters`/`UpsertCluster`/`DeleteCluster` + `GetSetting`/`SetSetting` (low-level, used by SettingsStore)
- **Modify** `internal/api/middleware.go` — `CORSMiddleware(*SettingsStore)` per-request
- **Modify** `internal/auth/gitlab.go` — `GitLabHandler` reads gitlab settings per-request; no boot `Fatalf`
- **Modify** `internal/api/user/handler.go` — `Handler` gets `settings *config.SettingsStore`; `NewHandler(cfg, settings, db, scheduler, appState)`
- **Modify** `internal/api/user/router.go` — `RegisterRoutes(..., settings, ...)`; `CORSMiddleware` call site
- **Modify** `internal/api/user/auth.go` — `getAuthStatus` reads `auth.local` from settings; `localLogin` checks the toggle
- **Modify** `internal/api/admin/handler.go` — `Handler` gets `settings`; `NewHandler(cfg, settings, db, scheduler, appState)`
- **Modify** `internal/api/admin/router.go` — `RegisterRoutes(..., settings, ...)`
- **Create** `internal/api/admin/settings.go` — `GET /admin/settings`, `PUT /admin/settings/:key`
- **Modify** `internal/api/admin/cluster.go` — add `GET/POST/PUT/DELETE /admin/clusters` + `POST /admin/clusters/reload` (cluster rows, distinct from pools)
- **Modify** `internal/judger/scheduler.go` — `NewScheduler(db, settings, appState)` (drop `cfg`); `ReloadClusters()`; build clientsets from `database.GetAllClusters` (kubeconfig text)
- **Modify** `internal/judger/recovery.go` — `RecoverAndCleanup(db, instanceID)` (drop `cfg`); reads clusters from DB
- **Modify** `internal/judger/heartbeat.go` — reads `heartbeat_ttl` from the DB `clusters` row
- **Modify** `cmd/CSOJ/main.go` — boot: build `SettingsStore`, logger from settings, `RecoverAndCleanup(db, instanceID)`, `NewScheduler(db, settings, appState)`, heartbeats per DB cluster, `CORSMiddleware(settingsStore)`
- **Modify** docs: `main-config.md`, `getting-started.md`, `admin-api.md`

### Out of scope
- Logger hot-reload (restart-required).
- Admin-frontend settings page UI (API only; documented follow-up).
- `podspec` unit tests (unchanged).

---

## Phase A: Models + SettingsStore + CRUD

### Task A1: `Setting` + `Cluster` models

**Files:**
- Modify: `internal/database/models/models.go`

- [ ] **Step 1: Add the `Setting` and `Cluster` models**

Append to `internal/database/models/models.go`:

```go
// Setting is a generic key/value runtime setting (JSON-encoded value).
type Setting struct {
	Key       string    `gorm:"primaryKey" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Cluster is a backend-managed K8s cluster (kubeconfig stored as text).
// Node-pool caps live in ClusterNodePool (cluster_name is the parent key).
type Cluster struct {
	Name         string    `gorm:"primaryKey" json:"name"`
	Kubeconfig   string    `gorm:"type:text" json:"kubeconfig"`
	Context      string    `json:"context"`
	Namespace    string    `json:"namespace"`
	Concurrency  int       `json:"concurrency"`
	HeartbeatTTL int       `json:"heartbeat_ttl"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/database/models/`
Expected: exit 0.

- [ ] **Step 3: Commit**

```bash
git add internal/database/models/models.go
git commit -m "feat(db): add Setting and Cluster models"
```

### Task A2: AutoMigrate + low-level CRUD

**Files:**
- Modify: `internal/database/database.go`, `internal/database/crud.go`

- [ ] **Step 1: AutoMigrate the new models**

In `internal/database/database.go`, add `&models.Setting{}` and `&models.Cluster{}` to the `db.AutoMigrate(...)` call (after `&models.Heartbeat{}`).

- [ ] **Step 2: Add low-level CRUD helpers**

Append to `internal/database/crud.go`:

```go
// --- Settings (low-level; SettingsStore wraps these with caching) ---

func GetSetting(db *gorm.DB, key string) (*models.Setting, error) {
	var s models.Setting
	err := db.Where("key = ?", key).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &s, err
}

func SetSetting(db *gorm.DB, key, value string) error {
	s := models.Setting{Key: key, Value: value, UpdatedAt: time.Now()}
	return db.Save(&s).Error
}

func GetAllSettings(db *gorm.DB) ([]models.Setting, error) {
	var rows []models.Setting
	err := db.Find(&rows).Error
	return rows, err
}

// --- Clusters (DB rows; replaces config.Cluster) ---

func GetAllClusters(db *gorm.DB) ([]models.Cluster, error) {
	var rows []models.Cluster
	err := db.Find(&rows).Error
	return rows, err
}

func UpsertCluster(db *gorm.DB, c *models.Cluster) error {
	return db.Save(c).Error
}

func DeleteCluster(db *gorm.DB, name string) error {
	return db.Where("name = ?", name).Delete(&models.Cluster{}).Error
}
```

Ensure `time` is imported in `crud.go` (it may already be; add if not).

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/database/...`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/database/database.go internal/database/crud.go
git commit -m "feat(db): AutoMigrate + CRUD for Setting and Cluster"
```

### Task A3: `SettingsStore` + tests

**Files:**
- Create: `internal/config/settings.go`, `internal/config/settings_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/config/settings_test.go`:

```go
package config

import (
	"encoding/json"
	"testing"

	"github.com/ZJUSCT/CSOJ/internal/database"
)

func newTestStore(t *testing.T) *SettingsStore {
	t.Helper()
	db, err := database.Init(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return NewSettingsStore(db)
}

func TestSettingsStore_SetGet_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	type corsVal struct{ AllowedOrigins []string }
	want := corsVal{AllowedOrigins: []string{"https://oj.example.com", "*"}}
	if err := s.Set("cors", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	var got corsVal
	if err := s.Get("cors", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.AllowedOrigins) != 2 || got.AllowedOrigins[0] != "https://oj.example.com" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestSettingsStore_Get_Missing(t *testing.T) {
	s := newTestStore(t)
	var got struct{ X int }
	if err := s.Get("nope", &got); err != nil {
		t.Fatalf("Get missing should not error: %v", err)
	}
	if got.X != 0 {
		t.Errorf("missing key should yield zero value; got %+v", got)
	}
}

func TestSettingsStore_ReloadAll(t *testing.T) {
	s := newTestStore(t)
	_ = s.Set("k", "v1")
	// Mutate the DB behind the cache.
	if err := database.SetSetting(s.db, "k", `"v2"`); err != nil {
		t.Fatalf("direct SetSetting: %v", err)
	}
	// Cache still has v1.
	var cached string
	_ = s.Get("k", &cached)
	if cached != "v1" {
		t.Errorf("cache should have v1 before reload; got %s", cached)
	}
	s.ReloadAll()
	var after string
	_ = s.Get("k", &after)
	if after != "v2" {
		t.Errorf("after reload should have v2; got %s", after)
	}
}

// keep encoding/json referenced if not used directly above
var _ = json.Marshal
```

Note: `database.Init(":memory:")` opens an in-memory SQLite — verify `database.Init` accepts that path (it does: `os.Stat` on a non-existent `:memory:` is fine because GORM/sqlite handles `:memory:` specially). If `database.Init` creates a directory for the path, `:memory:` would fail; in that case use a temp file path and clean up. Check `database.Init` first — it does `os.MkdirAll(filepath.Dir(dsn))`. For `:memory:`, `filepath.Dir(":memory:")` = `.`, and `os.MkdirAll(".", 0755)` is a no-op. So `:memory:` works.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/config/ -run TestSettingsStore -v`
Expected: FAIL — `SettingsStore`/`NewSettingsStore` undefined.

- [ ] **Step 3: Implement `SettingsStore`**

Create `internal/config/settings.go`:

```go
package config

import (
	"encoding/json"
	"sync"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"gorm.io/gorm"
)

// SettingsStore is a read-through cache over the `settings` DB table.
type SettingsStore struct {
	db    *gorm.DB
	mu    sync.RWMutex
	cache map[string]string // key -> JSON value (raw)
}

// NewSettingsStore loads all settings rows into the cache.
func NewSettingsStore(db *gorm.DB) *SettingsStore {
	s := &SettingsStore{db: db, cache: make(map[string]string)}
	s.ReloadAll()
	return s
}

// Get unmarshals the cached JSON value for `key` into `dst`.
// A missing key leaves `dst` at its zero value and returns nil.
func (s *SettingsStore) Get(key string, dst interface{}) error {
	s.mu.RLock()
	raw, ok := s.cache[key]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	return json.Unmarshal([]byte(raw), dst)
}

// Set marshals `value` to JSON, writes it to the DB, and updates the cache.
func (s *SettingsStore) Set(key string, value interface{}) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	raw := string(b)
	if err := database.SetSetting(s.db, key, raw); err != nil {
		return err
	}
	s.mu.Lock()
	s.cache[key] = raw
	s.mu.Unlock()
	return nil
}

// ReloadAll re-reads every settings row from the DB into the cache.
func (s *SettingsStore) ReloadAll() {
	rows, err := database.GetAllSettings(s.db)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[string]string, len(rows))
	for _, r := range rows {
		s.cache[r.Key] = r.Value
	}
}

// HasKey reports whether a settings row exists for `key` (used for
// bootstrapping defaults: e.g. a missing `auth.local` row = enabled).
func (s *SettingsStore) HasKey(key string) bool {
	s.mu.RLock()
	_, ok := s.cache[key]
	s.mu.RUnlock()
	return ok
}

// DB exposes the underlying *gorm.DB (used by callers that need direct DB access).
func (s *SettingsStore) DB() *gorm.DB { return s.db }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/ -run TestSettingsStore -v`
Expected: 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/settings.go internal/config/settings_test.go
git commit -m "feat(config): SettingsStore read-through cache + tests"
```

---

## Phase B: Shrink config.go + gitlab per-request

### Task B1: Shrink `Config` (drop Logger/CORS/Cluster/Local/GitLab)

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Replace the `Config` + removed types**

Replace the `Config`, `CORS`, `Cluster`, `NodePool`, `Duration`, `Logger`, and `Auth`/`Local`/`GitLab` types. The file should become:

```go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen  string  `yaml:"listen"`
	Storage Storage `yaml:"storage"`
	Auth    Auth    `yaml:"auth"`
}

type Storage struct {
	UserAvatar        string `yaml:"user_avatar"`
	SubmissionContent string `yaml:"submission_content"`
	Database          string `yaml:"database"`
	SubmissionLog     string `yaml:"submission_log"`
}

type Auth struct {
	JWT JWT `yaml:"jwt"`
}

type JWT struct {
	Secret      string `yaml:"secret"`
	ExpireHours int    `yaml:"expire_hours"` // overridden by settings at runtime
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
```

Remove the `"time"` import (no longer needed; `Duration` is gone). Keep `SettingsStore` in `settings.go` (separate file).

- [ ] **Step 2: Verify the config package compiles**

Run: `go build ./internal/config/`
Expected: exit 0. (`go build ./...` will fail broadly — many consumers reference `cfg.Logger`/`cfg.CORS`/`cfg.Cluster`/`cfg.Auth.Local`/`cfg.Auth.GitLab`; fixed in Phases C–F. That's expected.)

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "refactor(config): shrink Config to boot facts (listen/storage/jwt)"
```

### Task B2: GitLab handler reads settings per-request (no boot Fatalf)

**Files:**
- Modify: `internal/auth/gitlab.go`

- [ ] **Step 1: Rewrite `GitLabHandler` to be lazy**

The current `NewGitLabHandler` calls `oidc.NewProvider` at boot and `Fatalf`s if the URL is bad. Since gitlab is now admin-managed (possibly empty at first boot), the handler must build the provider lazily per-request and return an HTTP error (not crash) on failure.

Replace `internal/auth/gitlab.go` from the top through `NewGitLabHandler` and the `Login`/`Callback` method bodies. Keep the `OIDCClaims` struct and everything below `Callback` unchanged (the user-creation + JWT-issuance logic). New top:

```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type GitLabHandler struct {
	cfg      *config.Config
	settings *config.SettingsStore
	db       *gorm.DB
}

type OIDCClaims struct {
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
	Picture           string `json:"picture"`
}

func NewGitLabHandler(cfg *config.Config, settings *config.SettingsStore, db *gorm.DB) *GitLabHandler {
	return &GitLabHandler{cfg: cfg, settings: settings, db: db}
}

// gitlabSettings reads the auth.gitlab settings row.
type gitlabSettings struct {
	App                 string `json:"app"`
	URL                 string `json:"url"`
	ClientID            string `json:"client_id"`
	ClientSecret        string `json:"client_secret"`
	RedirectURI         string `json:"redirect_uri"`
	FrontendCallbackURL string `json:"frontend_callback_url"`
}

func (h *GitLabHandler) loadSettings(c *gin.Context) (*gitlabSettings, error) {
	var gl gitlabSettings
	if err := h.settings.Get("auth.gitlab", &gl); err != nil {
		return nil, fmt.Errorf("read gitlab settings: %w", err)
	}
	if gl.URL == "" || gl.ClientID == "" || gl.ClientSecret == "" || gl.RedirectURI == "" {
		return nil, errors.New("gitlab not configured")
	}
	return &gl, nil
}

// buildProvider constructs the OIDC provider + oauth2 config for this request.
func (h *GitLabHandler) buildProvider(c *gin.Context) (*oidc.Provider, *oauth2.Config, *oidc.IDTokenVerifier, *gitlabSettings, error) {
	gl, err := h.loadSettings(c)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	provider, err := oidc.NewProvider(c.Request.Context(), gl.URL)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("create OIDC provider: %w", err)
	}
	oauth2Config := &oauth2.Config{
		ClientID:     gl.ClientID,
		ClientSecret: gl.ClientSecret,
		RedirectURL:  gl.RedirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID},
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: gl.ClientID})
	return provider, oauth2Config, verifier, gl, nil
}

func (h *GitLabHandler) Login(c *gin.Context) {
	_, oauth2Config, _, _, err := h.buildProvider(c)
	if err != nil {
		util.Error(c, http.StatusServiceUnavailable, err)
		return
	}
	url := oauth2Config.AuthCodeURL("state")
	c.Redirect(http.StatusTemporaryRedirect, url)
}
```

Then update the start of `Callback` (the code-fetching + token-exchange) to call `buildProvider` instead of using the struct fields:

```go
func (h *GitLabHandler) Callback(c *gin.Context) {
	ctx := c.Request.Context()
	code := c.Query("code")

	_, oauth2Config, verifier, gl, err := h.buildProvider(c)
	if err != nil {
		frontendURL := h.cfg.Auth.GitLab.FrontendCallbackURL // legacy; replaced below
		_ = frontendURL
		util.Error(c, http.StatusServiceUnavailable, err)
		return
	}
	frontendURL := gl.FrontendCallbackURL
	// ... (rest of Callback unchanged: token exchange, id token verify, user upsert, GenerateJWT, redirect)
```

Read the rest of the existing `Callback` (the part after `code := c.Query("code")`) and keep it, but replace any reference to `h.oauth2`/`h.verifier`/`h.cfg.Auth.GitLab.*` with the per-request `oauth2Config`/`verifier`/`gl` variables. Specifically:
- `h.oauth2.Exchange` → `oauth2Config.Exchange`
- `h.verifier.Verify` → `verifier.Verify`
- `h.cfg.Auth.GitLab.FrontendCallbackURL` → `gl.FrontendCallbackURL` (already in `frontendURL`)
- `h.cfg.Auth.JWT.Secret`/`ExpireHours` → `h.cfg.Auth.JWT.Secret` (stays) + read `auth.jwt.expire_hours` from settings: add a helper `h.jwtExpireHours() int` that reads `settings.Get("auth.jwt.expire_hours", &n)` with fallback to `h.cfg.Auth.JWT.ExpireHours`.

Add the helper:
```go
func (h *GitLabHandler) jwtExpireHours() int {
	var n int
	if err := h.settings.Get("auth.jwt.expire_hours", &n); err != nil || n <= 0 {
		return h.cfg.Auth.JWT.ExpireHours
	}
	return n
}
```
And use `h.jwtExpireHours()` in the `GenerateJWT` call inside `Callback`.

Remove the now-unused `oauth2`/`provider`/`verifier` struct fields (they're per-request now). Remove the `"strings"` import if it becomes unused (it was used in the redirect-URL manipulation — keep if still referenced).

- [ ] **Step 2: Verify the auth package compiles**

Run: `go build ./internal/auth/`
Expected: exit 0. (`util` is imported — `util.Error` is used. If `util` wasn't imported in gitlab.go, add it.) `go build ./...` still fails elsewhere (handlers reference the new `NewGitLabHandler` signature) — expected.

- [ ] **Step 3: Commit**

```bash
git add internal/auth/gitlab.go
git commit -m "refactor(auth): GitLab handler reads settings per-request (no boot Fatalf)"
```

---

## Phase C: Handlers get `settings`; CORS/local/auth-status rewired

### Task C1: `CORSMiddleware(settings *SettingsStore)`

**Files:**
- Modify: `internal/api/middleware.go`

- [ ] **Step 1: Rewrite `CORSMiddleware`**

Replace `CORSMiddleware(cfg config.CORS) gin.HandlerFunc` with a version that reads from the `SettingsStore` per-request:

```go
func CORSMiddleware(settings *config.SettingsStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cors config.CORSConfig
		_ = settings.Get("cors", &cors)
		if len(cors.AllowedOrigins) == 0 {
			c.Next()
			return
		}
		origin := c.Request.Header.Get("Origin")
		allowOrigin := ""
		for _, o := range cors.AllowedOrigins {
			if o == "*" {
				allowOrigin = "*"
				break
			}
			if o == origin {
				allowOrigin = origin
				break
			}
		}
		if allowOrigin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, PATCH, DELETE")
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}
		c.Next()
	}
}
```

Define `CORSConfig` in `internal/config/settings.go` (the settings value struct):
```go
// CORSConfig is the JSON shape of the `cors` settings row.
type CORSConfig struct {
	AllowedOrigins []string `json:"allowed_origins"`
}
// LoggerConfig is the JSON shape of the `logger` settings row.
type LoggerConfig struct {
	Level string `json:"level"`
	File  string `json:"file"`
}
// LocalAuthConfig is the JSON shape of the `auth.local` settings row.
type LocalAuthConfig struct {
	Enabled bool `json:"enabled"`
}
// GitLabConfig is the JSON shape of the `auth.gitlab` settings row.
type GitLabConfig struct {
	App                 string `json:"app"`
	URL                 string `json:"url"`
	ClientID            string `json:"client_id"`
	ClientSecret        string `json:"client_secret"`
	RedirectURI         string `json:"redirect_uri"`
	FrontendCallbackURL string `json:"frontend_callback_url"`
}
```

Add the `"github.com/ZJUSCT/CSOJ/internal/config"` import to `middleware.go` (replace the old `config.CORS` usage).

- [ ] **Step 2: Verify the api package compiles (modulo handler fallout)**

Run: `go build ./internal/api/ 2>&1 | grep -v "handler.go\|router.go" | head`
Expected: no middleware.go errors.

- [ ] **Step 3: Commit**

```bash
git add internal/api/middleware.go internal/config/settings.go
git commit -m "refactor(api): CORSMiddleware reads cors from SettingsStore per-request"
```

### Task C2: `Handler` structs + `RegisterRoutes` get `settings`

**Files:**
- Modify: `internal/api/admin/handler.go`, `internal/api/admin/router.go`, `internal/api/user/handler.go`, `internal/api/user/router.go`

- [ ] **Step 1: Admin `Handler` + `NewHandler` + `RegisterRoutes`**

In `internal/api/admin/handler.go`:
- Add `settings *config.SettingsStore` to the `Handler` struct.
- `NewHandler(cfg *config.Config, settings *config.SettingsStore, db *gorm.DB, scheduler *judger.Scheduler, appState *judger.AppState) *Handler` — store `settings`.
- Add `import "github.com/ZJUSCT/CSOJ/internal/config"`.

In `internal/api/admin/router.go`:
- `RegisterRoutes(r *gin.Engine, cfg *config.Config, settings *config.SettingsStore, db *gorm.DB, scheduler *judger.Scheduler, appState *judger.AppState)` — pass `settings` to `NewHandler`.
- `adminV1.Use(api.AdminMiddleware(cfg.Auth.JWT.Secret, db))` — unchanged (jwt secret stays in cfg).
- Add `import "github.com/ZJUSCT/CSOJ/internal/config"`.

- [ ] **Step 2: User `Handler` + `NewHandler` + `RegisterRoutes`**

In `internal/api/user/handler.go`:
- Add `settings *config.SettingsStore` to the `Handler` struct.
- `NewHandler(cfg *config.Config, settings *config.SettingsStore, db *gorm.DB, scheduler *judger.Scheduler, appState *judger.AppState) *Handler`.
- `gitlabAuthHandler: auth.NewGitLabHandler(cfg, settings, db)` (new signature from B2).

In `internal/api/user/router.go`:
- `RegisterRoutes(r *gin.Engine, cfg *config.Config, settings *config.SettingsStore, db *gorm.DB, scheduler *judger.Scheduler, appState *judger.AppState)`.
- Pass `settings` to `NewHandler`.

- [ ] **Step 3: Verify the api packages compile (modulo auth-status/local still reading cfg)**

Run: `go build ./internal/api/... 2>&1 | grep -v "auth.go\|submission.go\|cluster.go\|settings.go" | head`
Expected: no handler.go/router.go errors. (auth.go reads `cfg.Auth.Local.Enabled` — fixed in C3; cluster.go/settings.go are new files — fixed in C4/E.)

- [ ] **Step 4: Commit**

```bash
git add internal/api/admin/handler.go internal/api/admin/router.go internal/api/user/handler.go internal/api/user/router.go
git commit -m "refactor(api): Handler + RegisterRoutes take SettingsStore"
```

### Task C3: `getAuthStatus` + `localLogin` read `auth.local` from settings

**Files:**
- Modify: `internal/api/user/auth.go`

- [ ] **Step 1: `getAuthStatus` reads the toggle from settings**

Replace the `getAuthStatus` body:
```go
func (h *Handler) getAuthStatus(c *gin.Context) {
	var local config.LocalAuthConfig
	_ = h.settings.Get("auth.local", &local)
	util.Success(c, gin.H{
		"local_auth_enabled": local.Enabled,
	}, "Auth status retrieved")
}
```
Add `import "github.com/ZJUSCT/CSOJ/internal/config"`.

- [ ] **Step 2: `localLogin`/`localRegister` guard on the toggle**

In `localLogin` and `localRegister`, replace the `if cfg.Auth.Local.Enabled` guard (if any — check the current code; the router only registers the local routes when `cfg.Auth.Local.Enabled` is true at boot). Since the toggle is now live, the routes should be registered unconditionally and the handlers check the toggle:

In `router.go` (user), change the `if cfg.Auth.Local.Enabled { localAuthGroup := ... }` block to register `local/login` + `local/register` unconditionally. Then in `localLogin`/`localRegister`, add at the top:
```go
	var local config.LocalAuthConfig
	_ = h.settings.Get("auth.local", &local)
	if !local.Enabled {
		util.Error(c, http.StatusNotFound, "local auth is disabled")
		return
	}
```

- [ ] **Step 3: Verify**

Run: `go build ./internal/api/user/`
Expected: exit 0 (admin package may still fail on cluster.go/settings.go — fixed in D/E).

- [ ] **Step 4: Commit**

```bash
git add internal/api/user/auth.go internal/api/user/router.go
git commit -m "refactor(user): auth status + local-auth guard read from settings"
```

---

## Phase D: Scheduler reads clusters from DB + `ReloadClusters`

### Task D1: `NewScheduler(db, settings, appState)` + `ReloadClusters`

**Files:**
- Modify: `internal/judger/scheduler.go`, `internal/judger/recovery.go`, `internal/judger/heartbeat.go`

- [ ] **Step 1: `NewScheduler` reads clusters from the DB**

In `internal/judger/scheduler.go`, change the signature:
```go
func NewScheduler(db *gorm.DB, settings *config.SettingsStore, appState *AppState) *Scheduler
```
Drop the `cfg *config.Config` parameter. Replace the `for i := range cfg.Cluster { cc := cfg.Cluster[i] ... }` loop with:
```go
	dbClusters, err := database.GetAllClusters(db)
	if err != nil {
		zap.S().Fatalf("failed to load clusters from DB: %v", err)
	}
	for _, cc := range dbClusters {
		clusterState, err := buildClusterState(db, cc)
		if err != nil {
			zap.S().Warnf("cluster %s failed to init: %v (skipping)", cc.Name, err)
			continue
		}
		clusters[cc.Name] = clusterState
	}
```

Extract the per-cluster clientset/pool construction into a helper `buildClusterState(db *gorm.DB, cc models.Cluster) (*ClusterState, error)` that:
- Parses the kubeconfig text: `cfg, err := clientcmd.Load([]byte(cc.Kubeconfig))` → `clientcmd.NewNonInteractiveClientConfig(*cfg, cc.Context, &overrides, nil)` → `restCfg`.
- Builds `kubernetes.Interface` + `dynamic.Interface`.
- Loads that cluster's pools (`database.GetClusterPools(db, cc.Name)`).
- Builds the `sem chan struct{}` of size `cc.Concurrency` (default 1 if ≤0).
- Probes MPI.
- Returns the `*ClusterState` (with `sem`/`queue`/`mpiEnabled`/`pools`).

Store `db` and `settings` on the `Scheduler` struct (add `settings *config.SettingsStore` field; `db` is already there).

- [ ] **Step 2: `ReloadClusters()`**

Append:
```go
func (s *Scheduler) ReloadClusters() ([]string, error) {
	dbClusters, err := database.GetAllClusters(s.db)
	if err != nil {
		return nil, err
	}
	newMap := make(map[string]*ClusterState, len(dbClusters))
	var warnings []string
	for _, cc := range dbClusters {
		cs, err := buildClusterState(s.db, cc)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("cluster %s: %v", cc.Name, err))
			continue
		}
		// Preserve the existing queue if this cluster already exists (in-flight work).
		if old, ok := s.clusters[cc.Name]; ok {
			cs.queue = old.queue
		} else {
			cs.queue = make(chan QueuedSubmission, 1024)
		}
		newMap[cc.Name] = cs
	}
	s.Lock()
	s.clusters = newMap
	s.Unlock()
	return warnings, nil
}
```

Add a `sync.Mutex` to `Scheduler` (if not present) for the `s.clusters` swap; the existing per-`ClusterState` mutex covers node maps. Actually `Scheduler` already has no top-level mutex — add one (`sync.Mutex` field) for the `clusters` map swap, and use it in `GetClusterStates`/`Submit`/`GetQueueLengths` reads too (or use `sync.RWMutex`). Simplest: add `mu sync.RWMutex` to `Scheduler`, RLock in readers, Lock in `ReloadClusters`.

- [ ] **Step 3: `RecoverAndCleanup` drops `cfg`**

In `internal/judger/recovery.go`, change `RecoverAndCleanup(db *gorm.DB, cfg *config.Config, instanceID string)` to `RecoverAndCleanup(db *gorm.DB, instanceID string)`. Replace the `for i := range cfg.Cluster { cc := cfg.Cluster[i] ... }` loops with `dbClusters, _ := database.GetAllClusters(db)` and iterate `dbClusters`. `buildKubeManagerForCluster` now takes `models.Cluster` (not `config.Cluster`) and parses the kubeconfig text. `HeartbeatTTL` comes from `cc.HeartbeatTTL` (int seconds).

- [ ] **Step 4: Heartbeat reads `cc.HeartbeatTTL` from the DB row**

`main.go`'s heartbeat loop (Phase E) iterates `database.GetAllClusters(db)`; `cc.HeartbeatTTL` (int seconds) → `time.Duration(cc.HeartbeatTTL) * time.Second`. No change to `heartbeat.go` itself (`StartHeartbeat(db, clusterName, instanceID, ttl, stop)` already takes a `time.Duration`).

- [ ] **Step 5: Verify the judger package compiles**

Run: `go build ./internal/judger/`
Expected: exit 0. (`go build ./...` still fails on `cmd/CSOJ/main.go` and the admin settings/cluster handlers — fixed in E/F.)

- [ ] **Step 6: Commit**

```bash
git add internal/judger/scheduler.go internal/judger/recovery.go
git commit -m "refactor(judger): NewScheduler reads clusters from DB; ReloadClusters()"
```

---

## Phase E: Admin settings + cluster endpoints + `ReloadClusters` route

### Task E1: `internal/api/admin/settings.go`

**Files:**
- Create: `internal/api/admin/settings.go`, modify `internal/api/admin/router.go`

- [ ] **Step 1: Settings handlers**

Create `internal/api/admin/settings.go`:

```go
package admin

import (
	"net/http"

	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

// listSettings returns all settings rows + boot facts for display.
func (h *Handler) listSettings(c *gin.Context) {
	rows, err := h.settings.ListAll()
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, gin.H{
		"settings": rows,
		"boot": gin.H{
			"listen":             h.cfg.Listen,
			"storage":            h.cfg.Storage,
			"jwt_secret_present": h.cfg.Auth.JWT.Secret != "",
		},
	}, "Settings retrieved")
}

// updateSetting writes a settings value.
func (h *Handler) updateSetting(c *gin.Context) {
	key := c.Param("key")
	var body struct {
		Value interface{} `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if err := h.settings.Set(key, body.Value); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	restart := key == "logger"
	util.Success(c, gin.H{"restart_required": restart}, "Setting updated")
}
```

Add a `ListAll()` method to `SettingsStore` (in `internal/config/settings.go`) that returns `map[string]string` (a copy of the cache):
```go
func (s *SettingsStore) ListAll() (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.cache))
	for k, v := range s.cache {
		out[k] = v
	}
	return out, nil
}
```

- [ ] **Step 2: Register routes**

In `internal/api/admin/router.go`, inside `adminV1`, add:
```go
		settings := adminV1.Group("/settings")
		{
			settings.GET("", h.listSettings)
			settings.PUT("/:key", h.updateSetting)
		}
```

- [ ] **Step 3: Verify**

Run: `go build ./internal/api/admin/ 2>&1 | grep -v "cluster.go" | head`
Expected: no settings.go errors. (cluster.go still red — E2.)

- [ ] **Step 4: Commit**

```bash
git add internal/api/admin/settings.go internal/api/admin/router.go internal/config/settings.go
git commit -m "feat(admin): settings list + update endpoints"
```

### Task E2: Cluster CRUD + `reload` route

**Files:**
- Modify: `internal/api/admin/cluster.go`, `internal/api/admin/router.go`

- [ ] **Step 1: Add cluster-row CRUD + reload handlers**

Append to `internal/api/admin/cluster.go`:

```go
func (h *Handler) listClusters(c *gin.Context) {
	rows, err := database.GetAllClusters(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	// Omit kubeconfig text from the list response (may contain secrets).
	type clusterSummary struct {
		Name         string `json:"name"`
		Context      string `json:"context"`
		Namespace    string `json:"namespace"`
		Concurrency  int    `json:"concurrency"`
		HeartbeatTTL int    `json:"heartbeat_ttl"`
	}
	out := make([]clusterSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, clusterSummary{
			Name: r.Name, Context: r.Context, Namespace: r.Namespace,
			Concurrency: r.Concurrency, HeartbeatTTL: r.HeartbeatTTL,
		})
	}
	util.Success(c, out, "Clusters retrieved")
}

func (h *Handler) createCluster(c *gin.Context) {
	var cl models.Cluster
	if err := c.ShouldBindJSON(&cl); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if err := database.UpsertCluster(h.db, &cl); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, cl, "Cluster created")
}

func (h *Handler) updateCluster(c *gin.Context) {
	name := c.Param("name")
	var cl models.Cluster
	if err := c.ShouldBindJSON(&cl); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if name != cl.Name {
		util.Error(c, http.StatusBadRequest, "cluster name in path does not match body")
		return
	}
	if err := database.UpsertCluster(h.db, &cl); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, cl, "Cluster updated")
}

func (h *Handler) deleteCluster(c *gin.Context) {
	name := c.Param("name")
	if err := database.DeleteCluster(h.db, name); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "Cluster deleted")
}

func (h *Handler) reloadClusters(c *gin.Context) {
	warnings, err := h.scheduler.ReloadClusters()
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, gin.H{"warnings": warnings}, "Clusters reloaded")
}
```

Add imports `"github.com/ZJUSCT/CSOJ/internal/database"` and `"github.com/ZJUSCT/CSOJ/internal/database/models"` to `cluster.go` (some may already be present).

- [ ] **Step 2: Register cluster-row routes**

In `internal/api/admin/router.go`, inside `adminV1`, add a `clusters` group (distinct from the existing `clusters` group that holds `/status` + `/pools` — actually that group is `/admin/clusters`; fold these in):

The existing `clusters := adminV1.Group("/clusters")` block already has `/status`, `/:cluster/pools`, `:cluster/concurrency`. Add:
```go
			clusters.GET("", h.listClusters)
			clusters.POST("", h.createCluster)
			clusters.PUT("/:name", h.updateCluster)
			clusters.DELETE("/:name", h.deletePool)
			clusters.POST("/reload", h.reloadClusters)
```
Note: `DELETE /:name` vs `DELETE /:cluster/pools/:pool` — Gin routes `/clusters/:name` and `/clusters/:cluster/pools/:pool` are distinguishable (different path depths). Verify Gin doesn't conflict on `:name` vs `:cluster` at the same segment — they're different paths (`/clusters/:name` vs `/clusters/:cluster/pools/:pool`), so no conflict.

Actually the `DELETE /:name` could clash with `DELETE /:cluster/pools/:pool`? No — `/clusters/:name` (2 segments) vs `/clusters/:cluster/pools/:pool` (4 segments). Distinct.

- [ ] **Step 3: Verify the admin package compiles**

Run: `go build ./internal/api/admin/`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/api/admin/cluster.go internal/api/admin/router.go
git commit -m "feat(admin): cluster-row CRUD + reload endpoint"
```

---

## Phase F: Boot flow (`main.go`)

### Task F1: `main.go` uses `SettingsStore` + DB clusters

**Files:**
- Modify: `cmd/CSOJ/main.go`

- [ ] **Step 1: Build `SettingsStore` + logger from settings; rewired boot**

In `cmd/CSOJ/main.go`, replace the logger build + recovery + scheduler + heartbeat + CORS + RegisterRoutes calls:

```go
	// SettingsStore (runtime settings from the DB)
	settings := config.NewSettingsStore(db)

	// Logger (level/file from settings; restart-required to change)
	var loggerCfg config.LoggerConfig
	_ = settings.Get("logger", &loggerCfg)
	if loggerCfg.Level == "" {
		loggerCfg.Level = "info"
	}
	var zapCfg zap.Config
	if loggerCfg.Level == "debug" {
		zapCfg = zap.NewDevelopmentConfig()
	} else {
		zapCfg = zap.NewProductionConfig()
	}
	if loggerCfg.File != "" {
		zapCfg.OutputPaths = []string{"stdout", loggerCfg.File}
		zapCfg.ErrorOutputPaths = []string{"stderr", loggerCfg.File}
	} else {
		zapCfg.OutputPaths = []string{"stdout"}
		zapCfg.ErrorOutputPaths = []string{"stderr"}
	}
	logger, err := zapCfg.Build()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	// recovery (clusters from DB)
	instanceID := uuid.NewString()
	if err := judger.RecoverAndCleanup(db, instanceID); err != nil {
		zap.S().Errorf("failed to recover and cleanup: %v", err)
	}

	// appState
	appState := &judger.AppState{ /* ... unchanged ... */ }
	contests, problems, problemToContestMap, err := judger.LoadFromDB(db)
	// ... unchanged ...
	appState.Contests = contests
	appState.Problems = problems
	appState.ProblemToContestMap = problemToContestMap

	// scheduler (clusters from DB)
	scheduler := judger.NewScheduler(db, settings, appState)
	if err := judger.RequeuePendingSubmissions(db, scheduler, appState); err != nil {
		zap.S().Fatalf("failed to requeue pending submissions: %v", err)
	}

	// Heartbeats (one per DB cluster row)
	hbStop := make(chan struct{})
	dbClusters, _ := database.GetAllClusters(db)
	for _, cc := range dbClusters {
		ttl := time.Duration(cc.HeartbeatTTL) * time.Second
		if ttl <= 0 {
			ttl = 30 * time.Second
		}
		go judger.StartHeartbeat(db, cc.Name, instanceID, ttl, hbStop)
	}

	go scheduler.Run()
	zap.S().Info("judger scheduler started")

	// API engine
	r := gin.Default()
	r.Use(api.CORSMiddleware(settings))
	user.RegisterRoutes(r, cfg, settings, db, scheduler, appState)
	admin.RegisterRoutes(r, cfg, settings, db, scheduler, appState)
	embedui.RegisterUIHandlers(r, "user")

	go func() {
		zap.S().Infof("starting server at %s", cfg.Listen)
		if err := r.Run(cfg.Listen); err != nil {
			zap.S().Fatalf("failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	close(hbStop)
	zap.S().Info("shutting down server...")
```

Add imports: `"github.com/ZJUSCT/CSOJ/internal/config"`, `"github.com/ZJUSCT/CSOJ/internal/database"`. Remove now-unused (`"sync"` if the appState literal no longer needs it — keep if `sync.RWMutex{}` is still in the literal).

- [ ] **Step 2: Verify the whole project builds**

Run: `go build ./...`
Expected: exit 0.

Run: `go vet ./...`
Expected: clean.

Run: `go test ./internal/config/ ./internal/judger/podspec/`
Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add -f cmd/CSOJ/main.go
git commit -m "refactor(main): boot uses SettingsStore + DB clusters + per-request CORS"
```

---

## Phase G: Docs

### Task G1: Update docs

**Files:**
- Modify: `docs/configuration/main-config.md`, `docs/getting-started.md`, `docs/api-reference/admin-api.md`

- [ ] **Step 1: `main-config.md`** — config.yaml sample shrinks to:
```yaml
listen: ":8080"
storage:
  database: "data/csoj.db"
  user_avatar: "data/avatars"
  submission_content: "data/submissions"
  submission_log: "data/logs"
auth:
  jwt:
    secret: "change-me"
    expire_hours: 72   # overridden by settings at runtime
```
Add a "Runtime Settings (database)" section listing the admin-managed keys (`logger`, `cors`, `auth.local`, `auth.gitlab`, `auth.jwt.expire_hours`, `clusters`) + their endpoints. Remove the `cluster` config block docs (clusters are now DB rows via `POST /admin/clusters`).

- [ ] **Step 2: `getting-started.md`** — update the sample config.yaml to the shrunk form; note "after first boot, configure logger/CORS/GitLab/clusters via the admin panel (`GET/PUT /api/v1/admin/settings`, `POST /api/v1/admin/clusters`)". Remove Docker/k3s cluster-block prereqs from the config section (the K8s cluster + kubeconfig are now uploaded via the admin API).

- [ ] **Step 3: `admin-api.md`** — add:
  - `GET /api/v1/admin/settings` — list settings + boot facts.
  - `PUT /api/v1/admin/settings/:key` — update a setting (body `{"value": ...}`; logger is restart-required).
  - `GET /api/v1/admin/clusters` — list cluster rows (kubeconfig omitted).
  - `POST /api/v1/admin/clusters` — create a cluster (body `{name, kubeconfig, context, namespace, concurrency, heartbeat_ttl}`).
  - `PUT /api/v1/admin/clusters/:name` — update (including kubeconfig).
  - `DELETE /api/v1/admin/clusters/:name` — delete.
  - `POST /api/v1/admin/clusters/reload` — rebuild clientsets; returns warnings.

- [ ] **Step 4: Commit**

```bash
git add docs/
git commit -m "docs: config.yaml shrinks; runtime settings + clusters admin-managed"
```

---

## Phase H: Verification

### Task H1: Build + tests

**Files:** (no changes — verification)

- [ ] **Step 1: Build + vet + tests**

Run: `go build ./... && go vet ./... && go test ./internal/config/ ./internal/judger/podspec/`
Expected: clean; all tests pass.

- [ ] **Step 2: No commit**

### Task H2: UI smoke test (existing :18080 instance)

**Files:** (no changes — manual)

- [ ] **Step 1: Rebuild + restart the UI instance with the shrunk config**

Write `/tmp/csoj-ui-test/configs/config.yaml`:
```yaml
listen: ":18080"
storage:
  database: "data/csoj.db"
  user_avatar: "data/avatars"
  submission_content: "data/submissions"
  submission_log: "data/logs"
auth:
  jwt:
    secret: "ui-demo-secret"
    expire_hours: 72
```
(No `cluster`, no `logger`, no `cors`, no `gitlab`, no `local`.)

`make build` → restart the server on `:18080`.

- [ ] **Step 2: Register first user → superadmin; verify the admin endpoints**

```bash
# register + login (local auth: settings row missing → defaults to disabled!)
# So first SET local auth enabled via the admin... but there's no admin yet.
```

**Bootstrapping gotcha:** with `auth.local` defaulting to disabled (no settings row), the first user can't register. Two options:
1. `auth.local` defaults to **enabled** when the setting is missing (bootstrapping-friendly). Change the `getAuthStatus`/`localLogin`/`localRegister` default: `var local config.LocalAuthConfig; _ = settings.Get("auth.local", &local); if !local.Enabled && !settings.HasKey("auth.local") { local.Enabled = true }` — i.e. missing row = enabled.
2. Seed the `auth.local` setting row at AutoMigrate time if absent.

**Decision: option 1** — missing `auth.local` row = enabled (so a fresh install can register the first superadmin, then disable local auth via the panel). Update Task C3's `getAuthStatus`/guards accordingly (add a `settings.HasKey(key)` helper).

- [ ] **Step 3: Verify the full flow**

Register → superadmin. `PUT /admin/settings/cors` → `{"value":{"allowed_origins":["*"]}}`; verify CORS headers. `PUT /admin/settings/auth.local` → `{"value":{"enabled":false}}`; verify `/auth/status` shows `false` and `/auth/local/login` 404s. `POST /admin/clusters` with a fake kubeconfig → `POST /admin/clusters/reload` → no crash, warning logged. Restart → settings persist.

- [ ] **Step 4: No commit**

### Task H3: `verify` skill

- [ ] Invoke the `verify` skill to drive: boot with shrunk config, first-user→superadmin (local-auth default-enabled), set CORS, disable local-auth, create a cluster row, reload clusters. Capture the result.

---

## Self-Review notes (for the implementer)

- The bootstrapping default (`auth.local` missing = enabled) is critical — without it, a fresh install can't register the first user. Implement `SettingsStore.HasKey(key)` and use it in `getAuthStatus`/`localLogin`/`localRegister`.
- `NewScheduler` now takes `(db, settings, appState)` — update the `cmd/CSOJ/main.go` call site (Phase F).
- `RecoverAndCleanup` drops the `cfg` param — update `main.go`.
- `CORSMiddleware` takes `*SettingsStore` — update `main.go` (no more `cfg.CORS`).
- The `GitLabHandler.Login`/`Callback` now return HTTP errors instead of boot-crashing — verify the user-facing `/auth/gitlab/login` returns 503 "gitlab not configured" when unset.
- `Scheduler` needs a top-level `sync.RWMutex` for the `clusters` map swap in `ReloadClusters`; guard the readers (`GetClusterStates`/`Submit`/`GetQueueLengths`).
- The admin frontend still calls the old endpoints — the settings/cluster APIs are new; the frontend settings page is a separate follow-up (documented). For manual testing use `curl`.
- `clientcmd.Load([]byte(kubeconfigText))` parses kubeconfig text — verify it handles the multi-doc YAML a kubeconfig file typically is (it does; `clientcmd.Load` returns `*DirectClientConfig` from raw bytes).
