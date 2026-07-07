# Contest Registration Review — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a contest registration approval mechanism with four configurable modes (auto/tag_auto/tag_review/review), a `ContestRegistration` DB table, admin review API + frontend page, and contest form registration config.

**Architecture:** A new `ContestRegistration` table tracks per-user registration status (approved/pending/rejected). The contest's `RegistrationConfig` JSON determines the mode. The user `POST /contests/:id/register` endpoint creates registrations per mode. Admins approve/reject via `PATCH /admin/registrations/:regID`. The frontend gains a registration config section in the contest form, status display on the user contest page, and a new admin review page.

**Tech Stack:** Go 1.24, Gin, GORM/SQLite, Next.js 14, React 18, TypeScript, shadcn/ui.

**Reference spec:** `docs/superpowers/specs/2026-07-07-contest-registration-review-design.md`

**Branch:** `merge-webui-admin`.

---

## Task 1: Backend — ContestRegistration model + CRUD + Contest config

**Files:**
- Modify: `internal/database/models/models.go`
- Modify: `internal/database/database.go`
- Modify: `internal/database/crud.go`
- Modify: `internal/judger/loader.go`
- Modify: `internal/judger/db.go`

- [ ] **Step 1: Add `ContestRegistration` model**

In `internal/database/models/models.go`, append:
```go
// ContestRegistration tracks a user's registration for a contest.
type ContestRegistration struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	ContestID  string     `gorm:"index:idx_reg_user_contest,unique" json:"contest_id"`
	UserID     string     `gorm:"index:idx_reg_user_contest,unique" json:"user_id"`
	User       User       `gorm:"foreignKey:UserID" json:"user"`
	Status     string     `gorm:"default:pending" json:"status"` // "approved" | "pending" | "rejected"
	CreatedAt  time.Time  `json:"created_at"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	ReviewerID string     `json:"reviewer_id,omitempty"`
}
```

- [ ] **Step 2: Add `RegistrationConfig` to `models.Contest`**

In `models.Contest`, add after `ProblemIDs`:
```go
	RegistrationConfig RawJSON `gorm:"type:text" json:"registration_config"`
```

- [ ] **Step 3: AutoMigrate `ContestRegistration`**

In `internal/database/database.go`, add `&models.ContestRegistration{}` to `AutoMigrate`.

- [ ] **Step 4: Add CRUD helpers in `crud.go`**

Append to `internal/database/crud.go`:
```go
// --- Contest Registrations ---

func CreateRegistration(db *gorm.DB, reg *models.ContestRegistration) error {
	return db.Create(reg).Error
}

func GetRegistration(db *gorm.DB, userID, contestID string) (*models.ContestRegistration, error) {
	var reg models.ContestRegistration
	err := db.Where("user_id = ? AND contest_id = ?", userID, contestID).First(&reg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &reg, err
}

func GetRegistrationsByContest(db *gorm.DB, contestID string, status string) ([]models.ContestRegistration, error) {
	var regs []models.ContestRegistration
	q := db.Preload("User").Where("contest_id = ?", contestID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("created_at desc").Find(&regs).Error
	return regs, err
}

func UpdateRegistrationStatus(db *gorm.DB, regID, status, reviewerID string) error {
	now := time.Now()
	return db.Model(&models.ContestRegistration{}).Where("id = ?", regID).
		Updates(map[string]interface{}{
			"status":      status,
			"reviewer_id": reviewerID,
			"reviewed_at": now,
		}).Error
}

func IsUserApprovedForContest(db *gorm.DB, userID, contestID string) (bool, error) {
	var count int64
	err := db.Model(&models.ContestRegistration{}).
		Where("user_id = ? AND contest_id = ? AND status = ?", userID, contestID, "approved").
		Count(&count).Error
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	// Fallback: check ContestScoreHistory for backward compat (pre-feature registrations).
	return IsUserRegisteredForContest(db, userID, contestID)
}
```

- [ ] **Step 5: Add `RegistrationConfig` to judger types**

In `internal/judger/loader.go`, add:
```go
type RegistrationConfig struct {
	Mode        string   `json:"mode"`                    // "auto" | "tag_auto" | "tag_review" | "review"
	AllowedTags []string `json:"allowed_tags,omitempty"`
}
```

Add `RegistrationConfig *RegistrationConfig` to the `Contest` struct (after `Announcements`).

- [ ] **Step 6: Update `LoadFromDB` + `ContestToModel`**

In `internal/judger/db.go`, in `LoadFromDB`'s contest-building loop, add:
```go
		if len(c.RegistrationConfig) > 0 {
			var rc RegistrationConfig
			if err := json.Unmarshal(c.RegistrationConfig, &rc); err == nil {
				contests[c.ID].RegistrationConfig = &rc
			}
		}
```

In `ContestToModel`, add:
```go
		if c.RegistrationConfig != nil {
			rc, _ := json.Marshal(c.RegistrationConfig)
			mc.RegistrationConfig = models.RawJSON(rc)
		}
```

- [ ] **Step 7: Verify**

Run: `go build ./internal/database/... ./internal/judger/`
Expected: exit 0.

- [ ] **Step 8: Commit**

```bash
git add internal/database/models/models.go internal/database/database.go internal/database/crud.go internal/judger/loader.go internal/judger/db.go
git commit -m "feat(db): ContestRegistration model + CRUD + RegistrationConfig on Contest"
```

---

## Task 2: Backend — user + admin registration API

**Files:**
- Modify: `internal/api/user/contest.go`
- Modify: `internal/api/user/router.go`
- Modify: `internal/api/admin/contest.go`
- Modify: `internal/api/admin/router.go`

- [ ] **Step 1: Rewrite `registerForContest` in `internal/api/user/contest.go`**

Replace the existing `registerForContest` handler:
```go
func (h *Handler) registerForContest(c *gin.Context) {
	userID := c.GetString("userID")
	contestID := c.Param("id")

	h.appState.RLock()
	contest, ok := h.appState.Contests[contestID]
	h.appState.RUnlock()

	if !ok {
		util.Error(c, http.StatusNotFound, fmt.Errorf("contest not found"))
		return
	}

	now := time.Now()
	if now.Before(contest.StartTime) {
		util.Error(c, http.StatusForbidden, fmt.Errorf("contest has not started, cannot register"))
		return
	}
	if now.After(contest.EndTime) {
		util.Error(c, http.StatusForbidden, fmt.Errorf("contest has ended, cannot register"))
		return
	}

	// Check existing registration
	existing, err := database.GetRegistration(h.db, userID, contestID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	if existing != nil {
		util.Error(c, http.StatusConflict, fmt.Errorf("already registered"))
		return
	}

	// Determine registration mode
	mode := "auto"
	allowedTags := []string{}
	if contest.RegistrationConfig != nil {
		mode = contest.RegistrationConfig.Mode
		allowedTags = contest.RegistrationConfig.AllowedTags
	}

	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}

	userTags := strings.Split(user.Tags, ",")
	hasTag := false
	for _, t := range allowedTags {
		for _, ut := range userTags {
			if strings.TrimSpace(t) == strings.TrimSpace(ut) && t != "" {
				hasTag = true
				break
			}
		}
		if hasTag {
			break
		}
	}

	var status string
	var msg string
	switch mode {
	case "auto":
		status = "approved"
		msg = "Successfully registered for contest"
	case "tag_auto":
		if hasTag {
			status = "approved"
			msg = "Successfully registered for contest"
		} else {
			util.Error(c, http.StatusForbidden, fmt.Errorf("you are not allowed to register for this contest"))
			return
		}
	case "tag_review":
		if hasTag {
			status = "approved"
			msg = "Successfully registered for contest"
		} else {
			status = "pending"
			msg = "Registration submitted, pending review"
		}
	case "review":
		status = "pending"
		msg = "Registration submitted, pending review"
	default:
		status = "approved"
		msg = "Successfully registered for contest"
	}

	reg := &models.ContestRegistration{
		ID:        uuid.NewString(),
		ContestID: contestID,
		UserID:    userID,
		Status:    status,
	}
	if err := database.CreateRegistration(h.db, reg); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	util.Success(c, gin.H{"status": status}, msg)
}
```

Add imports: `"strings"`, `"github.com/google/uuid"`, `"github.com/ZJUSCT/CSOJ/internal/database/models"`.

- [ ] **Step 2: Add `getRegistrationStatus` handler**

In `internal/api/user/contest.go`, add:
```go
func (h *Handler) getRegistrationStatus(c *gin.Context) {
	userID := c.GetString("userID")
	contestID := c.Param("id")

	reg, err := database.GetRegistration(h.db, userID, contestID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	status := "not_registered"
	if reg != nil {
		status = reg.Status
	}

	util.Success(c, gin.H{"status": status}, "Registration status retrieved")
}
```

- [ ] **Step 3: Register the user route**

In `internal/api/user/router.go`, add after `authed.POST("/contests/:id/register", h.registerForContest)`:
```go
			authed.GET("/contests/:id/registration", h.getRegistrationStatus)
```

- [ ] **Step 4: Add admin registration handlers in `internal/api/admin/contest.go`**

Append:
```go
func (h *Handler) listRegistrations(c *gin.Context) {
	contestID := c.Param("id")
	status := c.Query("status")
	regs, err := database.GetRegistrationsByContest(h.db, contestID, status)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, regs, "Registrations retrieved")
}

func (h *Handler) reviewRegistration(c *gin.Context) {
	regID := c.Param("regID")
	var req struct {
		Action string `json:"action" binding:"required"` // "approve" | "reject"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}

	status := ""
	switch req.Action {
	case "approve":
		status = "approved"
	case "reject":
		status = "rejected"
	default:
		util.Error(c, http.StatusBadRequest, "action must be 'approve' or 'reject'")
		return
	}

	reviewerID := c.GetString("userID")
	if err := database.UpdateRegistrationStatus(h.db, regID, status, reviewerID); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "Registration "+status)
}
```

Add imports: `"github.com/ZJUSCT/CSOJ/internal/database"` (likely already imported).

- [ ] **Step 5: Register admin routes**

In `internal/api/admin/router.go`, inside the `contests` group block, add:
```go
			contests.GET("/:id/registrations", h.listRegistrations)
```

After the `contests` group, add a sibling for registration review (admin-level, no contest prefix):
```go
		// Registration review (admin)
		adminV1.PATCH("/registrations/:regID", h.reviewRegistration)
```

- [ ] **Step 6: Verify**

Run: `go build ./...`
Expected: exit 0.

- [ ] **Step 7: Commit**

```bash
git add internal/api/user/contest.go internal/api/user/router.go internal/api/admin/contest.go internal/api/admin/router.go
git commit -m "feat(api): contest registration flow + admin review endpoints"
```

---

## Task 3: Frontend — types + contest form + user page + admin review page

**Files:**
- Modify: `frontend/lib/types.ts`
- Modify: `frontend/components/admin/contest-actions.tsx`
- Modify: `frontend/app/(main)/contests/page.tsx`
- Modify: `frontend/components/layout/admin-sub-nav.tsx`
- Create: `frontend/app/(main)/admin/registrations/page.tsx`

- [ ] **Step 1: Update types**

In `frontend/lib/types.ts`, add `registration_config?: RegistrationConfig` to the `Contest` interface. Add:
```ts
export interface RegistrationConfig {
  mode: "auto" | "tag_auto" | "tag_review" | "review";
  allowed_tags?: string[];
}

export interface ContestRegistration {
  id: string;
  contest_id: string;
  user_id: string;
  user: { nickname: string; username: string; tags: string };
  status: "approved" | "pending" | "rejected";
  created_at: string;
  reviewed_at?: string;
}
```

- [ ] **Step 2: Add Registration config to contest form**

In `frontend/components/admin/contest-actions.tsx`, in the `ContestFormDialog`:
- Add a "Registration" section (after Score Settings, before Description):
  - Mode (Select): `auto` / `tag_auto` / `tag_review` / `review`
  - Allowed Tags (Input, comma-separated): visible when mode is `tag_auto` or `tag_review`
- The form stores `registration_config` as a JSON object. On submit, serialize it.

Read the current `contest-actions.tsx` to understand the form structure (it uses `zodResolver` + `FormField`).

- [ ] **Step 3: Update user contest page**

In `frontend/app/(main)/contests/page.tsx`:
- Replace the registration check: instead of `useSWR('/contests/:id/history')`, use `useSWR('/contests/:id/registration')`.
- Show the registration status: `approved` → "Registered ✓" + enable submit; `pending` → "Pending review" badge + disable submit; `rejected` → "Registration rejected" badge + disable submit; `not_registered` → show "Register" button.
- The `POST /contests/:id/register` response includes `{status: "approved"|"pending"}` — show an appropriate toast.

- [ ] **Step 4: Create admin review page**

Create `frontend/app/(main)/admin/registrations/page.tsx`:
- `withAdmin` + `<AdminSubNav />`.
- A contest Select (load all contests via `useSWR('/admin/contests')`).
- A status filter Select (all / pending / approved / rejected).
- `useSWR('/admin/contests/:id/registrations?status=pending')` → table: user nickname/username, tags, status badge, created_at.
- Each pending row: "Approve" (green) + "Reject" (red) buttons → `PATCH /admin/registrations/:regID`.
- Use existing shadcn primitives (Card, Table, Button, Badge, Select).

- [ ] **Step 5: Add sub-nav entry**

In `frontend/components/layout/admin-sub-nav.tsx`, add:
```ts
    { href: "/admin/registrations", label: "Registrations", icon: ClipboardCheck },
```
Import `ClipboardCheck` from `lucide-react`.

- [ ] **Step 6: Verify**

Run: `cd frontend && pnpm build`
Expected: 19 pages (18 + registrations), succeeds.

- [ ] **Step 7: Commit**

```bash
git add frontend/lib/types.ts frontend/components/admin/contest-actions.tsx "frontend/app/(main)/contests/page.tsx" "frontend/app/(main)/admin/registrations/page.tsx" frontend/components/layout/admin-sub-nav.tsx
git commit -m "feat(frontend): registration review page + contest form config + user status"
```

---

## Task 4: Build + verify

**Files:** (no changes — verification)

- [ ] **Step 1: Build everything**

Run: `go build ./... && go vet ./... && cd frontend && pnpm build`
Expected: Go clean, 19 frontend pages.

- [ ] **Step 2: Manual smoke test**

Start the server. Create a contest with `registration_config: {"mode":"review"}`. Register as a user → pending. Admin approves via `/admin/registrations` → user can submit. Test `tag_auto` with a user who has/lacks the tag.

- [ ] **Step 3: No commit**

---

## Self-Review notes

- `IsUserApprovedForContest` falls back to `IsUserRegisteredForContest` (which checks `ContestScoreHistory`) for backward compat — this means pre-feature users who submitted without a `ContestRegistration` row are still treated as approved.
- The `registerForContest` handler checks existing registration FIRST (before mode logic) to prevent duplicates and return 409.
- The admin `registerUserForContest` endpoint (in `admin/user.go`) still calls the old `database.RegisterForContest` (creates a `ContestScoreHistory` row). This should be updated to call `database.CreateRegistration` with `status="approved"` instead. Add this fix to Task 2.
- The `registration_config` field is stored as `RawJSON` on `models.Contest` — same pattern as `Problem.Workflow`. The contest form serializes/deserializes it as JSON.
- The frontend contest form needs to handle `registration_config` as a nested object in the form state, similar to how `score` and `upload` are handled.
