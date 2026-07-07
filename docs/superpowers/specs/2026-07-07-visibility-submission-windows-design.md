# Contest/Problem Visibility + Submission Time Windows — Design

**Date:** 2026-07-07
**Scope:** Split each contest's and problem's time window into a **visibility window** (when it appears on the homepage/contest list and can be browsed) and a **submission window** (when users can submit). Both are configurable from the admin frontend. `nil` submission times fall back to the visibility times (current behavior).

## Goals

1. **Contest gains `SubmitStartTime *time.Time` + `SubmitEndTime *time.Time`** — `nil` = fall back to `StartTime`/`EndTime` (current behavior).
2. **Problem gains `SubmitStartTime *time.Time` + `SubmitEndTime *time.Time`** — same pattern.
3. **Backend checks**: `submitToProblem` checks both contest-level and problem-level submission windows. `getProblem` returns the submit times so the frontend can show "submission opens at..." and disable the submit button.
4. **Frontend forms**: contest + problem edit dialogs gain a "Submission Time (optional)" section with two `datetime-local` inputs.
5. **Frontend user pages**: contest list + problem detail show submission-window status (opens at / closes at / closed), disabling the submit button accordingly.

## Non-goals

- Renaming the existing `StartTime`/`EndTime` fields (they stay; their meaning shifts to "visibility window" conceptually but the JSON field names are unchanged).
- Admin-level time restrictions (admin sees everything regardless of windows).
- Time zone handling (all times are UTC in the DB; the frontend displays local time via `datetime-local` inputs and `date-fns` formatting).

---

## Data model

### Go (`internal/judger/loader.go`)

`Contest` gains:
```go
SubmitStartTime *time.Time `json:"submit_start_time,omitempty"`
SubmitEndTime   *time.Time `json:"submit_end_time,omitempty"`
```

`Problem` gains:
```go
SubmitStartTime *time.Time `json:"submit_start_time,omitempty"`
SubmitEndTime   *time.Time `json:"submit_end_time,omitempty"`
```

### DB (`internal/database/models/models.go`)

`models.Contest` gains:
```go
SubmitStartTime *time.Time `json:"submit_start_time,omitempty"`
SubmitEndTime   *time.Time `json:"submit_end_time,omitempty"`
```

`models.Problem` gains:
```go
SubmitStartTime *time.Time `json:"submit_start_time,omitempty"`
SubmitEndTime   *time.Time `json:"submit_end_time,omitempty"`
```

GORM AutoMigrate adds the columns (nullable). Existing rows: `nil` = fall back to `StartTime`/`EndTime`.

### TS (`frontend/lib/types.ts`)

`Contest` gains `submit_start_time?: string | null` + `submit_end_time?: string | null`.
`Problem` gains same.

### `internal/judger/db.go`

`LoadFromDB` reads `c.SubmitStartTime`/`c.SubmitEndTime` into `judger.Contest`. `ContestToModel` writes them back. Same for `ProblemToModel`/`problemFromModel`.

---

## Backend API logic

### `submitToProblem` (`internal/api/user/problem.go`)

```go
now := time.Now()

// Contest visibility check
if now.Before(contest.StartTime) || now.After(contest.EndTime) {
    → 404 "contest not found"
}

// Contest submission window
submitStart := contest.SubmitStartTime
if submitStart == nil { submitStart = &contest.StartTime }
submitEnd := contest.SubmitEndTime
if submitEnd == nil { submitEnd = &contest.EndTime }
if now.Before(*submitStart) {
    → 403 "contest submission has not opened yet"
}
if now.After(*submitEnd) {
    → 403 "contest submission has ended"
}

// Problem visibility check
if now.Before(problem.StartTime) {
    → 403 "problem has not started yet"
}
if now.After(problem.EndTime) {
    → 403 "problem has ended"
}

// Problem submission window
probSubmitStart := problem.SubmitStartTime
if probSubmitStart == nil { probSubmitStart = &problem.StartTime }
probSubmitEnd := problem.SubmitEndTime
if probSubmitEnd == nil { probSubmitEnd = &problem.EndTime }
if now.Before(*probSubmitStart) {
    → 403 "problem submission has not opened yet"
}
if now.After(*probSubmitEnd) {
    → 403 "problem submission has ended"
}
```

### `getProblem` (`internal/api/user/problem.go`)

- Returns `submit_start_time` and `submit_end_time` in the response (from the problem + from the parent contest).
- The visibility check relaxes: if `now >= problem.StartTime`, return the problem (even if submission hasn't opened). Currently it blocks on `contest.StartTime` + `problem.StartTime`; the relaxation is that the problem is visible once its own `StartTime` has passed, regardless of the submission window.

### `registerForContest` (`internal/api/user/contest.go`)

- Checks **visibility times** (`StartTime`/`EndTime`), not submission times. Registration is open during the visibility window.

### Admin create/update

- `createContest`/`updateContest` accept `submit_start_time`/`submit_end_time` (optional, `nil` = unset).
- `createProblemInContest`/`updateProblem` accept same.
- Admin pages show all 4 time fields regardless of current time.

---

## Frontend

### Types (`frontend/lib/types.ts`)

`Contest` + `Problem` gain `submit_start_time?: string | null` + `submit_end_time?: string | null`.

### Contest form (`frontend/components/admin/contest-actions.tsx`)

The time section changes from:
```
Start Time    [datetime-local]
End Time      [datetime-local]
```
to:
```
Visibility Time
  Visible Start   [datetime-local]
  Visible End     [datetime-local]
Submission Time (optional — empty = same as visibility)
  Submit Start    [datetime-local]
  Submit End      [datetime-local]
```

- `submit_start_time`/`submit_end_time` are optional fields. Empty string → `null` on save.
- Pre-fill when editing if non-null.

### Problem form (`frontend/components/admin/problem-actions.tsx`)

Same layout change for the problem's time fields.

### User contest list (`frontend/app/(main)/contests/page.tsx`)

Status logic:
- `now < starttime` → "Upcoming"
- `starttime ≤ now ≤ endtime`:
  - `submit_start_time` non-null and `now < submit_start_time` → "Visible, submission opens at {time}"
  - `submit_end_time` non-null and `now > submit_end_time` → "Visible, submission closed"
  - Otherwise → "Ongoing"
- `now > endtime` → "Ended"

Submit button disabled when submission window not open.

### User problem detail (`frontend/app/(main)/problems/page.tsx`)

- If `submit_start_time` non-null and not yet → "Submission opens at {time}" + disabled submit.
- If `submit_end_time` non-null and past → "Submission has ended" + disabled submit.
- Otherwise → normal submit button.

### Admin detail pages

Show all 4 time fields (visible start/end + submit start/end) in the info card. Admin is not restricted by time windows.

---

## Testing

- **Backend**: manual — create a contest with `submit_start_time` 1 hour in the future; verify the contest is visible but submission is blocked (403). Verify `nil` fallback works.
- **Frontend**: `pnpm build` — 19 pages.
- **Manual**: edit a contest/problem with submit times; verify the user-facing pages show the correct status and disable the submit button.

## Out of scope

- Renaming `StartTime`/`EndTime` JSON fields.
- Time zone display logic (UTC in DB; frontend uses local time).
- Admin-level time restrictions.
