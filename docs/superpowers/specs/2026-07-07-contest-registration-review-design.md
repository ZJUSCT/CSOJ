# Contest Registration Review — Design

**Date:** 2026-07-07
**Scope:** Add a registration approval mechanism for contests. Four configurable modes: auto-approve, tag-based auto-approve, tag-based review, full review. A new `ContestRegistration` DB table tracks per-user registration status (approved/pending/rejected). An admin review page lists pending registrations.

## Goals

1. **`ContestRegistration` DB table** — per-user, per-contest registration records with `status` (approved/pending/rejected), `reviewer_id`, `reviewed_at`.
2. **Four registration modes** stored in the Contest's JSON `registration_config`:
   - `auto` — all registrations immediately approved (current behavior)
   - `tag_auto` — users with any of `allowed_tags` approved immediately; others 403
   - `tag_review` — users with tags approved immediately; others pending review
   - `review` — all registrations pending review
3. **User API** — `POST /contests/:id/register` (creates registration per mode), `GET /contests/:id/registration` (checks status).
4. **Admin API** — `GET /admin/contests/:id/registrations` (list), `PATCH /admin/registrations/:regID` (approve/reject).
5. **Frontend** — contest form gains a Registration config section; user contest page shows registration status; new admin `/admin/registrations` page for review.

## Non-goals

- Bulk approve/reject (single action per registration).
- Email notifications (out of scope; admin checks the dashboard).
- Registration time windows (the contest start/end time already gates submission).

---

## Data model

### New `ContestRegistration` table (`internal/database/models/models.go`)

```go
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

### Contest gains `RegistrationConfig`

`models.Contest` gains a `RegistrationConfig RawJSON` column (stored as JSON, same pattern as `Problem.Workflow`):
```go
type Contest struct {
    // ... existing fields ...
    RegistrationConfig RawJSON `gorm:"type:text" json:"registration_config"`
}
```

`judger.Contest` (in-memory) gains:
```go
type Contest struct {
    // ... existing fields ...
    RegistrationConfig *RegistrationConfig `json:"registration_config,omitempty"`
}

type RegistrationConfig struct {
    Mode         string   `json:"mode"`                    // "auto" | "tag_auto" | "tag_review" | "review"
    AllowedTags  []string `json:"allowed_tags,omitempty"`   // used by tag_auto, tag_review
}
```

### Backward compatibility

`IsUserRegisteredForContest` checks `ContestRegistration` with `status="approved"` first; if no row exists, falls back to `ContestScoreHistory` (so users who submitted before this feature are still registered). `RegisterForContest` now creates a `ContestRegistration` row instead of a `ContestScoreHistory` row (the score history is created on first submission, as today).

### AutoMigrate

`database.Init` adds `&models.ContestRegistration{}` to `AutoMigrate`.

---

## Registration flow

### `POST /api/v1/contests/:id/register` (user)

1. Load the contest from `appState.Contests[contestID]`. If not found → 404.
2. Check if a `ContestRegistration` already exists for this user+contest → 409 "Already registered".
3. Read `contest.RegistrationConfig`:
   - `auto` (or nil) → create `ContestRegistration{status: "approved"}`, return 200 "Registered successfully".
   - `tag_auto` → check user's tags against `AllowedTags`. If match → `approved`, return 200. If no match → return 403 "You are not allowed to register for this contest".
   - `tag_review` → if user has a matching tag → `approved`, return 200. If not → `pending`, return 200 "Registration submitted, pending review".
   - `review` → `pending`, return 200 "Registration submitted, pending review".
4. Create the `ContestRegistration` row.

### `GET /api/v1/contests/:id/registration` (user)

Returns `{status: "approved"|"pending"|"rejected"|"not_registered"}` for the current user + contest.

### `GET /api/v1/admin/contests/:id/registrations` (admin)

Returns all `ContestRegistration` rows for the contest, with user preloaded. Supports `?status=pending` filter.

### `PATCH /api/v1/admin/registrations/:regID` (admin)

Body `{"action": "approve"|"reject"}`. Updates the registration's status, sets `reviewer_id` and `reviewed_at`.

### Submission gate

`submitToProblem` checks `IsUserRegisteredForContest` (which now checks `ContestRegistration` with `status="approved"`). `pending`/`rejected`/`not_registered` → 403.

---

## Frontend

### Contest form (`contest-actions.tsx`)

New "Registration" section after Score Settings:
- **Mode** (Select): `auto` / `tag_auto` / `tag_review` / `review`
- **Allowed Tags** (Input, comma-separated): visible only when mode is `tag_auto` or `tag_review`
- Saved as `registration_config` JSON in the contest.

### User contest page (`contests/page.tsx`)

- `GET /contests/:id/registration` on load → drives the registration UI:
  - `not_registered` → show "Register" button
  - `pending` → show "Pending review" badge, disable submit
  - `approved` → show "Registered ✓" (current behavior)
  - `rejected` → show "Registration rejected" badge, disable submit
- `POST /contests/:id/register` response message drives the toast.

### Admin review page — `frontend/app/(main)/admin/registrations/page.tsx`

- `withAdmin` + `<AdminSubNav />`.
- `useSWR('/admin/clusters/status')`... no — fetches all contests, then lets the admin pick a contest (Select) to view its registrations.
- `useSWR('/admin/contests/:id/registrations')` → table: user nickname/username, tags, status badge, created_at, approve/reject buttons.
- Each pending row: "Approve" (green button) + "Reject" (red button) → `PATCH /admin/registrations/:regID`.
- Filter by status (default: pending).

### Sub-nav

Add `{ href: "/admin/registrations", label: "Registrations", icon: ClipboardCheck }` to `admin-sub-nav.tsx`.

### Types (`frontend/lib/types.ts`)

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

`Contest` gains `registration_config?: RegistrationConfig`.

---

## Testing

- **Backend**: manual smoke test — create a contest with mode `review`, register as a user → pending; admin approves → user can submit. Test `tag_auto` with a user who has the tag vs doesn't.
- **Frontend**: `pnpm build` — 19 pages (18 + the new registrations page).
- **Manual**: verify the registration status displays correctly on the user contest page.

## Out of scope

- Bulk approve/reject.
- Email notifications.
- Registration time windows.
- Migration of existing `ContestScoreHistory` rows to `ContestRegistration` (backward-compat fallback handles it).
