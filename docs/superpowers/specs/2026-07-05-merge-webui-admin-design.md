# Merge WebUI + AdminPanel into CSOJ — Design

**Date:** 2026-07-05
**Scope:** Consolidate the three CSOJ repositories (`CSOJ` backend, `CSOJ-WebUI`, `CSOJ-AdminPanel`) into a single repository. The frontend lives under `frontend/` and is built in-repo (no release-tarball downloads). Admin pages merge into the WebUI route tree behind a role-based gate. The two backend ports merge into one, with admin routes prefixed `/api/v1/admin/*` and guarded by a new role-based middleware.

## Goals

1. **One repo, one frontend build.** `CSOJ-WebUI` moves into `CSOJ/frontend/`. `CSOJ-AdminPanel`'s admin-only code is merged in. Build happens via pnpm inside this repo; the Go binary embeds the built `out/`.
2. **Admin panel as a top-nav menu item.** Visible and reachable only by users with `admin`/`superadmin` role. Admin pages are flattened into the WebUI's existing top-nav shell (they share the WebUI header/footer, not a separate sidebar shell).
3. **One API port.** The separate admin Gin engine on `cfg.Admin.Listen` is removed. A single engine serves both user routes (`/api/v1/*`) and admin routes (`/api/v1/admin/*`). Admin routes are JWT-authenticated and role-checked.
4. **Role-based admin permission.** A `Role` field is added to `User`, a `role` claim is added to the JWT. The first registered user becomes `superadmin`; superadmins can promote other users to `admin` (and demote back). `superadmin` is not delegable through the API.

## Non-goals

- Migrating the admin panel's hardcoded English strings to i18n (kept English for now, matching the AdminPanel repo).
- Changing the user-facing pages' behavior or styling.
- Archiving or deleting the `CSOJ-WebUI` / `CSOJ-AdminPanel` GitHub repos (left untouched per user decision; their local working copies are not deleted).
- Introducing a `superadmin`-promotion endpoint (superadmin is bootstrap-only).

---

## Architecture

### Current state

- `cmd/CSOJ/main.go` starts two `*gin.Engine`s: `user.NewUserRouter` (on `cfg.Listen`, JWT auth, serves embedded WebUI) and `admin.NewAdminRouter` (on `cfg.Admin.Listen`, **no auth**, serves embedded AdminPanel).
- Both routers register overlapping paths (`GET /contests/:id`, `/problems/:id`, `/contests/:id/leaderboard`, …) under `/api/v1` — feasible today only because they are separate engines on separate ports.
- `internal/embedui/ui.go` embeds `sites/user` and `sites/admin` separately via `//go:embed all:sites`; `RegisterUIHandlers(r, sitename)` mounts an SPA-fallback file server that passes through `/api/` and `/ws/`.
- `User` model (`internal/database/models/models.go`) has no role field. JWT (`internal/auth/jwt.go`) carries only standard `RegisteredClaims` (`sub` = user ID). `AuthMiddleware` (`internal/api/middleware.go`) validates the JWT and looks up the user; no role check.
- The two frontends are Next.js 14 App Router, `output: 'export'`, pnpm, shadcn/ui, Tailwind, axios with `baseURL: '/api/v1'`. WebUI has JWT auth (`localStorage` `csoj_jwt`, `AuthProvider`, `withAuth` HOC, next-intl en/zh). AdminPanel has **no auth**. Their `components/ui`, `globals.css`, `tailwind.config.ts`, `lib/utils.ts`, `use-toast.ts` are near-identical.

### Target state

- One `*gin.Engine` in `main.go`. `user.RegisterRoutes(r, ...)` mounts `/api/v1/*`; `admin.RegisterRoutes(r, ...)` mounts `/api/v1/admin/*` with group-level `AdminMiddleware`.
- One embedded UI site: `internal/embedui/sites/user/`. The Makefile copies `frontend/out/` into it before `go build`. `sites/admin/` is removed.
- `User.Role` field + JWT `role` claim + `AdminMiddleware`. First registered user → `superadmin`. `PATCH /api/v1/admin/users/:id/role` (superadmin-only) sets `admin`/`user`.
- Frontend under `frontend/`: WebUI tree + `app/(main)/admin/{cluster,users,submissions,containers,contests,problems}/page.tsx`. `MainNav` shows an "Admin" link only when `user.role ∈ {admin, superadmin}`. Admin pages render inside the WebUI `MainLayout` (shared header/footer); the AdminPanel's sidebar/header shell is **not** used.

---

## Backend changes

### Data model

`internal/database/models/models.go`:

```go
type Role string

const (
    RoleUser       Role = "user"
    RoleAdmin      Role = "admin"
    RoleSuperAdmin Role = "superadmin"
)

type User struct {
    // ...existing fields...
    Role Role `gorm:"type:text;default:'user';index" json:"role"`
}
```

`database.Init` already calls `AutoMigrate(&models.User{}, ...)` — GORM adds the `role` column with default `'user'`. Existing rows get `'user'`.

### JWT

`internal/auth/jwt.go`:

```go
type MyCustomClaims struct {
    Role string `json:"role,omitempty"`
    jwt.RegisteredClaims
}
```

`GenerateJWT(userID, role, secret string, expireHours int)` adds the `role` claim. `ValidateJWT` returns the claims (already does). Callers updated:
- `user/auth.go localLogin` → `GenerateJWT(user.ID, string(user.Role), ...)`
- `auth/gitlab.go Callback` → `GenerateJWT(user.ID, string(user.Role), ...)`

### Middleware

`internal/api/middleware.go`:

- `AuthMiddleware` already calls `auth.ValidateJWT`. After fetching the user, set `c.Set("userID", claims.Subject)` and additionally `c.Set("role", string(user.Role))`. (Use the DB-loaded role as source of truth, not the JWT claim, so a demoted admin loses access immediately even with a still-valid token.)
- New `AdminMiddleware(secret, db)`:
  ```go
  func AdminMiddleware(secret string, db *gorm.DB) gin.HandlerFunc {
      return func(c *gin.Context) {
          // reuse AuthMiddleware logic, then require role
      }
  }
  ```
  Implementation: call the same JWT+user lookup, then `if role != admin && role != superadmin { 403 }`. Store `userID` and `role` on the context.
- New `SuperAdminMiddleware`: `if role != superadmin { 403 }`. Used only for the role-management endpoint. Can be expressed as `AdminMiddleware` + a check, but a distinct middleware keeps the route file readable.

### Bootstrap: first user → superadmin

Add a helper in `internal/database/user.go`:

```go
func CountUsers(db *gorm.DB) (int64, error) {
    var count int64
    err := db.Model(&models.User{}).Count(&count).Error
    return count, err
}
```

In both user-creation paths, **before** creating, count users in a transaction; if `count == 0`, set `Role = RoleSuperAdmin`:

- `user/auth.go localRegister` — wrap the existing `CreateUser` call: if `CountUsers == 0`, `newUser.Role = models.RoleSuperAdmin` and log it.
- `auth/gitlab.go Callback` — same, before `database.CreateUser(h.db, &newUser)`.

Known race: two concurrent first-registrations could both see `count == 0`. Acceptable for bootstrap (first signup is a one-time event on a fresh DB). Logged.

### Role management endpoint

`internal/api/admin/user.go`:

```go
func (h *Handler) updateUserRole(c *gin.Context) {
    // superadmin-only (route uses SuperAdminMiddleware)
    targetID := c.Param("id")
    var req struct { Role string `json:"role" binding:"required"` }
    // validate req.Role ∈ {admin, user}
    // superadmin cannot demote self: if targetID == c.GetString("userID") && req.Role == user → 400
    // load target, set Role, UpdateUser
}
```

No endpoint to grant `superadmin`. Superadmin-to-superadmin is not supported; superadmin stays bootstrap-only.

### Router refactor

`internal/api/user/router.go`: change `NewUserRouter(...) *gin.Engine` → `RegisterRoutes(r *gin.Engine, ...)`. Remove `r := gin.Default()`, `r.Use(api.CORSMiddleware(...))`, and the `embedui.RegisterUIHandlers(r, "user")` call (the engine is created in `main.go` and the UI is registered once there). Keep all `/api/v1/*` groups unchanged.

`internal/api/admin/router.go`: change `NewAdminRouter(...) *gin.Engine` → `RegisterRoutes(r *gin.Engine, ...)`. Remove engine creation, CORS, and `embedui.RegisterUIHandlers`. Mount under `/api/v1/admin`:

```go
adminV1 := r.Group("/api/v1/admin")
adminV1.Use(api.AdminMiddleware(cfg.Auth.JWT.Secret, db))
{
    // WebSocket
    adminV1.GET("/ws/submissions/:id/containers/:conID/logs", ...)
    // Management — note: /reload is now /api/v1/admin/reload
    adminV1.POST("/reload", h.reload)
    // Users
    users := adminV1.Group("/users") { ... }     // includes new PATCH /:id/role
    // Submissions, Contests, Problems, Scores, Clusters, Containers — same shape, nested under /admin
}
```

Every path that was `/api/v1/users` is now `/api/v1/admin/users`, etc. The `scores/recalculate` and `reload` endpoints move under `/admin`.

### main.go

```go
r := gin.Default()
r.Use(api.CORSMiddleware(cfg.CORS))
user.RegisterRoutes(r, cfg, db, scheduler, appState)
admin.RegisterRoutes(r, cfg, db, scheduler, appState)
embedui.RegisterUIHandlers(r, "user")
go func() {
    if err := r.Run(cfg.Listen); err != nil { zap.S().Fatalf(...) }
}()
```

Remove the second `adminEngine.Run(cfg.Admin.Listen)` goroutine and the `if cfg.Admin.Enabled` block.

### Config

`internal/config/config.go`: remove `Admin.Enabled` / `Admin.Listen` (and the `Admin` struct, or leave it empty but unused — prefer removing). Update `docs/configuration/main-config.md` and the sample config in `docs/getting-started.md` to drop the `admin:` block.

### Profile responses

`user/profile.go getUserProfile` returns the full `user` (already includes `Role` via the JSON tag). `getPublicUserProfile`'s `PublicProfileResponse` — **do not** expose `role` publicly; keep it limited to id/username/nickname/signature/avatar_url/tags. (Admin sees role via `GET /api/v1/admin/users/:id`.) The WebUI's own `User` type will carry `role` from `/user/profile`.

---

## Frontend changes

### Layout / move

1. `rsync -a CSOJ-WebUI/ CSOJ/frontend/` (excluding `.git`, `node_modules`, `out`). Keep WebUI's `package.json`, `next.config.mjs` (with next-intl plugin), `tailwind.config.ts`, `tsconfig.json`, `app/`, `components/`, `lib/`, `providers/`, `hooks/`, `i18n/`, `public/`.
2. Merge AdminPanel-only pieces into `frontend/`:
   - `components/admin/` → `frontend/components/admin/` (all 10 files: `admin-submission-log-viewer`, `announcement-*`, `asset-manager`, `contest-actions`, `echarts-trend-chart`, `problem-actions`, `submission-*`, `user-actions`).
   - `components/ui/{checkbox,select,sheet}.tsx` → `frontend/components/ui/` (AdminPanel-only primitives).
   - `components/shared/{pagination-controls,refresh-interval-selector,strict-mode-droppable}.tsx` → `frontend/components/shared/` (merge; keep WebUI's `markdown-viewer`, `submission-status-badge`, `user-profile-card`).
   - `lib/utils.ts` — union: keep `cn`, `getScoreColor` (WebUI), add `formatBytes` (AdminPanel), reconcile `getTagColorClasses` (both have "Admin" → amber special case; identical).
   - `app/(main)/admin/{cluster,users,submissions,containers,contests,problems}/page.tsx` — port from AdminPanel's `app/(main)/<route>/page.tsx`.
3. Regenerate `pnpm-lock.yaml` (`pnpm install`). `package.json` deps: union (WebUI is the base; add `@radix-ui/react-checkbox`, `@radix-ui/react-select`, `react-beautiful-dnd`, `@types/react-beautiful-dnd`, `@radix-ui/react-icons`). Pin versions to WebUI's where both have a dep.

### Route-tree collision

AdminPanel's pages are `/cluster`, `/users`, `/submissions`, `/containers`, `/contests`, `/problems`. WebUI already owns `/contests`, `/problems`, `/submissions`, `/profile`. **All admin pages live under `/admin/*`** to avoid collision:

```
app/(main)/admin/
  cluster/page.tsx
  users/page.tsx
  submissions/page.tsx
  containers/page.tsx
  contests/page.tsx
  problems/page.tsx
```

So the admin "Contests" page is `/admin/contests`, distinct from the user `/contests`. Each admin page is `use client` and gated by `withAdmin` (below). Detail views continue to use `?id=` / `?view=` query params (as AdminPanel does today).

### API client

`frontend/lib/api.ts`: keep the single axios instance (`baseURL: '/api/v1'`, Bearer interceptor, banned-response interceptor) from WebUI. **All admin calls get the `/admin` prefix at the call site** (e.g. `api.get('/admin/users')`, `api.post('/admin/reload')`). This is explicit and greppable. Grep the ported AdminPanel components/pages for `api.get/post/put/patch/delete('...')` and prepend `/admin` to each path, and update the WebSocket URL in `admin-submission-log-viewer.tsx` from `/api/v1/ws/...` to `/api/v1/admin/ws/...`.

### Types

`frontend/lib/types.ts`: add `role: "user" | "admin" | "superadmin"` to `User`. Merge AdminPanel's admin-specific types (it reuses the same `User`/`Contest`/`Problem`/`Submission` shapes — verify and reconcile any drift).

### Auth / admin gating

- `providers/auth-provider.tsx`: `user` now carries `role` (from `/user/profile`). No structural change; `AuthState.user` already typed as `User`.
- New `hooks/use-auth.ts` exports `useAuth()` (already exists). Add a derived `isAdmin` check where needed.
- New `components/layout/with-admin.tsx`: an HOC mirroring `with-auth.tsx` that additionally redirects to `/contests` (or shows a 403 page) when `user.role` is not `admin`/`superadmin`. Wrap each admin page's default export. (Admin pages are already behind `withAuth` via the `(main)` layout, so unauthenticated users hit `/login` first; `withAdmin` adds the role check on top.)
- `components/layout/main-nav.tsx`: add an "Admin" link to `allRoutes` when `user.role ∈ {admin, superadmin}` — `const { user } = useAuth(); ... user?.role === 'admin' || user?.role === 'superadmin'`. Link to `/admin/contests` (the most useful landing page).

### Admin shell

Per the user's "flatten into the user top nav" decision: **do not** use AdminPanel's `AdminSidebar`/`AdminHeader`. Admin pages render inside the WebUI `MainLayout` (the `(main)/layout.tsx` header with `MainNav`/`UserNav`/theme/lang toggles). The admin section's own sub-navigation (the 6 admin pages) becomes either:
- A secondary nav bar rendered at the top of each admin page (a small `AdminSubNav` component listing Cluster/Users/Submissions/Containers/Contests/Problems), **or**
- A dropdown from the "Admin" top-nav item.

**Decision: secondary nav bar.** A lightweight `components/layout/admin-sub-nav.tsx` (adapted from AdminPanel's `admin-nav.tsx`, re-pointed to `/admin/*` paths) rendered at the top of every admin page. The AdminPanel `admin-header.tsx`'s "Reload Config" and "Recalculate Score" buttons move into this sub-nav (or a small toolbar on the admin pages that need them). Keep these as English-hardcoded for now.

### i18n

WebUI uses `next-intl` with `public/messages/{en,zh}.json`. AdminPanel is English-only. **Admin pages keep English-hardcoded strings** in this pass (matching the AdminPanel repo). A follow-up can migrate them to i18n keys. WebUI's existing strings are untouched.

### `components/ui` dedup

The shared shadcn primitives (`button`, `dialog`, `table`, `tabs`, `toast`, etc.) are near-identical. Diff each; keep the richer version. WebUI's `collapsible`, `hover-card`, `progress`, `shadcn-io` are WebUI-only (keep). AdminPanel's `checkbox`, `select`, `sheet` are AdminPanel-only (add). `globals.css` and `tailwind.config.ts` are essentially identical (both derived from shadcn new-york) — merge the AdminPanel tag-color safelist into WebUI's (likely already present).

### `next.config.mjs`

Keep WebUI's `output: 'export'` + next-intl plugin. The commented-out `rewrites()` stays commented (the Go backend serves everything same-origin in production; for local dev, the user runs `pnpm dev` on `:3000` and points the API at `:8080` — existing workflow, no change needed for this merge).

---

## Build

New `Makefile` at repo root:

```makefile
.PHONY: build frontend backend clean dev-frontend

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
```

`.gitignore` already ignores `internal/embedui/sites/` — leave it (the built UI is not committed; the Makefile materializes it). Add `frontend/node_modules/` and `frontend/out/` to `.gitignore` if not already covered (extend the existing `node_modules`/`out` patterns).

CI (`.github/workflows/build.yml`): replace the "Download WebUI" and "Download AdminPanel" steps with a single "Build frontend" step that runs `make embed` (or `cd frontend && pnpm install && pnpm build` then copy). Keep the `go build` step. Drop the tarball downloads. Update the workflow's `paths-ignore` if needed.

---

## Data flow

- **Login (local):** `POST /api/v1/auth/local/login` → backend loads user, `GenerateJWT(id, role, ...)`, returns token. Frontend `login(token)` stores it, fetches `/user/profile` (now includes `role`), populates `AuthContext`.
- **Login (GitLab):** OAuth callback → backend finds/creates user (first user → superadmin), `GenerateJWT(id, role, ...)`, redirects to `/callback?token=...`. Frontend `/callback` page calls `login(token)`.
- **Admin nav visibility:** `MainNav` reads `useAuth().user.role`; renders "Admin" link iff admin/superadmin.
- **Admin page access:** `/admin/*` pages wrapped in `withAdmin` → non-admins redirected away. Backend `AdminMiddleware` independently enforces 403 on `/api/v1/admin/*` regardless of what the frontend does.
- **Admin API calls:** `api.get('/admin/users')` → `Authorization: Bearer <jwt>` → `AdminMiddleware` validates JWT + role → handler.

## Error handling

- `AdminMiddleware` returns `403 {"code":-1,"message":"admin privileges required"}` for non-admin authenticated users, and `401` for missing/invalid tokens (delegated to the JWT check inside it).
- `SuperAdminMiddleware` returns `403` for non-superadmin.
- Role-management endpoint returns `400` for invalid `role` values and for self-demotion attempts.
- Frontend: `withAdmin` shows a brief "loading" state then redirects non-admins to `/contests`. (The existing `withAuth` already handles the unauthenticated case → `/login`.)
- SWR global error handler (AdminPanel's `swr-provider.tsx` has a destructive-toast `onError`; WebUI's does not) — adopt WebUI's `SWRProvider` and add AdminPanel's `onError` toast to it, so admin API errors surface toasts. (Merge detail, not a new feature.)

## Testing / verification

- **Backend:** no existing Go test suite. Manual verification:
  1. Fresh DB → register first user → assert `role == superadmin` in `/user/profile` response.
  2. Superadmin promotes a second user to `admin` via `PATCH /api/v1/admin/users/:id/role` → assert 200, target user's `/user/profile` shows `role: admin`.
  3. Non-admin calls `GET /api/v1/admin/users` → assert 403. Admin calls it → 200.
  4. All previous user endpoints (`/contests`, `/submissions`, …) still work on the single port.
  5. Admin endpoints reachable at `/api/v1/admin/*`; old `/api/v1/users` (admin) returns 404 (user router doesn't register it).
- **Frontend:** `pnpm build` succeeds (static export). Manual:
  1. Non-admin sees no "Admin" link; navigating to `/admin/contests` redirects to `/contests`.
  2. Admin sees "Admin" link; sub-nav renders; admin pages load and call `/api/v1/admin/*`.
  3. en/zh locale toggle still works on user pages; admin pages show English.
- **End-to-end:** `make build` produces a `CSOJ` binary; run it, verify single-port UI + API.
- A `verify` pass will exercise the affected flows (login, admin nav visibility, an admin API call) once implemented.

---

## Migration / rollback notes

- The `User.Role` column is additive (default `'user'`); no data loss. Reverting the code leaves the column in place (harmless).
- The single-port change: deployments relying on a separate `:8081` admin port need to update their reverse proxy / network config to expose admin only via the merged port (admin endpoints now require a valid admin JWT). This is a deployment-affecting change — call it out in the release notes and in `docs/getting-started.md`.
- The CI change (build frontend in-repo) requires the CI runner to have pnpm/Node — the `setup-node` + `pnpm/action-setup` actions handle this (matching what the WebUI/AdminPanel CI already used).

## Out of scope (explicit)

- i18n migration for admin pages.
- Superadmin promotion endpoint / multi-superadmin support.
- Archiving or deleting the upstream `CSOJ-WebUI` / `CSOJ-AdminPanel` repos.
- Changes to judger, scheduler, container, or scoring logic.
- Replacing `react-beautiful-dnd` (deprecated) — keep it for now to stay mechanical.
