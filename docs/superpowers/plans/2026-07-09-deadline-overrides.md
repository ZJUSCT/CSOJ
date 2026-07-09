# Dynamic Deadline Overrides by Tag — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-problem tag-based deadline override rules — users with certain tags get a different (later) submission deadline.

**Architecture:** `Problem` gains `DeadlineOverrides []DeadlineOverride` (JSON, same pattern as `Workflow`). `submitToProblem` checks overrides after the default window. `getProblem` returns `effective_end_time` per-user. Frontend gains a `DeadlineOverridesEditor` in the problem form.

**Tech Stack:** Go 1.24, Gin, GORM/SQLite, Next.js 14, React 18, TypeScript.

**Reference spec:** `docs/superpowers/specs/2026-07-09-deadline-overrides-design.md`

**Branch:** `hpc101`

---

## Task 1: Backend — add DeadlineOverride type + DB field + db.go wiring

**Files:**
- Modify: `internal/judger/loader.go`
- Modify: `internal/database/models/models.go`
- Modify: `internal/judger/db.go`

- [ ] **Step 1: Add `DeadlineOverride` type + `DeadlineOverrides` field to `Problem`**

In `internal/judger/loader.go`, add after the `MPIConfig` type:

```go
type DeadlineOverride struct {
	Tags    []string `json:"tags"`
	EndTime string   `json:"end_time"`
}
```

Add `DeadlineOverrides` to the `Problem` struct (after `Scheduling`):
```go
	DeadlineOverrides []DeadlineOverride `json:"deadline_overrides,omitempty"`
```

- [ ] **Step 2: Add `DeadlineOverrides` to `models.Problem`**

In `internal/database/models/models.go`, in the `Problem` struct, add after `Score`:
```go
	DeadlineOverrides RawJSON `gorm:"type:text" json:"deadline_overrides"`
```

- [ ] **Step 3: Wire `db.go` — `problemFromModel` + `ProblemToModel`**

In `internal/judger/db.go`, in `problemFromModel`, after the `Score` unmarshal block, add:
```go
	if len(p.DeadlineOverrides) > 0 {
		if err := json.Unmarshal(p.DeadlineOverrides, &prob.DeadlineOverrides); err != nil {
			return nil, fmt.Errorf("parse deadline_overrides: %w", err)
		}
	}
```

In `ProblemToModel`, after the `Score` marshal block, add:
```go
	deadlineOverrides, err := json.Marshal(p.DeadlineOverrides)
	if err != nil {
		return models.Problem{}, fmt.Errorf("marshal deadline_overrides: %w", err)
	}
```

And in the returned `models.Problem`, add:
```go
		DeadlineOverrides: models.RawJSON(deadlineOverrides),
```

- [ ] **Step 4: Verify**

Run: `go build ./internal/database/... ./internal/judger/`
Expected: exit 0.

- [ ] **Step 5: Commit**

```bash
git add internal/judger/loader.go internal/database/models/models.go internal/judger/db.go
git commit -m "feat(db): add DeadlineOverrides to Problem model + db.go wiring"
```

---

## Task 2: Backend — submitToProblem + getProblem

**Files:**
- Modify: `internal/api/user/submission.go`
- Modify: `internal/api/user/problem.go`

- [ ] **Step 1: Add tag-based override check to `submitToProblem`**

In `internal/api/user/submission.go`, in `submitToProblem`, find the block that checks `now.After(*probSubmitEnd)` (the "problem submission has ended" error). Replace that block with:

```go
	if now.After(*probSubmitEnd) {
		// Check tag-based deadline overrides
		if len(problem.DeadlineOverrides) > 0 {
			user, _ := database.GetUserByID(h.db, userID)
			userTags := strings.Split(user.Tags, ",")

			effectiveEnd := *probSubmitEnd

			for _, override := range problem.DeadlineOverrides {
				matched := false
				for _, userTag := range userTags {
					for _, overrideTag := range override.Tags {
						if strings.TrimSpace(userTag) != "" && strings.TrimSpace(userTag) == strings.TrimSpace(overrideTag) {
							matched = true
							break
						}
					}
					if matched {
						break
					}
				}
				if matched {
					overrideTime, err := time.Parse(time.RFC3339, override.EndTime)
					if err == nil && overrideTime.After(effectiveEnd) {
						effectiveEnd = overrideTime
					}
				}
			}

			if now.After(effectiveEnd) {
				h.appState.RUnlock()
				util.Error(c, http.StatusForbidden, fmt.Errorf("problem submission has ended"))
				return
			}
		} else {
			h.appState.RUnlock()
			util.Error(c, http.StatusForbidden, fmt.Errorf("problem submission has ended"))
			return
		}
	}
```

Make sure `"strings"` is imported in this file.

- [ ] **Step 2: Add `EffectiveEndTime` to `ProblemResponse` + compute it**

In `internal/api/user/problem.go`:

Add to `ProblemResponse` struct:
```go
	EffectiveEndTime *time.Time `json:"effective_end_time,omitempty"`
```

In the `getProblem` handler, after building the response and before `util.Success`, add logic to compute the effective end time:

```go
	// Compute effective end time based on tag overrides
	var effectiveEnd *time.Time
	if problem.SubmitEndTime != nil {
		effectiveEnd = problem.SubmitEndTime
	} else {
		endTime := problem.EndTime
		effectiveEnd = &endTime
	}

	if len(problem.DeadlineOverrides) > 0 {
		user, _ := database.GetUserByID(h.db, userID)
		userTags := strings.Split(user.Tags, ",")
		for _, override := range problem.DeadlineOverrides {
			matched := false
			for _, userTag := range userTags {
				for _, overrideTag := range override.Tags {
					if strings.TrimSpace(userTag) != "" && strings.TrimSpace(userTag) == strings.TrimSpace(overrideTag) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if matched {
				overrideTime, err := time.Parse(time.RFC3339, override.EndTime)
				if err == nil && overrideTime.After(*effectiveEnd) {
					effectiveEnd = &overrideTime
				}
			}
		}
	}
	response.EffectiveEndTime = effectiveEnd
```

Add imports `"strings"`, `"time"` (if not already present), `"github.com/ZJUSCT/CSOJ/internal/database"`.

- [ ] **Step 3: Verify**

Run: `go build ./...`
Expected: exit 0.

Run: `go vet ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/api/user/submission.go internal/api/user/problem.go
git commit -m "feat(api): tag-based deadline overrides in submitToProblem + getProblem"
```

---

## Task 3: Frontend — types + problem form editor

**Files:**
- Modify: `frontend/lib/types.ts`
- Modify: `frontend/components/admin/problem-actions.tsx`

- [ ] **Step 1: Add types**

In `frontend/lib/types.ts`, add:
```ts
export interface DeadlineOverride {
  tags: string[];
  end_time: string;
}
```

Add to `Problem` interface: `deadline_overrides?: DeadlineOverride[];` and `effective_end_time?: string | null;`

- [ ] **Step 2: Add `DeadlineOverridesEditor` to problem form**

In `frontend/components/admin/problem-actions.tsx`:

Add to `WorkflowStepEntry`... no — deadline overrides are at the problem level, not per-step. Add to the form state separately. In the `ProblemFormDialog` component:

Add state + handlers:
```tsx
const [deadlineOverrides, setDeadlineOverrides] = useState(
    problem?.deadline_overrides?.map(o => ({ tags: o.tags.join(', '), end_time: o.end_time ? format(new Date(o.end_time), "yyyy-MM-dd'T'HH:mm") : '' })) || []
);

const addOverride = () => setDeadlineOverrides([...deadlineOverrides, { tags: '', end_time: '' }]);
const removeOverride = (i: number) => setDeadlineOverrides(deadlineOverrides.filter((_, idx) => idx !== i));
const updateOverride = (i: number, field: 'tags' | 'end_time', value: string) => {
    const next = [...deadlineOverrides];
    next[i] = { ...next[i], [field]: value };
    setDeadlineOverrides(next);
};
```

In the `useEffect` that resets the form on open, add:
```tsx
setDeadlineOverrides(problem?.deadline_overrides?.map(o => ({ tags: o.tags.join(', '), end_time: o.end_time ? format(new Date(o.end_time), "yyyy-MM-dd'T'HH:mm") : '' })) || []);
```

In `onSubmit`, serialize the overrides into the payload:
```tsx
deadline_overrides: deadlineOverrides.filter(o => o.tags.trim() !== '' && o.end_time !== '').map(o => ({
    tags: o.tags.split(',').map(t => t.trim()).filter(Boolean),
    end_time: new Date(o.end_time).toISOString(),
})),
```

Render the editor after the Time Windows section:
```tsx
<Separator />
<div className="space-y-2">
    <Label className="flex items-center gap-1">Deadline Overrides (optional)</Label>
    <p className="text-xs text-muted-foreground">Users with matching tags get a later submission deadline. Multiple matches take the latest.</p>
    {deadlineOverrides.map((override, i) => (
        <div key={i} className="grid grid-cols-[1fr_1fr_32px] gap-2 items-center">
            <Input className="text-xs" value={override.tags} onChange={e => updateOverride(i, 'tags', e.target.value)} placeholder="VIP, Staff" />
            <Input className="text-xs" type="datetime-local" value={override.end_time} onChange={e => updateOverride(i, 'end_time', e.target.value)} />
            <Button type="button" variant="ghost" size="icon" className="h-8 w-8 text-destructive" onClick={() => removeOverride(i)}><Trash2 className="h-3 w-3" /></Button>
        </div>
    ))}
    <Button type="button" variant="outline" size="sm" onClick={addOverride}><PlusCircle className="h-3 w-3 mr-1" /> Add Override</Button>
</div>
```

- [ ] **Step 3: Verify**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
git add frontend/lib/types.ts frontend/components/admin/problem-actions.tsx
git commit -m "feat(frontend): deadline overrides editor in problem form"
```

---

## Task 4: Frontend — user problem page effective deadline display

**Files:**
- Modify: `frontend/app/(main)/problems/page.tsx`

- [ ] **Step 1: Use `effective_end_time` in the submit check**

In `frontend/app/(main)/problems/page.tsx`, in the `ProblemDetails` component:

Find where `submit_end_time` is used to compute the submission window. Replace the effective end time used for display + submit button disabling:

```tsx
const effectiveEndTime = problem.effective_end_time
    ? new Date(problem.effective_end_time)
    : problem.submit_end_time
        ? new Date(problem.submit_end_time)
        : new Date(problem.endtime);

const now = new Date();
const submissionEnded = now > effectiveEndTime;
const submissionNotOpened = now < submitStartTime;
```

Use `effectiveEndTime` for the "submission opens at" / "submission closed" messages instead of the default.

- [ ] **Step 2: Verify**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add "frontend/app/(main)/problems/page.tsx"
git commit -m "feat(frontend): display effective deadline on user problem page"
```

---

## Task 5: Build + verify

- [ ] **Step 1: Full build**

Run: `go build ./... && go vet ./... && cd frontend && pnpm build`
Expected: clean.

Run: `make build`
Expected: binary produced.

- [ ] **Step 2: No commit**

---

## Self-Review notes

- `DeadlineOverride.EndTime` is a string (RFC 3339) — stored as JSON in the `RawJSON` column, parsed at runtime with `time.Parse(time.RFC3339, ...)`. Invalid dates are silently skipped (the override doesn't apply).
- The override logic takes the **latest** matching override end time (most favorable to the user). If no override matches, the default `probSubmitEnd` applies.
- `getProblem` returns `effective_end_time` computed per-request from the authenticated user's tags. The frontend uses this to display the user's actual deadline.
- The `DeadlineOverridesEditor` follows the same add/remove-row pattern as `MountsEditor` and `TolerationsEditor` in the problem form.
- The overrides are at the problem level (not per workflow step) — they only affect the submission deadline, not the step execution.
- `strings.TrimSpace` is used when comparing tags to handle whitespace from comma-separated input.
