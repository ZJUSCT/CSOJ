# Merge WebUI + AdminPanel into CSOJ — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Consolidate the three CSOJ repos into one: move `CSOJ-WebUI` into `CSOJ/frontend/`, merge `CSOJ-AdminPanel`'s admin code into that frontend behind a role gate, merge the two backend ports into one with `/api/v1/admin/*` routes guarded by a new `AdminMiddleware`, and build the frontend in-repo via a Makefile.

**Architecture:** Single Go binary on one port. `User.Role` field + JWT `role` claim; first registered user becomes `superadmin`; superadmins promote/demote admins. Admin routes prefixed `/api/v1/admin/*` with group-level `AdminMiddleware` (JWT + role check). Frontend is a Next.js static export built by pnpm and embedded via `go:embed`; admin pages live under `app/(main)/admin/*`, reachable from an "Admin" top-nav link shown only to admins.

**Tech Stack:** Go 1.24, Gin, GORM/SQLite, golang-jwt/v5 (backend); Next.js 14 App Router (static export), React 18, TypeScript, pnpm, shadcn/ui, Tailwind, axios/SWR, next-intl (frontend).

**Reference spec:** `docs/superpowers/specs/2026-07-05-merge-webui-admin-design.md`

---

## File Structure

### Backend (Go) — files to modify/create

- **Modify** `internal/database/models/models.go` — add `Role` type + `User.Role` field
- **Modify** `internal/database/crud.go` — add `CountUsers` helper
- **Modify** `internal/auth/jwt.go` — add `Role` claim to `MyCustomClaims`, update `GenerateJWT` signature
- **Modify** `internal/api/middleware.go` — `AuthMiddleware` sets `role` on context; add `AdminMiddleware`, `SuperAdminMiddleware`
- **Modify** `internal/api/user/auth.go` — `localRegister`/`localLogin` pass role; first-user → superadmin bootstrap
- **Modify** `internal/auth/gitlab.go` — `Callback` passes role; first-user → superadmin bootstrap
- **Modify** `internal/api/user/profile.go` — `getUserProfile` returns role (via JSON tag); `PublicProfileResponse` excludes role
- **Modify** `internal/api/user/router.go` — `NewUserRouter` → `RegisterRoutes(r *gin.Engine, ...)`
- **Modify** `internal/api/admin/router.go` — `NewAdminRouter` → `RegisterRoutes(r *gin.Engine, ...)`; mount under `/api/v1/admin`; add `PATCH /users/:id/role`
- **Modify** `internal/api/admin/user.go` — add `updateUserRole` handler
- **Modify** `internal/config/config.go` — remove `Admin` struct + `Admin.Enabled`/`Admin.Listen`
- **Modify** `cmd/CSOJ/main.go` — single engine; call `user.RegisterRoutes` + `admin.RegisterRoutes` + `embedui.RegisterUIHandlers(r, "user")`
- **Modify** `internal/embedui/ui.go` — (no logic change; `sites/admin` no longer used but `RegisterUIHandlers` signature stays)
- **Delete** `internal/embedui/sites/admin/.index.html` and the `sites/admin/` dir
- **Create** `Makefile`
- **Modify** `.gitignore`
- **Modify** `.github/workflows/build.yml`
- **Modify** `docs/configuration/main-config.md`, `docs/getting-started.md`, `docs/api-reference/admin-api.md`, `docs/index.md`

### Frontend (Next.js) — move + merge

- **Create** `frontend/` — copy of `CSOJ-WebUI/` (excluding `.git`, `node_modules`, `out`)
- **Modify** `frontend/package.json` — union deps (add `@radix-ui/react-checkbox`, `@radix-ui/react-select`, `@radix-ui/react-icons`, `react-beautiful-dnd`, `@types/react-beautiful-dnd`, `js-yaml`, `@types/js-yaml`)
- **Modify** `frontend/lib/utils.ts` — add `formatBytes`
- **Modify** `frontend/lib/types.ts` — add `role` to `User`; add admin types (`PaginatedResponse`, `UserBestScore`, `AssetFile`, `ConfigNode`, `NodeState`, `ClusterState`, `ClusterStatusResponse`, `NodeDetail`)
- **Modify** `frontend/lib/api.ts` — keep axios instance + interceptors (no change to baseURL)
- **Modify** `frontend/providers/swr-provider.tsx` — add AdminPanel's `onError` toast
- **Create** `frontend/components/ui/checkbox.tsx`, `select.tsx`, `sheet.tsx` (from AdminPanel)
- **Create** `frontend/components/shared/pagination-controls.tsx`, `refresh-interval-selector.tsx`, `strict-mode-droppable.tsx` (from AdminPanel)
- **Create** `frontend/components/admin/*` (10 files from AdminPanel) — with API paths prefixed `/admin`
- **Create** `frontend/components/layout/admin-sub-nav.tsx` — secondary nav for admin pages
- **Create** `frontend/components/layout/with-admin.tsx` — role-gate HOC
- **Modify** `frontend/components/layout/main-nav.tsx` — add "Admin" link when `user.role ∈ {admin, superadmin}`
- **Create** `frontend/app/(main)/admin/{cluster,users,submissions,containers,contests,problems}/page.tsx` — from AdminPanel, prefixed `/admin`, wrapped in `withAdmin`
- **Modify** `frontend/tailwind.config.ts` — union tag-color safelist (likely already covered)

---

## Phase A: Backend role + auth foundations

### Task A1: Add `Role` to the User model

**Files:**
- Modify: `internal/database/models/models.go:36-52`

- [ ] **Step 1: Add the Role type and field**

Replace the top of `models.go` (the `Status` block) additions and the `User` struct. Add after the `Status` constants (after line 19):

```go
type Role string

const (
	RoleUser       Role = "user"
	RoleAdmin      Role = "admin"
	RoleSuperAdmin Role = "superadmin"
)
```

Add the `Role` field to the `User` struct (after `Tags`):

```go
type User struct {
	ID        string `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	GitLabID     *string    `gorm:"uniqueIndex" json:"-"`
	Username     string     `gorm:"uniqueIndex" json:"username"`
	PasswordHash string     `json:"-"`
	Nickname     string     `json:"nickname"`
	Signature    string     `json:"signature"`
	AvatarURL    string     `json:"avatar_url"`
	BannedUntil  *time.Time `json:"banned_until"`
	BanReason    string     `json:"ban_reason"`
	DisableRank  bool       `gorm:"default:false" json:"disable_rank"`
	Tags         string     `gorm:"type:text" json:"tags"` // Comma-separated tags
	Role         Role       `gorm:"type:text;default:'user';index" json:"role"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: exit 0 (GORM AutoMigrate will add the column at runtime; no migration code needed here).

- [ ] **Step 3: Commit**

```bash
git add internal/database/models/models.go
git commit -m "feat(db): add Role field to User model"
```

### Task A2: Add `CountUsers` helper

**Files:**
- Modify: `internal/database/crud.go` (add after `GetAllUsers`, ~line 58)

- [ ] **Step 1: Add the helper**

Insert after the `GetAllUsers` function:

```go
func CountUsers(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&models.User{}).Count(&count).Error
	return count, err
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: exit 0

- [ ] **Step 3: Commit**

```bash
git add internal/database/crud.go
git commit -m "feat(db): add CountUsers helper"
```

### Task A3: Add `Role` claim to JWT

**Files:**
- Modify: `internal/auth/jwt.go:8-12` (claims struct), `GenerateJWT` signature, callers

- [ ] **Step 1: Update the claims struct**

In `internal/auth/jwt.go`, replace the `MyCustomClaims` struct:

```go
type MyCustomClaims struct {
	Role string `json:"role,omitempty"`
	jwt.RegisteredClaims
}
```

- [ ] **Step 2: Update `GenerateJWT` signature**

Replace the `GenerateJWT` function:

```go
func GenerateJWT(userID, role, secret string, expireHours int) (string, error) {
	claims := MyCustomClaims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Subject:   userID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
```

- [ ] **Step 3: Update the `localLogin` caller**

In `internal/api/user/auth.go`, find the `GenerateJWT` call in `localLogin` and change:

```go
jwtToken, err := auth.GenerateJWT(user.ID, string(user.Role), h.cfg.Auth.JWT.Secret, h.cfg.Auth.JWT.ExpireHours)
```

- [ ] **Step 4: Update the GitLab `Callback` caller**

In `internal/auth/gitlab.go`, find the `GenerateJWT` call near line 146 and change:

```go
jwtToken, err := GenerateJWT(user.ID, string(user.Role), h.cfg.Auth.JWT.Secret, h.cfg.Auth.JWT.ExpireHours)
```

- [ ] **Step 5: Verify it compiles**

Run: `go build ./...`
Expected: exit 0

- [ ] **Step 6: Commit**

```bash
git add internal/auth/jwt.go internal/api/user/auth.go internal/auth/gitlab.go
git commit -m "feat(auth): add role claim to JWT"
```

### Task A4: First-user → superadmin bootstrap (local register)

**Files:**
- Modify: `internal/api/user/auth.go` `localRegister`

- [ ] **Step 1: Add bootstrap logic to `localRegister`**

In `internal/api/user/auth.go`, in `localRegister`, after building `newUser` and before `database.CreateUser`, insert:

```go
	// Bootstrap: the first registered user becomes superadmin.
	count, err := database.CountUsers(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "database error")
		return
	}
	if count == 0 {
		newUser.Role = models.RoleSuperAdmin
		zap.S().Infof("first user registered (%s); granting superadmin", newUser.Username)
	}
```

(`models` is already imported in this file.)

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: exit 0

- [ ] **Step 3: Commit**

```bash
git add internal/api/user/auth.go
git commit -m "feat(auth): first registered local user becomes superadmin"
```

### Task A5: First-user → superadmin bootstrap (GitLab callback)

**Files:**
- Modify: `internal/auth/gitlab.go` `Callback` (~line 128-139)

- [ ] **Step 1: Add bootstrap logic before `CreateUser`**

In `internal/auth/gitlab.go`, in `Callback`, just before `if err := database.CreateUser(h.db, &newUser); err != nil {` (line 135), insert:

```go
		// Bootstrap: the first registered user becomes superadmin.
		count, err := database.CountUsers(h.db)
		if err != nil {
			c.Redirect(http.StatusTemporaryRedirect, frontendURL+"database_error")
			return
		}
		if count == 0 {
			newUser.Role = models.RoleSuperAdmin
			zap.S().Infof("first user registered (%s); granting superadmin", newUser.Username)
		}
```

- [ ] **Step 2: Ensure imports**

Check `internal/auth/gitlab.go` imports include `"github.com/ZJUSCT/CSOJ/internal/database/models"` and `"go.uber.org/zap"`. If `models` is not imported, add it. If `zap` is not imported, add it.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: exit 0

- [ ] **Step 4: Commit**

```bash
git add internal/auth/gitlab.go
git commit -m "feat(auth): first GitLab user becomes superadmin"
```

### Task A6: `AuthMiddleware` sets `role` on context

**Files:**
- Modify: `internal/api/middleware.go:106` (the `c.Set("userID", ...)` line)

- [ ] **Step 1: Set role on context**

In `internal/api/middleware.go`, in `AuthMiddleware`, replace the final context set:

```go
		c.Set("userID", claims.Subject)
		c.Set("role", string(user.Role))
		c.Next()
```

(The `user` variable is already loaded earlier in the function via `database.GetUserByID`.)

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: exit 0

- [ ] **Step 3: Commit**

```bash
git add internal/api/middleware.go
git commit -m "feat(api): AuthMiddleware exposes role on context"
```

### Task A7: Add `AdminMiddleware` and `SuperAdminMiddleware`

**Files:**
- Modify: `internal/api/middleware.go` (append at end of file)

- [ ] **Step 1: Add the two middlewares**

Append to `internal/api/middleware.go`:

```go
// requireRole is a shared helper that runs AuthMiddleware then enforces a role.
func requireRole(secret string, db *gorm.DB, allowSuperAdminOnly bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			util.Error(c, http.StatusUnauthorized, "Authorization header is required")
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			util.Error(c, http.StatusUnauthorized, "Authorization header format must be Bearer {token}")
			c.Abort()
			return
		}

		claims, err := auth.ValidateJWT(parts[1], secret)
		if err != nil {
			util.Error(c, http.StatusUnauthorized, err.Error())
			c.Abort()
			return
		}

		userID := claims.Subject
		user, err := database.GetUserByID(db, userID)
		if err != nil {
			util.Error(c, http.StatusUnauthorized, "User not found")
			c.Abort()
			return
		}

		if user.BannedUntil != nil && time.Now().Before(*user.BannedUntil) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    -1,
				"message": "You have been banned from this service.",
				"data": gin.H{
					"ban_reason":   user.BanReason,
					"banned_until": user.BannedUntil.Format(time.RFC3339),
				},
			})
			c.Abort()
			return
		}

		// Use the DB-loaded role (source of truth), not the JWT claim, so a
		// demoted admin loses access even with a still-valid token.
		role := string(user.Role)
		if role != string(models.RoleAdmin) && role != string(models.RoleSuperAdmin) {
			util.Error(c, http.StatusForbidden, "admin privileges required")
			c.Abort()
			return
		}
		if allowSuperAdminOnly && role != string(models.RoleSuperAdmin) {
			util.Error(c, http.StatusForbidden, "superadmin privileges required")
			c.Abort()
			return
		}

		c.Set("userID", claims.Subject)
		c.Set("role", role)
		c.Next()
	}
}

// AdminMiddleware allows admin and superadmin users.
func AdminMiddleware(secret string, db *gorm.DB) gin.HandlerFunc {
	return requireRole(secret, db, false)
}

// SuperAdminMiddleware allows only superadmin users.
func SuperAdminMiddleware(secret string, db *gorm.DB) gin.HandlerFunc {
	return requireRole(secret, db, true)
}
```

- [ ] **Step 2: Ensure imports**

In `internal/api/middleware.go`, ensure the import block includes `"github.com/ZJUSCT/CSOJ/internal/database/models"`. If not, add it to the existing import group.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: exit 0

- [ ] **Step 4: Commit**

```bash
git add internal/api/middleware.go
git commit -m "feat(api): add AdminMiddleware and SuperAdminMiddleware"
```

---

## Phase B: Backend router refactor

### Task B1: Convert user router to `RegisterRoutes`

**Files:**
- Modify: `internal/api/user/router.go`

- [ ] **Step 1: Refactor the function signature and remove engine creation**

In `internal/api/user/router.go`, replace the function declaration and the first lines (the `r := gin.Default()` / `r.Use(...)` and the trailing `embedui.RegisterUIHandlers`):

Change:
```go
func NewUserRouter(
	cfg *config.Config,
	db *gorm.DB,
	scheduler *judger.Scheduler,
	appState *judger.AppState) *gin.Engine {

	r := gin.Default()

	r.Use(api.CORSMiddleware(cfg.CORS))
```
to:
```go
// RegisterRoutes mounts user routes onto the given engine.
func RegisterRoutes(
	r *gin.Engine,
	cfg *config.Config,
	db *gorm.DB,
	scheduler *judger.Scheduler,
	appState *judger.AppState) {
```

Remove the trailing two lines of the function:
```go
	embedui.RegisterUIHandlers(r, "user")

	return r
```
Replace them with a single closing brace:
```go
}
```

- [ ] **Step 2: Remove now-unused imports if any**

`embedui` is no longer used in this file — remove `"github.com/ZJUSCT/CSOJ/internal/embedui"` from the imports. Keep `api`, `config`, `judger`, `gin`, `gorm`.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: exit 0 (main.go still references `NewUserRouter`, so this will fail at main.go — that's fixed in Task B5. For now, verify only this package compiles:)

Run: `go build ./internal/api/user/`
Expected: exit 0

- [ ] **Step 4: Commit**

```bash
git add internal/api/user/router.go
git commit -m "refactor(api): user router becomes RegisterRoutes on shared engine"
```

### Task B2: Convert admin router to `RegisterRoutes` under `/api/v1/admin`

**Files:**
- Modify: `internal/api/admin/router.go`

- [ ] **Step 1: Replace the function declaration and engine setup**

Change:
```go
func NewAdminRouter(
	cfg *config.Config,
	db *gorm.DB,
	scheduler *judger.Scheduler,
	appState *judger.AppState) *gin.Engine {

	r := gin.Default()

	r.Use(api.CORSMiddleware(cfg.CORS))

	h := NewHandler(cfg, db, scheduler, appState)

	v1 := r.Group("/api/v1")
	{
```
to:
```go
// RegisterRoutes mounts admin routes onto the given engine under /api/v1/admin.
func RegisterRoutes(
	r *gin.Engine,
	cfg *config.Config,
	db *gorm.DB,
	scheduler *judger.Scheduler,
	appState *judger.AppState) {

	h := NewHandler(cfg, db, scheduler, appState)

	adminV1 := r.Group("/api/v1/admin")
	adminV1.Use(api.AdminMiddleware(cfg.Auth.JWT.Secret, db))
	{
```

- [ ] **Step 2: Rename all sub-group parents from `v1` to `adminV1`**

In the same file, replace every occurrence of the group parent variable name. Specifically, change:
- `users := v1.Group("/users")` → `users := adminV1.Group("/users")`
- `submissions := v1.Group("/submissions")` → `submissions := adminV1.Group("/submissions")`
- `contests := adminV1.Group("/contests")` (was `v1.Group`)
- `problems := adminV1.Group("/problems")` (was `v1.Group`)
- `scores := adminV1.Group("/scores")` (was `v1.Group`)
- `clusters := adminV1.Group("/clusters")` (was `v1.Group`)
- `containers := adminV1.Group("/containers")` (was `v1.Group`)

Also the WebSocket route:
- `v1.GET("/ws/submissions/:id/containers/:conID/logs", h.handleAdminContainerWs)` → `adminV1.GET("/ws/submissions/:id/containers/:conID/logs", h.handleAdminContainerWs)`
- `v1.POST("/reload", h.reload)` → `adminV1.POST("/reload", h.reload)`

- [ ] **Step 3: Remove the trailing `embedui.RegisterUIHandlers` and `return r`**

Replace:
```go
	embedui.RegisterUIHandlers(r, "admin")

	return r
}
```
with:
```go
}
```

- [ ] **Step 4: Remove now-unused imports**

Remove `"github.com/ZJUSCT/CSOJ/internal/embedui"` from imports. Keep `api`, `config`, `judger`, `gin`, `gorm`.

- [ ] **Step 5: Verify the package compiles**

Run: `go build ./internal/api/admin/`
Expected: exit 0

- [ ] **Step 6: Commit**

```bash
git add internal/api/admin/router.go
git commit -m "refactor(api): admin router mounts under /api/v1/admin with AdminMiddleware"
```

### Task B3: Add `updateUserRole` handler

**Files:**
- Modify: `internal/api/admin/user.go` (add handler), `internal/api/admin/router.go` (register route)

- [ ] **Step 1: Add the handler in `user.go`**

Append to `internal/api/admin/user.go`:

```go
// updateUserRole lets a superadmin promote a user to admin or demote back to user.
func (h *Handler) updateUserRole(c *gin.Context) {
	targetID := c.Param("id")
	target, err := database.GetUserByID(h.db, targetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.Error(c, http.StatusNotFound, "user not found")
		} else {
			util.Error(c, http.StatusInternalServerError, "database error")
		}
		return
	}

	var req struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	// Only allow setting admin or user (never superadmin via API).
	switch models.Role(req.Role) {
	case models.RoleAdmin, models.RoleUser:
		// ok
	default:
		util.Error(c, http.StatusBadRequest, "role must be 'admin' or 'user'")
		return
	}

	// Superadmin cannot demote themselves (prevent lockout).
	callerID := c.GetString("userID")
	if callerID == targetID && req.Role != string(models.RoleSuperAdmin) {
		// target is a superadmin being demoted by themselves → block
		if target.Role == models.RoleSuperAdmin {
			util.Error(c, http.StatusBadRequest, "you cannot demote yourself")
			return
		}
	}

	target.Role = models.Role(req.Role)
	if err := database.UpdateUser(h.db, target); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, target, "user role updated")
}
```

- [ ] **Step 2: Ensure imports in `user.go`**

Check `internal/api/admin/user.go` imports. Ensure it has `"github.com/ZJUSCT/CSOJ/internal/database/models"`, `"github.com/ZJUSCT/CSOJ/internal/database"`, `"github.com/ZJUSCT/CSOJ/internal/util"`, `"net/http"`, `"errors"`, `"gorm.io/gorm"`. Add any that are missing.

- [ ] **Step 3: Register the route (superadmin-only)**

The admin group already runs `AdminMiddleware` (which accepts admin or superadmin). The role-management endpoint needs to additionally require superadmin. To avoid double JWT validation, register it as a sibling route outside the `adminV1` group, with its own `SuperAdminMiddleware`:

In `internal/api/admin/router.go`, **after** the closing brace of the `adminV1 := r.Group("/api/v1/admin")` block (i.e. after the whole `adminV1` group is defined), add:

```go
	// Role management — superadmin only.
	// Sibling group to avoid stacking SuperAdminMiddleware on top of AdminMiddleware.
	roleMgmt := r.Group("/api/v1/admin")
	roleMgmt.Use(api.SuperAdminMiddleware(cfg.Auth.JWT.Secret, db))
	{
		roleMgmt.PATCH("/users/:id/role", h.updateUserRole)
	}
```

This yields the path `PATCH /api/v1/admin/users/:id/role`, guarded by `SuperAdminMiddleware` alone. (Gin matches the more specific `PATCH /users/:id/role` on this group before the broader `adminV1` `PATCH /users/:id` route — verify in Task H2 that the role endpoint returns 200 for a superadmin and 403 for a plain admin.)

- [ ] **Step 4: Verify the package compiles**

Run: `go build ./internal/api/admin/`
Expected: exit 0

- [ ] **Step 5: Commit**

```bash
git add internal/api/admin/user.go internal/api/admin/router.go
git commit -m "feat(api): superadmin-only PATCH /admin/users/:id/role"
```

### Task B4: Remove `Admin` config + single engine in main.go

**Files:**
- Modify: `internal/config/config.go:25,87-90`, `cmd/CSOJ/main.go:121-139`

- [ ] **Step 1: Remove the `Admin` config struct and field**

In `internal/config/config.go`:
- Delete line 25: `	Admin        Admin     `yaml:"admin"``
- Delete the `Admin` struct (lines 87-90):
```go
type Admin struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
}
```

- [ ] **Step 2: Update `main.go` to use a single engine**

In `cmd/CSOJ/main.go`, replace the API routers + server block (lines ~120-139):

```go
	// API routers
	userEngine := user.NewUserRouter(cfg, db, scheduler, appState)
	adminEngine := admin.NewAdminRouter(cfg, db, scheduler, appState)

	// start servers
	go func() {
		zap.S().Infof("starting user server at %s", cfg.Listen)
		if err := userEngine.Run(cfg.Listen); err != nil {
			zap.S().Fatalf("failed to start user server: %v", err)
		}
	}()

	if cfg.Admin.Enabled {
		go func() {
			zap.S().Infof("starting admin server at %s", cfg.Admin.Listen)
			if err := adminEngine.Run(cfg.Admin.Listen); err != nil {
				zap.S().Fatalf("failed to start admin server: %v", err)
			}
		}()
	}
```

with:

```go
	// Single API engine
	r := gin.Default()
	r.Use(api.CORSMiddleware(cfg.CORS))
	user.RegisterRoutes(r, cfg, db, scheduler, appState)
	admin.RegisterRoutes(r, cfg, db, scheduler, appState)
	embedui.RegisterUIHandlers(r, "user")

	// start server
	go func() {
		zap.S().Infof("starting server at %s", cfg.Listen)
		if err := r.Run(cfg.Listen); err != nil {
			zap.S().Fatalf("failed to start server: %v", err)
		}
	}()
```

- [ ] **Step 3: Update imports in main.go**

Add to the import block:
- `"github.com/ZJUSCT/CSOJ/internal/api"` (for `CORSMiddleware`)
- `"github.com/ZJUSCT/CSOJ/internal/embedui"`
- `"github.com/gin-gonic/gin"`

The `admin` and `user` imports stay. Remove any now-unused imports.

- [ ] **Step 4: Verify it compiles**

Run: `go build ./...`
Expected: exit 0

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go cmd/CSOJ/main.go
git commit -m "refactor: single Gin engine on one port; drop separate admin config"
```

### Task B5: Profile responses — exclude role from public profile

**Files:**
- Modify: `internal/api/user/profile.go:11-19` (`PublicProfileResponse`)

- [ ] **Step 1: Confirm `getUserProfile` already returns role**

`getUserProfile` returns the full `user` (with the `Role` JSON tag) — no change needed; `role` is included for the authenticated self-profile. Good.

- [ ] **Step 2: Verify `PublicProfileResponse` excludes role**

In `internal/api/user/profile.go`, the `PublicProfileResponse` struct has fields: ID, Username, Nickname, Signature, AvatarURL, Tags. It does **not** include Role — correct, no change needed.

- [ ] **Step 3: No commit needed (verification only)**

This task is a verification gate; no code changes. Move on.

### Task B6: Remove the admin embed site

**Files:**
- Delete: `internal/embedui/sites/admin/` (the `.index.html` placeholder)

- [ ] **Step 1: Delete the admin site directory**

Run:
```bash
git rm -r internal/embedui/sites/admin
```

(The `sites/` dir is gitignored, so `git rm` may report "did not match any files". If so, just delete from disk: `rm -rf internal/embedui/sites/admin` — it's not tracked anyway. The `sites/user/.index.html` placeholder stays so `//go:embed all:sites` still compiles when no frontend has been built yet.)

- [ ] **Step 2: Verify it still compiles**

Run: `go build ./...`
Expected: exit 0 (the `//go:embed all:sites` directive still finds `sites/user/.index.html`)

- [ ] **Step 3: Commit (only if files were tracked)**

```bash
git status
# If internal/embedui/sites/admin was tracked, commit the removal:
git commit -m "chore: remove admin embed site placeholder" || echo "nothing tracked, skip"
```

---

## Phase C: Backend build + docs

### Task C1: Add the Makefile

**Files:**
- Create: `Makefile`
- Modify: `.gitignore`

- [ ] **Step 1: Create the Makefile**

Create `Makefile` at repo root:

```makefile
.PHONY: build frontend embed backend clean dev-frontend

frontend:
	cd frontend && pnpm install && pnpm build

embed: frontend
	rm -rf internal/embedui/sites/user
	mkdir -p internal/embedui/sites/user
	cp -r frontend/out/. internal/embedui/sites/user/

backend:
	go build -ldflags "-X main.Version=$$(git describe --tags --always 2>/dev/null || echo dev)" -o CSOJ ./cmd/CSOJ

build: embed backend

dev-frontend:
	cd frontend && pnpm dev

clean:
	rm -rf frontend/out frontend/node_modules CSOJ
```

- [ ] **Step 2: Update `.gitignore`**

Replace the contents of `.gitignore` with:

```
test

configs
storage
contests

CSOJ
CSOJ.exe

venv

internal/embedui/sites

*.log

# Frontend
frontend/node_modules
frontend/.next
frontend/out
```

- [ ] **Step 3: Commit**

```bash
git add Makefile .gitignore
git commit -m "build: add Makefile to build frontend in-repo and embed it"
```

### Task C2: Update CI workflow

**Files:**
- Modify: `.github/workflows/build.yml` (steps "Download WebUI" + "Download AdminPanel")

- [ ] **Step 1: Replace the two download steps with a frontend build step**

In `.github/workflows/build.yml`, find and remove these two steps:

```yaml
      - name: Download WebUI
        run: |
          wget -O webui.tar.gz https://github.com/ZJUSCT/CSOJ-WebUI/releases/latest/download/site.tar.gz
          tar -xzf webui.tar.gz -C ./internal/embedui/sites/user


      - name: Download AdminPanel
        run: |
          wget -O adminpanel.tar.gz https://github.com/ZJUSCT/CSOJ-AdminPanel/releases/latest/download/site.tar.gz
          tar -xzf adminpanel.tar.gz -C ./internal/embedui/sites/admin
```

Replace them with:

```yaml
      - name: Setup pnpm
        uses: pnpm/action-setup@v4
        with:
          version: 10

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: pnpm
          cache-dependency-path: frontend/pnpm-lock.yaml

      - name: Build frontend
        run: |
          cd frontend
          pnpm install --frozen-lockfile
          pnpm build
          rm -rf ../internal/embedui/sites/user
          mkdir -p ../internal/embedui/sites/user
          cp -r out/. ../internal/embedui/sites/user/
```

- [ ] **Step 2: Verify the workflow YAML is valid**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/build.yml')); print('ok')"`
Expected: `ok`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/build.yml
git commit -m "ci: build frontend in-repo instead of downloading release tarballs"
```

### Task C3: Update docs

**Files:**
- Modify: `docs/configuration/main-config.md`, `docs/getting-started.md`, `docs/api-reference/admin-api.md`, `docs/index.md`

- [ ] **Step 1: Update `main-config.md`**

In `docs/configuration/main-config.md`, remove any documentation of the `admin.enabled` / `admin.listen` keys (search for `admin:` and remove that block). If the doc shows a sample `config.yaml`, delete the `admin:` section from it.

- [ ] **Step 2: Update `getting-started.md`**

In `docs/getting-started.md`:
- Remove mention of the separate admin port / `:8081`.
- Update the build instructions to reference `make build` (or `make backend` if the frontend is already embedded). Add a note that the first registered user becomes superadmin.
- Update any sample `config.yaml` to remove the `admin:` block.

- [ ] **Step 3: Update `admin-api.md`**

In `docs/api-reference/admin-api.md`, replace the "Authentication" section (which currently says "no built-in authentication") with:

```markdown
## Authentication

All Admin API routes are prefixed with `/api/v1/admin` and require a valid JWT
belonging to a user with the `admin` or `superadmin` role. Send the token in the
`Authorization: Bearer <token>` header. Role-management endpoints
(`PATCH /users/:id/role`) require `superadmin`.

The first user to register (local or GitLab) is automatically granted the
`superadmin` role.
```

Also update every endpoint path in that doc to be prefixed with `/api/v1/admin` (e.g. `POST /reload` → `POST /api/v1/admin/reload`, `GET /users` → `GET /api/v1/admin/users`). Add the new `PATCH /api/v1/admin/users/:id/role` endpoint.

- [ ] **Step 4: Update `index.md`**

In `docs/index.md`, under "Related Repo", remove the `CSOJ-WebUI` and `CSOJ-AdminPanel` links (they are now merged in-repo). Keep `CSOJ-cli`.

- [ ] **Step 5: Commit**

```bash
git add docs/
git commit -m "docs: reflect single-port, role-based admin and in-repo frontend"
```

---

## Phase D: Move WebUI into `frontend/`

### Task D1: Copy WebUI into `frontend/`

**Files:**
- Create: `frontend/` (full copy of `CSOJ-WebUI/` minus `.git`, `node_modules`, `out`, `.next`)

- [ ] **Step 1: Copy the WebUI tree**

Run:
```bash
rsync -a --exclude='.git' --exclude='node_modules' --exclude='out' --exclude='.next' \
  ~/develop/CSOJ-WebUI/ ./frontend/
```

- [ ] **Step 2: Verify the structure**

Run: `ls frontend/ && ls frontend/app && ls frontend/components`
Expected: see `package.json`, `next.config.mjs`, `app/(auth)`, `app/(main)`, `components/ui`, `lib/api.ts`, etc.

- [ ] **Step 3: Verify TypeScript baseline compiles**

Run:
```bash
cd frontend && pnpm install && pnpm build
```
Expected: `pnpm build` succeeds (produces `out/`). If there are errors, fix them before proceeding — this is the baseline that must build before we merge admin code.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/
git commit -m "feat: move CSOJ-WebUI into frontend/ (in-repo)"
```

### Task D2: Add AdminPanel-only `package.json` deps

**Files:**
- Modify: `frontend/package.json`

- [ ] **Step 1: Add the missing dependencies**

In `frontend/package.json`, add to `dependencies` (keep existing versions where a dep already exists):

```json
    "@radix-ui/react-checkbox": "^1.3.3",
    "@radix-ui/react-icons": "^1.3.2",
    "@radix-ui/react-select": "^2.2.6",
    "js-yaml": "^4.1.0",
    "react-beautiful-dnd": "^13.1.1",
```

And to `devDependencies`:

```json
    "@types/js-yaml": "^4.0.9",
    "@types/react-beautiful-dnd": "^13.1.8",
```

- [ ] **Step 2: Reinstall and regenerate the lockfile**

Run:
```bash
cd frontend && pnpm install
```
Expected: lockfile updated, install succeeds.

- [ ] **Step 3: Verify build still passes**

Run: `pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/package.json frontend/pnpm-lock.yaml
git commit -m "feat(frontend): add admin panel dependencies"
```

### Task D3: Merge `lib/utils.ts` (add `formatBytes`)

**Files:**
- Modify: `frontend/lib/utils.ts`

- [ ] **Step 1: Add `formatBytes`**

In `frontend/lib/utils.ts`, after the `cn` function (and before `getInitials`), add:

```ts
export function formatBytes(bytes: number, decimals = 2) {
  if (bytes === 0) return '0 Bytes';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}
```

(`getTagColorClasses` already has the "Admin" → amber special case; WebUI's TAG_COLORS list is fine — AdminPanel adds a `yellow` entry but that's cosmetic; keep WebUI's list to avoid changing user-facing tag colors.)

- [ ] **Step 2: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
cd ..
git add frontend/lib/utils.ts
git commit -m "feat(frontend): add formatBytes util"
```

### Task D4: Merge `lib/types.ts` (add admin types + role)

**Files:**
- Modify: `frontend/lib/types.ts`

- [ ] **Step 1: Add `role` to the `User` interface**

In `frontend/lib/types.ts`, in the `User` interface, add after `tags`:

```ts
export interface User {
  id: string;
  username: string;
  nickname: string;
  signature: string;
  avatar_url: string;
  tags: string;
  role: "user" | "admin" | "superadmin";
}
```

- [ ] **Step 2: Add the admin-only types**

Append at the end of `frontend/lib/types.ts`:

```ts
export interface PaginatedResponse<T> {
  items: T[];
  total_items: number;
  total_pages: number;
  current_page: number;
  per_page: number;
}

export interface UserBestScore {
  ID: number;
  UserID: string;
  ContestID: string;
  ProblemID: string;
  Score: number;
  Performance: number;
  SubmissionID: string;
  SubmissionCount: number;
  LastScoreTime: string;
}

export interface AssetFile {
    name: string;
    path: string;
    is_dir: boolean;
    size: number;
    mod_time: string;
}

export interface ConfigNode {
  name: string;
  cpu: number;
  memory: number;
  docker: {
    host: string;
  };
}

export interface NodeState extends ConfigNode {
    used_memory: number;
    is_paused: boolean;
    used_cores: boolean[];
}

export interface ClusterState {
    name: string;
    node: ConfigNode[];
    nodes: Record<string, NodeState>;
}

export interface ClusterStatusResponse {
    resource_status: Record<string, ClusterState>;
    queue_lengths: Record<string, number>;
}

export interface NodeDetail extends NodeState {
    used_cores: boolean[];
}
```

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/lib/types.ts
git commit -m "feat(frontend): add role to User + admin types"
```

### Task D5: Add AdminPanel-only `components/ui` primitives

**Files:**
- Create: `frontend/components/ui/checkbox.tsx`, `select.tsx`, `sheet.tsx`, `textarea.tsx`

(Note: `textarea.tsx` does **not** exist in the WebUI today, but `problem-actions.tsx` imports `../ui/textarea`. Add it here so the admin components compile in Phase E.)

- [ ] **Step 1: Copy the four files from AdminPanel**

Run:
```bash
cp ~/develop/CSOJ-AdminPanel/components/ui/checkbox.tsx frontend/components/ui/checkbox.tsx
cp ~/develop/CSOJ-AdminPanel/components/ui/select.tsx frontend/components/ui/select.tsx
cp ~/develop/CSOJ-AdminPanel/components/ui/sheet.tsx frontend/components/ui/sheet.tsx
cp ~/develop/CSOJ-AdminPanel/components/ui/textarea.tsx frontend/components/ui/textarea.tsx
```

- [ ] **Step 2: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
cd ..
git add frontend/components/ui/checkbox.tsx frontend/components/ui/select.tsx frontend/components/ui/sheet.tsx frontend/components/ui/textarea.tsx
git commit -m "feat(frontend): add checkbox, select, sheet, textarea UI primitives"
```

### Task D6: Add AdminPanel-only `components/shared` helpers

**Files:**
- Create: `frontend/components/shared/pagination-controls.tsx`, `refresh-interval-selector.tsx`, `strict-mode-droppable.tsx`

- [ ] **Step 1: Copy the three files from AdminPanel**

Run:
```bash
cp ~/develop/CSOJ-AdminPanel/components/shared/pagination-controls.tsx frontend/components/shared/pagination-controls.tsx
cp ~/develop/CSOJ-AdminPanel/components/shared/refresh-interval-selector.tsx frontend/components/shared/refresh-interval-selector.tsx
cp ~/develop/CSOJ-AdminPanel/components/shared/strict-mode-droppable.tsx frontend/components/shared/strict-mode-droppable.tsx
```

- [ ] **Step 2: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds (these are not imported by anything yet, so no errors expected).

- [ ] **Step 3: Commit**

```bash
cd ..
git add frontend/components/shared/pagination-controls.tsx frontend/components/shared/refresh-interval-selector.tsx frontend/components/shared/strict-mode-droppable.tsx
git commit -m "feat(frontend): add pagination, refresh-interval, strict-mode-droppable shared components"
```

### Task D7: Merge SWR provider onError toast

**Files:**
- Modify: `frontend/providers/swr-provider.tsx`

- [ ] **Step 1: Add the onError toast**

Replace `frontend/providers/swr-provider.tsx` with:

```tsx
"use client";
import { SWRConfig } from 'swr';
import { useToast } from '@/hooks/use-toast';

export const SWRProvider = ({ children }: { children: React.ReactNode }) => {
    const { toast } = useToast();
    return (
        <SWRConfig
            value={{
                revalidateOnFocus: true,
                errorRetryCount: 3,
                onError: (error: any) => {
                    toast({
                        variant: "destructive",
                        title: "API Error",
                        description: error?.response?.data?.message || error?.message || "An unknown error occurred",
                    });
                },
            }}
        >
            {children}
        </SWRConfig>
    );
};
```

- [ ] **Step 2: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
cd ..
git add frontend/providers/swr-provider.tsx
git commit -m "feat(frontend): surface SWR errors as toasts"
```

---

## Phase E: Merge AdminPanel's `components/admin/`

The 10 admin components use API paths that must be prefixed with `/admin`. This phase copies each file then rewrites the API paths in it. Do each component as its own task so a failure is isolated.

**Path-rewrite rules** (applied uniformly to every admin component and page):

| Old (AdminPanel) | New (merged) |
|---|---|
| `api.get('/reload')` etc. on `/reload`, `/scores/recalculate` | `/admin/reload`, `/admin/scores/recalculate` |
| `/users`, `/users/:id`, `/users/:id/reset-password`, `/register-contest`, `/scores`, `/history`, `/download_solutions` | prefix `/admin` |
| `/submissions`, `/submissions/:id`, `/containers/:conID/log`, `/rejudge`, `/interrupt`, `/validity`, `/content` | prefix `/admin` |
| `/contests`, `/contests/:id`, `/problems/order`, `/announcements`, `/assets` (contest), `/leaderboard`, `/trend` | prefix `/admin` |
| `/problems`, `/problems/:id`, `/assets` (problem) | prefix `/admin` |
| `/clusters/status`, `/clusters/:c/nodes/:n`, `/pause`, `/resume` | prefix `/admin` |
| `/containers`, `/containers/:id` | prefix `/admin` |
| WS `/api/v1/ws/submissions/:id/containers/:conID/logs` | `/api/v1/admin/ws/submissions/:id/containers/:conID/logs` |
| Asset download `/contests/:id/assets/index.assets/:path` and `/problems/:id/assets/index.assets/:path` | prefix `/admin` → `/admin/contests/:id/assets/...` |
| Internal `href` links `/contests?id=`, `/problems?id=`, `/submissions?id=`, `/users?id=` | prefix `/admin` → `/admin/contests?id=` etc. |
| `router.push('/contests')`, `router.push('/problems')` (within admin pages) | `/admin/contests`, `/admin/problems` |

### Task E1: Merge `user-actions.tsx`

**Files:**
- Create: `frontend/components/admin/user-actions.tsx`

- [ ] **Step 1: Copy the file**

Run: `cp ~/develop/CSOJ-AdminPanel/components/admin/user-actions.tsx frontend/components/admin/user-actions.tsx`

- [ ] **Step 2: Rewrite API paths**

In `frontend/components/admin/user-actions.tsx`, prefix every admin API call with `/admin`:

- `api.post('/users', values)` → `api.post('/admin/users', values)`
- `api.patch(`/users/${user.id}`, payload)` → `api.patch(`/admin/users/${user.id}`, payload)`
- `api.post(`/users/${userId}/reset-password`, values)` → `api.post(`/admin/users/${userId}/reset-password`, values)`
- `api.post(`/users/${userId}/register-contest`, values)` → `api.post(`/admin/users/${userId}/register-contest`, values)`
- `api.delete(`/users/${userId}`)` → `api.delete(`/admin/users/${userId}`)`

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/components/admin/user-actions.tsx
git commit -m "feat(frontend): merge admin user-actions component (/admin paths)"
```

### Task E2: Merge `contest-actions.tsx`

**Files:**
- Create: `frontend/components/admin/contest-actions.tsx`

- [ ] **Step 1: Copy the file**

Run: `cp ~/develop/CSOJ-AdminPanel/components/admin/contest-actions.tsx frontend/components/admin/contest-actions.tsx`

- [ ] **Step 2: Rewrite API paths**

In `frontend/components/admin/contest-actions.tsx`:

- `api.put(`/contests/${contest.id}`, payload)` → `api.put(`/admin/contests/${contest.id}`, payload)`
- `api.post('/contests', payload)` → `api.post('/admin/contests', payload)`
- `api.delete(`/contests/${contest.id}`)` → `api.delete(`/admin/contests/${contest.id}`)`

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/components/admin/contest-actions.tsx
git commit -m "feat(frontend): merge admin contest-actions component (/admin paths)"
```

### Task E3: Merge `problem-actions.tsx`

**Files:**
- Create: `frontend/components/admin/problem-actions.tsx`

- [ ] **Step 1: Copy the file**

Run: `cp ~/develop/CSOJ-AdminPanel/components/admin/problem-actions.tsx frontend/components/admin/problem-actions.tsx`

- [ ] **Step 2: Rewrite API paths**

In `frontend/components/admin/problem-actions.tsx`:

- `api.put(`/problems/${problem.id}`, payload)` → `api.put(`/admin/problems/${problem.id}`, payload)`
- `api.post(`/contests/${finalContestId}/problems`, payload)` → `api.post(`/admin/contests/${finalContestId}/problems`, payload)`
- `api.delete(`/problems/${problem.id}`)` → `api.delete(`/admin/problems/${problem.id}`)`

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/components/admin/problem-actions.tsx
git commit -m "feat(frontend): merge admin problem-actions component (/admin paths)"
```

### Task E4: Merge `announcement-actions.tsx`

**Files:**
- Create: `frontend/components/admin/announcement-actions.tsx`

- [ ] **Step 1: Copy the file**

Run: `cp ~/develop/CSOJ-AdminPanel/components/admin/announcement-actions.tsx frontend/components/admin/announcement-actions.tsx`

- [ ] **Step 2: Rewrite API paths**

In `frontend/components/admin/announcement-actions.tsx`:

- `api.put(`/contests/${contestId}/announcements/${announcement.id}`, values)` → `api.put(`/admin/contests/${contestId}/announcements/${announcement.id}`, values)`
- `api.post(`/contests/${contestId}/announcements`, values)` → `api.post(`/admin/contests/${contestId}/announcements`, values)`
- `api.delete(`/contests/${contestId}/announcements/${announcement.id}`)` → `api.delete(`/admin/contests/${contestId}/announcements/${announcement.id}`)`

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/components/admin/announcement-actions.tsx
git commit -m "feat(frontend): merge admin announcement-actions component (/admin paths)"
```

### Task E5: Merge `announcement-manager.tsx`

**Files:**
- Create: `frontend/components/admin/announcement-manager.tsx`

- [ ] **Step 1: Copy the file**

Run: `cp ~/develop/CSOJ-AdminPanel/components/admin/announcement-manager.tsx frontend/components/admin/announcement-manager.tsx`

- [ ] **Step 2: Rewrite the SWR key**

In `frontend/components/admin/announcement-manager.tsx`, the SWR key is (line 17):

```ts
useSWR<Announcement[]>(`/contests/${contestId}/announcements`, fetcher)
```

Change it to:

```ts
useSWR<Announcement[]>(`/admin/contests/${contestId}/announcements`, fetcher)
```

(The `api.delete`/`api.put`/`api.post` calls for announcements live in `announcement-actions.tsx` and were already prefixed in Task E4 — this manager only fetches the list. Verify with `grep -n "api\.\|useSWR" frontend/components/admin/announcement-manager.tsx` that no other contest-anchored path remains unprefixed.)

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/components/admin/announcement-manager.tsx
git commit -m "feat(frontend): merge admin announcement-manager component (/admin paths)"
```

### Task E6: Merge `asset-manager.tsx`

**Files:**
- Create: `frontend/components/admin/asset-manager.tsx`

- [ ] **Step 1: Copy the file**

Run: `cp ~/develop/CSOJ-AdminPanel/components/admin/asset-manager.tsx frontend/components/admin/asset-manager.tsx`

- [ ] **Step 2: Rewrite API paths**

In `frontend/components/admin/asset-manager.tsx`:

- The `apiPrefix` is built from `assetType`. Change:
  ```ts
  const apiPrefix = assetType === 'contest' ? `/contests` : `/problems`;
  ```
  to:
  ```ts
  const apiPrefix = assetType === 'contest' ? `/admin/contests` : `/admin/problems`;
  ```
- The download URL (line ~71):
  ```ts
  const downloadUrl = `/${assetType}s/${assetId}/assets/index.assets/${asset.path}`;
  ```
  change to:
  ```ts
  const downloadUrl = `/admin/${assetType}s/${assetId}/assets/index.assets/${asset.path}`;
  ```

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/components/admin/asset-manager.tsx
git commit -m "feat(frontend): merge admin asset-manager component (/admin paths)"
```

### Task E7: Merge `submission-actions.tsx`

**Files:**
- Create: `frontend/components/admin/submission-actions.tsx`

- [ ] **Step 1: Copy the file**

Run: `cp ~/develop/CSOJ-AdminPanel/components/admin/submission-actions.tsx frontend/components/admin/submission-actions.tsx`

- [ ] **Step 2: Rewrite API paths**

In `frontend/components/admin/submission-actions.tsx`, the patch call (line ~77) is:

```ts
await api.patch(`/submissions/${submission.id}`, payload)
```

Change it to:

```ts
await api.patch(`/admin/submissions/${submission.id}`, payload)
```

(Confirm with `grep -n "api\.\|useSWR" frontend/components/admin/submission-actions.tsx` that no other API path remains — this component only has the one `api.patch`.)

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/components/admin/submission-actions.tsx
git commit -m "feat(frontend): merge admin submission-actions component (/admin paths)"
```

### Task E8: Merge `submission-table-actions.tsx`

**Files:**
- Create: `frontend/components/admin/submission-table-actions.tsx`

- [ ] **Step 1: Copy the file**

Run: `cp ~/develop/CSOJ-AdminPanel/components/admin/submission-table-actions.tsx frontend/components/admin/submission-table-actions.tsx`

- [ ] **Step 2: Rewrite API paths**

Read the file. It builds an `endpoint` from a `submission.id` and an `action` (`rejudge`/`interrupt`/`delete`/`validity`). The endpoint construction (around line 33) is:

```ts
const endpoint = action === 'validity' ? `/submissions/${submission.id}/validity` : `/submissions/${submission.id}/${action}`;
```

Change it to:

```ts
const endpoint = action === 'validity' ? `/admin/submissions/${submission.id}/validity` : `/admin/submissions/${submission.id}/${action}`;
```

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/components/admin/submission-table-actions.tsx
git commit -m "feat(frontend): merge admin submission-table-actions component (/admin paths)"
```

### Task E9: Merge `admin-submission-log-viewer.tsx`

**Files:**
- Create: `frontend/components/admin/admin-submission-log-viewer.tsx`

- [ ] **Step 1: Copy the file**

Run: `cp ~/develop/CSOJ-AdminPanel/components/admin/admin-submission-log-viewer.tsx frontend/components/admin/admin-submission-log-viewer.tsx`

- [ ] **Step 2: Rewrite the WS URL + log-fetch paths**

In `frontend/components/admin/admin-submission-log-viewer.tsx`:

- The `getWsUrl` function (line ~148):
  ```ts
  return `${wsProtocol}//${host}/api/v1/ws/submissions/${submission.id}/containers/${containerId}/logs`;
  ```
  change to:
  ```ts
  return `${wsProtocol}//${host}/api/v1/admin/ws/submissions/${submission.id}/containers/${containerId}/logs`;
  ```
- The `StaticLogViewer` SWR key (line ~18) currently fetches `/submissions/${submissionId}/containers/${containerId}/log`. Change the `textFetcher`'s `useSWR` key to `/admin/submissions/${submissionId}/containers/${containerId}/log`:

  ```ts
  const { data: logText, error, isLoading } = useSWR(`/admin/submissions/${submissionId}/containers/${containerId}/log`, textFetcher);
  ```

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/components/admin/admin-submission-log-viewer.tsx
git commit -m "feat(frontend): merge admin submission-log-viewer (/admin ws + log paths)"
```

### Task E10: Merge `echarts-trend-chart.tsx`

**Files:**
- Create: `frontend/components/admin/echarts-trend-chart.tsx`

- [ ] **Step 1: Copy the file**

Run: `cp ~/develop/CSOJ-AdminPanel/components/admin/echarts-trend-chart.tsx frontend/components/admin/echarts-trend-chart.tsx`

- [ ] **Step 2: Verify it does not fetch**

This component receives its data as props (it imports only `TrendEntry` from `@/lib/types` and uses `echarts-for-react`/`next-themes`/`date-fns` — no `api` or `useSWR`). So **no API path rewrite is needed.** Confirm with:

Run: `grep -n "api\.\|useSWR" frontend/components/admin/echarts-trend-chart.tsx`
Expected: no output.

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/components/admin/echarts-trend-chart.tsx
git commit -m "feat(frontend): merge admin echarts-trend-chart component"
```

---

## Phase F: Admin pages under `app/(main)/admin/`

Each admin page is copied from AdminPanel, then all API paths and internal `href`/`router` paths are prefixed `/admin`, and the default export is wrapped in `withAdmin`.

### Task F0: Create `with-admin.tsx` and `admin-sub-nav.tsx`

**Files:**
- Create: `frontend/components/layout/with-admin.tsx`
- Create: `frontend/components/layout/admin-sub-nav.tsx`

(Do this BEFORE Tasks F1–F6 — the admin pages import `withAdmin` and `AdminSubNav`.)

- [ ] **Step 1: Create `with-admin.tsx`**

Create `frontend/components/layout/with-admin.tsx`:

```tsx
"use client";
import { useAuth } from "@/hooks/use-auth";
import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { Loader2 } from "lucide-react";

const withAdmin = <P extends object>(Component: React.ComponentType<P>) => {
  const AdminComponent = (props: P) => {
    const { user, isAuthenticated, isLoading } = useAuth();
    const router = useRouter();
    const isAdmin = user?.role === "admin" || user?.role === "superadmin";

    useEffect(() => {
      if (!isLoading && (!isAuthenticated || !isAdmin)) {
        router.push("/contests");
      }
    }, [isAuthenticated, isLoading, isAdmin, router]);

    if (isLoading) {
      return (
        <div className="flex h-screen items-center justify-center">
          <Loader2 className="h-12 w-12 animate-spin" />
        </div>
      );
    }

    if (!isAuthenticated || !isAdmin) {
      return null;
    }

    return <Component {...props} />;
  };

  AdminComponent.displayName = `withAdmin(${
    Component.displayName || Component.name || "Component"
  })`;

  return AdminComponent;
};

export default withAdmin;
```

- [ ] **Step 2: Create `admin-sub-nav.tsx`**

Create `frontend/components/layout/admin-sub-nav.tsx` (adapted from AdminPanel's `admin-nav.tsx`, re-pointed to `/admin/*`):

```tsx
"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
    Users,
    Server,
    FileCode,
    Trophy,
    BookCopy,
    Package,
} from "lucide-react"
import { cn } from "@/lib/utils";

const routes = [
    { href: "/admin/cluster", label: "Cluster", icon: Server },
    { href: "/admin/users", label: "Users", icon: Users },
    { href: "/admin/submissions", label: "Submissions", icon: FileCode },
    { href: "/admin/containers", label: "Containers", icon: Package },
    { href: "/admin/contests", label: "Contests", icon: Trophy },
    { href: "/admin/problems", label: "Problems", icon: BookCopy },
];

export function AdminSubNav() {
    const pathname = usePathname();

    return (
        <nav className="flex items-center gap-1 overflow-x-auto border-b pb-2 mb-6">
            {routes.map(route => {
                const Icon = route.icon;
                return (
                    <Link
                        key={route.href}
                        href={route.href}
                        className={cn(
                            "flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium text-muted-foreground transition-all hover:text-primary whitespace-nowrap",
                            pathname.startsWith(route.href) && "bg-muted text-primary"
                        )}
                    >
                        <Icon className="h-4 w-4" />
                        {route.label}
                    </Link>
                )
            })}
        </nav>
    );
}
```

- [ ] **Step 3: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/components/layout/with-admin.tsx frontend/components/layout/admin-sub-nav.tsx
git commit -m "feat(frontend): add withAdmin HOC and AdminSubNav"
```

### Task F1: Merge the cluster admin page

**Files:**
- Create: `frontend/app/(main)/admin/cluster/page.tsx`

- [ ] **Step 1: Copy the file**

Run:
```bash
mkdir -p "frontend/app/(main)/admin/cluster"
cp ~/develop/CSOJ-AdminPanel/app/'(main)'/cluster/page.tsx "frontend/app/(main)/admin/cluster/page.tsx"
```

- [ ] **Step 2: Rewrite API + SWR paths**

In the new file:

- `useSWR<ClusterStatusResponse>('/clusters/status', ...)` → `useSWR<ClusterStatusResponse>('/admin/clusters/status', ...)`
- `useSWR<NodeDetail>(`/clusters/${clusterName}/nodes/${nodeName}`, ...)` → `useSWR<NodeDetail>(`/admin/clusters/${clusterName}/nodes/${nodeName}`, ...)`
- `api.post(`/clusters/${clusterName}/nodes/${nodeName}/${action}`)` → `api.post(`/admin/clusters/${clusterName}/nodes/${nodeName}/${action}`)`

- [ ] **Step 3: Wrap in `withAdmin` + add `AdminSubNav`**

Add imports:
```tsx
import withAdmin from "@/components/layout/with-admin";
import { AdminSubNav } from "@/components/layout/admin-sub-nav";
```
Change `export default function ClusterStatusPage()` → `function ClusterStatusPage()`, add `export default withAdmin(ClusterStatusPage);` at the end. Render `<AdminSubNav />` at the top of the returned JSX (inside the outermost wrapper).

- [ ] **Step 4: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 5: Commit**

```bash
cd ..
git add "frontend/app/(main)/admin/cluster/page.tsx"
git commit -m "feat(frontend): admin cluster page under /admin/cluster"
```

### Task F2: Merge the users admin page

**Files:**
- Create: `frontend/app/(main)/admin/users/page.tsx`

- [ ] **Step 1: Copy the file**

Run:
```bash
mkdir -p "frontend/app/(main)/admin/users"
cp ~/develop/CSOJ-AdminPanel/app/'(main)'/users/page.tsx "frontend/app/(main)/admin/users/page.tsx"
```

- [ ] **Step 2: Rewrite API + SWR + href paths**

In the new file, prefix `/admin` to every admin API/SWR path and internal `href`:

- `useSWR<User[]>(`/users?query=${debouncedSearchQuery}`, ...)` → `useSWR<User[]>(`/admin/users?query=${debouncedSearchQuery}`, ...)`
- `useSWR<Record<string, Contest>>('/contests', ...)` → `useSWR<Record<string, Contest>>('/admin/contests', ...)`
- `useSWR<ScoreHistoryPoint[]>(... \`/users/${userId}/history?contest_id=${selectedContestId}\` ...)` → `\`/admin/users/${userId}/history?contest_id=${selectedContestId}\``
- `useSWR<User>(`/users/${userId}`, ...)` → `useSWR<User>(`/admin/users/${userId}`, ...)`
- `useSWR<UserBestScore[]>(`/users/${userId}/scores`, ...)` → `useSWR<UserBestScore[]>(`/admin/users/${userId}/scores`, ...)`
- All `href={`/users?id=${...}`}` → `href={`/admin/users?id=${...}`}`
- `href={`/problems?id=${h.problem_id}`}` → `href={`/admin/problems?id=${h.problem_id}`}`
- `href={`/contests?id=${s.ContestID}`}` → `href={`/admin/contests?id=${s.ContestID}`}`
- `href={`/submissions?id=${s.SubmissionID}`}` → `href={`/admin/submissions?id=${s.SubmissionID}`}`

- [ ] **Step 3: Wrap default export in `withAdmin`**

Add `import withAdmin from "@/components/layout/with-admin";`. Change `export default function UsersPage()` to `function UsersPage()` and add `export default withAdmin(UsersPage);` at the end.

- [ ] **Step 4: Add `AdminSubNav` to the page**

Inside the `UsersPage` component's returned JSX, add `<AdminSubNav />` at the top of the content area. The `UsersPage` default export wraps `<Suspense><UsersPageContent /></Suspense>`; place `<AdminSubNav />` inside `UsersPageContent`'s return, as the first element above the `UserList`/`UserDetails` conditional. Import it: `import { AdminSubNav } from "@/components/layout/admin-sub-nav";`.

(Most admin pages render a `<div className="space-y-6">` or `<Card>` wrapper — put `<AdminSubNav />` as the first child of that wrapper so the nav bar sits above the page content.)

- [ ] **Step 5: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 6: Commit**

```bash
cd ..
git add "frontend/app/(main)/admin/users/page.tsx"
git commit -m "feat(frontend): admin users page under /admin/users"
```

### Task F3: Merge the submissions admin page

**Files:**
- Create: `frontend/app/(main)/admin/submissions/page.tsx`

- [ ] **Step 1: Copy the file**

Run:
```bash
mkdir -p "frontend/app/(main)/admin/submissions"
cp ~/develop/CSOJ-AdminPanel/app/'(main)'/submissions/page.tsx "frontend/app/(main)/admin/submissions/page.tsx"
```

- [ ] **Step 2: Rewrite API + SWR + href paths**

In the new file:

- `useSWR<PaginatedResponse<Submission>>(`/submissions?${query}`, ...)` → `\`/admin/submissions?${query}\``
- `useSWR<Submission>(`/submissions/${submissionId}`, ...)` → `\`/admin/submissions/${submissionId}\``
- `useSWR<Problem>(submission ? `/problems/${submission.problem_id}` : null, ...)` → `\`/admin/problems/${submission.problem_id}\``
- The `endpoint` in `handleAction`: `/submissions/${submissionId}/...` → `/admin/submissions/${submissionId}/...`
- All `href={`/submissions?id=${s.id}`}` → `href={`/admin/submissions?id=${s.id}`}`
- All `href={`/problems?id=${s.problem_id}`}` → `href={`/admin/problems?id=${s.problem_id}`}`
- All `href={`/users?id=${s.user_id}`}` → `href={`/admin/users?id=${s.user_id}`}`
- Any container log fetch `/submissions/:id/containers/:conID/log` → `/admin/submissions/:id/containers/:conID/log`

- [ ] **Step 3: Wrap in `withAdmin` + add `AdminSubNav`**

Add imports:
```tsx
import withAdmin from "@/components/layout/with-admin";
import { AdminSubNav } from "@/components/layout/admin-sub-nav";
```
Rename `export default function SubmissionsPage()` → `function SubmissionsPage()`, add `export default withAdmin(SubmissionsPage);`. Render `<AdminSubNav />` at the top of the page content.

- [ ] **Step 4: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 5: Commit**

```bash
cd ..
git add "frontend/app/(main)/admin/submissions/page.tsx"
git commit -m "feat(frontend): admin submissions page under /admin/submissions"
```

### Task F4: Merge the containers admin page

**Files:**
- Create: `frontend/app/(main)/admin/containers/page.tsx`

- [ ] **Step 1: Copy the file**

Run:
```bash
mkdir -p "frontend/app/(main)/admin/containers"
cp ~/develop/CSOJ-AdminPanel/app/'(main)'/containers/page.tsx "frontend/app/(main)/admin/containers/page.tsx"
```

- [ ] **Step 2: Rewrite API + SWR + href paths**

In the new file:

- `useSWR<PaginatedResponse<Container>>(`/containers?${query}`, ...)` → `\`/admin/containers?${query}\``
- `useSWR<Container>(`/containers/${containerId}`, ...)` → `\`/admin/containers/${containerId}\``
- `href={`/submissions?id=${container.submission_id}`}` → `href={`/admin/submissions?id=${container.submission_id}`}`
- `href={`/users?id=${c.user.id}`}` → `href={`/admin/users?id=${c.user.id}`}`

- [ ] **Step 3: Wrap in `withAdmin` + add `AdminSubNav`**

(Same pattern as Task F3.)

- [ ] **Step 4: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 5: Commit**

```bash
cd ..
git add "frontend/app/(main)/admin/containers/page.tsx"
git commit -m "feat(frontend): admin containers page under /admin/containers"
```

### Task F5: Merge the contests admin page

**Files:**
- Create: `frontend/app/(main)/admin/contests/page.tsx`

- [ ] **Step 1: Copy the file**

Run:
```bash
mkdir -p "frontend/app/(main)/admin/contests"
cp ~/develop/CSOJ-AdminPanel/app/'(main)'/contests/page.tsx "frontend/app/(main)/admin/contests/page.tsx"
```

- [ ] **Step 2: Rewrite API + SWR + href + router paths**

In the new file (this is the largest admin page; grep thoroughly):

- `useSWR('/contests', ...)` → `useSWR('/admin/contests', ...)`
- `useSWR('/problems', ...)` → `useSWR('/admin/problems', ...)`
- `useSWR(`/contests/${contestId}`, ...)` → `\`/admin/contests/${contestId}\``
- `useSWR(`/contests/${contestId}/leaderboard...`, ...)` → `\`/admin/contests/${contestId}/leaderboard...\``
- `useSWR(`/contests/${contestId}/announcements`, ...)` → `\`/admin/contests/${contestId}/announcements\``
- `api.get(`/users/${userId}/download_solutions/${contestId}`, ...)` → `api.get(`/admin/users/${userId}/download_solutions/${contestId}`, ...)`
- `api.put(`/contests/${contest.id}/problems/order`, ...)` → `api.put(`/admin/contests/${contest.id}/problems/order`, ...)`
- `router.push('/contests')` → `router.push('/admin/contests')`
- All `href={`/contests?id=...`}` → `href={`/admin/contests?id=...`}`
- All `href={`/problems?id=...`}` → `href={`/admin/problems?id=...`}`
- All `href={`/submissions?id=...`}` → `href={`/admin/submissions?id=...`}`
- All `href={`/users?id=...`}` → `href={`/admin/users?id=...`}`

- [ ] **Step 3: Wrap in `withAdmin` + add `AdminSubNav`**

(Same pattern.)

- [ ] **Step 4: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 5: Commit**

```bash
cd ..
git add "frontend/app/(main)/admin/contests/page.tsx"
git commit -m "feat(frontend): admin contests page under /admin/contests"
```

### Task F6: Merge the problems admin page

**Files:**
- Create: `frontend/app/(main)/admin/problems/page.tsx`

- [ ] **Step 1: Copy the file**

Run:
```bash
mkdir -p "frontend/app/(main)/admin/problems"
cp ~/develop/CSOJ-AdminPanel/app/'(main)'/problems/page.tsx "frontend/app/(main)/admin/problems/page.tsx"
```

- [ ] **Step 2: Rewrite API + SWR + href + router paths**

In the new file:

- `useSWR('/problems', ...)` → `useSWR('/admin/problems', ...)`
- `useSWR(`/problems/${problemId}`, ...)` → `\`/admin/problems/${problemId}\``
- `router.push('/problems')` → `router.push('/admin/problems')`
- All `href={`/problems?id=...`}` → `href={`/admin/problems?id=...`}`
- Any `href={`/contests?id=...`}` → `href={`/admin/contests?id=...`}`

- [ ] **Step 3: Wrap in `withAdmin` + add `AdminSubNav`**

(Same pattern.)

- [ ] **Step 4: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 5: Commit**

```bash
cd ..
git add "frontend/app/(main)/admin/problems/page.tsx"
git commit -m "feat(frontend): admin problems page under /admin/problems"
```

---

## Phase G: Wire admin into the WebUI top nav

### Task G1: Add the "Admin" link to `MainNav`

**Files:**
- Modify: `frontend/components/layout/main-nav.tsx`

- [ ] **Step 1: Add the admin link conditionally**

In `frontend/components/layout/main-nav.tsx`, add the auth import and a conditional admin route. Add at the top with the other imports:

```tsx
import { useAuth } from "@/hooks/use-auth";
```

Inside the `MainNav` component, after the `const { data: dynamicLinks } = useSWR(...)` line, add:

```tsx
  const { user } = useAuth();
  const isAdmin = user?.role === "admin" || user?.role === "superadmin";
```

Then in the `allRoutes` array, add the admin link conditionally. Change:

```tsx
  const allRoutes = [
    { href: "/contests", label: t("contests") },
    { href: "/submissions", label: t("submissions") },
    { href: "/profile", label: t("profile") },
    ...(dynamicLinks?.map(link => ({ href: link.url, label: link.name })) || []),
  ];
```

to:

```tsx
  const allRoutes = [
    { href: "/contests", label: t("contests") },
    { href: "/submissions", label: t("submissions") },
    { href: "/profile", label: t("profile") },
    ...(isAdmin ? [{ href: "/admin/contests", label: "Admin" }] : []),
    ...(dynamicLinks?.map(link => ({ href: link.url, label: link.name })) || []),
  ];
```

- [ ] **Step 2: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
cd ..
git add frontend/components/layout/main-nav.tsx
git commit -m "feat(frontend): show Admin nav link for admin/superadmin users"
```

### Task G2: Add a role-management UI on the admin users page

**Files:**
- Modify: `frontend/app/(main)/admin/users/page.tsx`

- [ ] **Step 1: Add a "Set Role" menu item for superadmins**

In the admin users page, the `UserList` component renders a `DropdownMenu` per row. Add a role toggle visible only to superadmins. Add to the imports at the top of `frontend/app/(main)/admin/users/page.tsx`:

```tsx
import { useAuth } from "@/hooks/use-auth";
```

(Also ensure `api` is imported: `import api from '@/lib/api';` — it already is, since the page defines a `fetcher` using `api`.)

In the `UserList` component, after the `useSWR` call, get the current user's role:

```tsx
    const { user: currentUser } = useAuth();
    const isSuperAdmin = currentUser?.role === "superadmin";

    const handleRoleToggle = async (u: User) => {
        const newRole = u.role === "admin" ? "user" : "admin";
        try {
            await api.patch(`/admin/users/${u.id}/role`, { role: newRole });
            mutate();
        } catch (err: any) {
            // SWR onError toast surfaces the error
        }
    };
```

Then in the `DropdownMenuContent` (the per-row menu), add this item before `<DropdownMenuSeparator />`:

```tsx
                                                {isSuperAdmin && (
                                                    <DropdownMenuItem
                                                        onSelect={(e) => {
                                                            e.preventDefault();
                                                            handleRoleToggle(user);
                                                        }}
                                                    >
                                                        {user.role === "admin"
                                                            ? "Demote to User"
                                                            : "Promote to Admin"}
                                                    </DropdownMenuItem>
                                                )}
```

(`user` here is the row's user — the `.map(user => ...)` variable in `UserList`; rename the destructured `useAuth` value to `currentUser` as shown above so the two don't collide.)

- [ ] **Step 2: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
cd ..
git add "frontend/app/(main)/admin/users/page.tsx"
git commit -m "feat(frontend): superadmin can promote/demote admins from users page"
```

---

## Phase H: End-to-end verification

### Task H1: Build everything end-to-end

**Files:** (no file changes — verification only)

- [ ] **Step 1: Build the frontend**

Run: `cd frontend && pnpm install && pnpm build`
Expected: `out/` produced, no errors.

- [ ] **Step 2: Build the Go binary with embedded frontend**

Run: `cd .. && make build`
Expected: `CSOJ` binary produced.

- [ ] **Step 3: Commit nothing (verification only)**

No commit.

### Task H2: Manual smoke test (single port, role bootstrap)

**Files:** (no file changes — verification only)

- [ ] **Step 1: Start the server**

Run (in a separate shell or background):
```bash
./CSOJ -c configs/config.yaml
```
(Ensure `configs/config.yaml` exists — if not, create a minimal one based on `docs/getting-started.md` with `listen: ":8080"`, a `storage.database` path, an `auth.jwt.secret`, and `auth.local.enabled: true`. No `admin:` block.)

Expected: server logs `starting server at :8080`, no admin-port log.

- [ ] **Step 2: Register the first user; verify superadmin**

Run:
```bash
curl -s localhost:8080/api/v1/auth/local/register -H 'Content-Type: application/json' \
  -d '{"username":"root","password":"rootpass","nickname":"Root"}'
```
Then login:
```bash
TOKEN=$(curl -s localhost:8080/api/v1/auth/local/login -H 'Content-Type: application/json' \
  -d '{"username":"root","password":"rootpass"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])")
curl -s localhost:8080/api/v1/user/profile -H "Authorization: Bearer $TOKEN"
```
Expected: profile JSON has `"role":"superadmin"`.

- [ ] **Step 3: Verify admin route gating**

Run (no token):
```bash
curl -s -o /dev/null -w "%{http_code}\n" localhost:8080/api/v1/admin/users
```
Expected: `401`.

Run (with superadmin token):
```bash
curl -s -o /dev/null -w "%{http_code}\n" localhost:8080/api/v1/admin/users -H "Authorization: Bearer $TOKEN"
```
Expected: `200`.

- [ ] **Step 4: Verify old admin port is gone**

Run: `curl -s -o /dev/null -w "%{http_code}\n" localhost:8081/api/v1/users`
Expected: connection refused / `000`.

- [ ] **Step 5: No commit**

This is a verification gate. If any step fails, debug with the superpowers:systematic-debugging skill before proceeding.

### Task H3: Run the `verify` skill

- [ ] **Step 1: Invoke the verify skill**

Use the `verify` skill to drive the affected flows end-to-end: login as the first user (becomes superadmin), confirm the "Admin" nav link appears, click into `/admin/contests`, and confirm an admin API call succeeds. Capture the result.

- [ ] **Step 2: Commit any fixes the verification surfaced**

If verification found bugs, fix them and commit. If clean, no commit.

---

## Notes for the implementer

- **Toolchain on this machine:** Go 1.24.4, pnpm 10.33.3, Node 20.19.2 — all present.
- **Order matters:** Phase A → B → C for the backend; Phase D → E → F → G for the frontend. Task F0 must precede F1–F6.
- **`sites/` is gitignored** — the `internal/embedui/sites/user/.index.html` placeholder is the only tracked file there; the Makefile overwrites `sites/user/` at build time. Don't commit built frontend output.
- **The admin pages are English-hardcoded** (matching the AdminPanel repo). Do not introduce i18n keys in this pass.
- **`react-beautiful-dnd` is deprecated** but kept here to stay mechanical; replacing it is out of scope.
- **Frontend dev workflow** is unchanged: `cd frontend && pnpm dev` (port 3000) with the API at `:8080`. The `next.config.mjs` rewrites are commented out — same as today.
