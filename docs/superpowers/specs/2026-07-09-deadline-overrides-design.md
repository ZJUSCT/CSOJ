# Dynamic Deadline Overrides by Tag — Design

**Date:** 2026-07-09
**Scope:** Add per-problem deadline override rules based on user tags. For each problem, admins can define multiple rules: users with certain tags get a different (typically later) submission deadline. A user matching multiple rules gets the latest applicable deadline.

## Goals

1. **`Problem.DeadlineOverrides`** — a list of `{tags: string[], end_time: string}` rules stored as JSON on the Problem.
2. **`submitToProblem` checks tag-based overrides** — if the user's tags match any override rule, the override's end_time **replaces** the default deadline (even if earlier). Multiple matches → take the latest end_time (most favorable to the user). The effective deadline is checked unconditionally, so an earlier override that has already passed still rejects a late submission.
3. **`GET /problems/:id` returns `effective_end_time`** — computed per-request based on the authenticated user's tags, so the frontend can display the user's actual deadline. The route uses `OptionalAuthMiddleware`: a valid Bearer token identifies the user; anonymous or invalid tokens fall back to the default window (no override applied).
4. **Frontend problem form** — a "Deadline Overrides" panel with add/remove rows (tags input + datetime-local).

## Non-goals

- Override `start_time` (only end/deadline).
- Contest-level overrides (only problem-level).
- Per-user custom deadlines (only tag-based).
- Preserving the default deadline as a floor — overrides **replace** the default when matched, so an admin can also shorten a user's window via a tag (the default is kept only when no rule matches).

---

## Data model

### Go (`internal/judger/loader.go`)

```go
type DeadlineOverride struct {
	Tags    []string `json:"tags"`
	EndTime string   `json:"end_time"` // RFC 3339
}

type Problem struct {
	// ... existing fields ...
	DeadlineOverrides []DeadlineOverride `json:"deadline_overrides,omitempty"`
}
```

### DB (`internal/database/models/models.go`)

`models.Problem` gains:
```go
DeadlineOverrides RawJSON `gorm:"type:text" json:"deadline_overrides"`
```

Same pattern as `Workflow`/`Score` — stored as JSON text, no schema migration.

### `db.go` — LoadFromDB / ProblemToModel

`problemFromModel`: unmarshal `p.DeadlineOverrides` into `[]DeadlineOverride`.
`ProblemToModel`: marshal `p.DeadlineOverrides` back to `models.RawJSON`.

### TS (`frontend/lib/types.ts`)

```ts
export interface DeadlineOverride {
  tags: string[];
  end_time: string;
}
```

`Problem` gains `deadline_overrides?: DeadlineOverride[]` and `effective_end_time?: string | null`.

---

## Backend API

### `submitToProblem` (`internal/api/user/submission.go`)

After the existing problem submission window check (`probSubmitEnd`), add:

```go
// Tag-based deadline overrides
if len(problem.DeadlineOverrides) > 0 {
    user, _ := database.GetUserByID(h.db, userID)
    userTags := strings.Split(user.Tags, ",")
    
    effectiveEnd := *probSubmitEnd  // default
    
    for _, override := range problem.DeadlineOverrides {
        matched := false
        for _, userTag := range userTags {
            for _, overrideTag := range override.Tags {
                if strings.TrimSpace(userTag) != "" && strings.TrimSpace(userTag) == strings.TrimSpace(overrideTag) {
                    matched = true
                    break
                }
            }
            if matched { break }
        }
        if matched {
            overrideTime, err := time.Parse(time.RFC3339, override.EndTime)
            if err == nil && overrideTime.After(effectiveEnd) {
                effectiveEnd = overrideTime
            }
        }
    }
    
    if now.After(effectiveEnd) {
        // 403 "problem submission has ended"
    }
}
```

### `getProblem` (`internal/api/user/problem.go`)

`ProblemResponse` gains:
```go
EffectiveEndTime *time.Time `json:"effective_end_time,omitempty"`
```

In the handler, after fetching the problem, compute `effective_end_time` from the user's tags + overrides (same logic as above). Return it so the frontend can display the user's actual deadline.

---

## Frontend

### Problem form (`frontend/components/admin/problem-actions.tsx`)

New `DeadlineOverridesEditor` component (inside `StepCard`... no, at the problem level, not per-step). Added after the Time Windows section:

```
Deadline Overrides (optional)
┌─────────────────────────────────────────────────┐
│ Tags: VIP, Staff        End: 2026-08-01 23:59   │  [Remove]
│ Tags: Late               End: 2026-09-01 23:59  │  [Remove]
│ [+ Add Override]                                  │
└─────────────────────────────────────────────────┘
```

- Tags: comma-separated Input
- End Time: datetime-local Input
- Add/Remove buttons
- Serialized into `deadline_overrides` JSON on save
- `WorkflowStepEntry` and form state gain `deadline_overrides` array

### User problem detail (`frontend/app/(main)/problems/page.tsx`)

- If `effective_end_time` is non-null and differs from `endtime`/`submit_end_time`, display "Your deadline: {time}" instead of the default.
- The submit button disabled logic uses `effective_end_time` if present.

---

## Testing

- **Backend**: create a problem with `deadline_overrides: [{tags: ["VIP"], end_time: "2027-01-01T00:00:00Z"}]`. Submit as a user with tag "VIP" → allowed past the default deadline. Submit as a user without the tag → blocked.
- **Frontend**: `pnpm build` passes. Admin form shows the override editor; user page shows the effective deadline.
- **Manual**: create a problem with a default end time of today + an override for "VIP" tag with tomorrow. Register two users (one with "VIP" tag, one without). Verify the VIP user can submit while the other can't.

## Out of scope

- Override `start_time`.
- Contest-level overrides.
- Per-user custom deadlines.
- i18n for the override editor (English-only admin, as with other admin features).
