# Visibility + Submission Time Windows — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split each contest's and problem's time window into a visibility window (when it appears and can be browsed) and an optional submission window (when users can submit). Both configurable from the admin frontend.

**Architecture:** Contest and Problem gain `SubmitStartTime *time.Time` + `SubmitEndTime *time.Time` fields (nil = fall back to visibility times). Backend `submitToProblem` checks both windows. Frontend forms gain optional submit-time inputs; user pages show submission-window status.

**Tech Stack:** Go 1.24, Gin, GORM/SQLite, Next.js 14, React 18, TypeScript.

**Reference spec:** `docs/superpowers/specs/2026-07-07-visibility-submission-windows-design.md`

**Branch:** `merge-webui-admin`.

---

## Task 1: Backend — add SubmitStartTime/SubmitEndTime to models + judger types + db.go

**Files:**
- Modify: `internal/database/models/models.go`
- Modify: `internal/judger/loader.go`
- Modify: `internal/judger/db.go`

- [ ] **Step 1: Add fields to `models.Contest` and `models.Problem`**

In `internal/database/models/models.go`:
- In `Contest` struct, after `EndTime`:
```go
	SubmitStartTime *time.Time `json:"submit_start_time,omitempty"`
	SubmitEndTime   *time.Time `json:"submit_end_time,omitempty"`
```
- In `Problem` struct, after `EndTime`:
```go
	SubmitStartTime *time.Time `json:"submit_start_time,omitempty"`
	SubmitEndTime   *time.Time `json:"submit_end_time,omitempty"`
```

- [ ] **Step 2: Add fields to `judger.Contest` and `judger.Problem`**

In `internal/judger/loader.go`:
- In `Contest` struct, after `EndTime`:
```go
	SubmitStartTime *time.Time `json:"submit_start_time,omitempty"`
	SubmitEndTime   *time.Time `json:"submit_end_time,omitempty"`
```
- In `Problem` struct, after `EndTime`:
```go
	SubmitStartTime *time.Time `json:"submit_start_time,omitempty"`
	SubmitEndTime   *time.Time `json:"submit_end_time,omitempty"`
```

- [ ] **Step 3: Update `LoadFromDB` + `ContestToModel` in `db.go`**

In `internal/judger/db.go`:
- In `LoadFromDB` contest loop, add after `EndTime: c.EndTime,`:
```go
			SubmitStartTime: c.SubmitStartTime,
			SubmitEndTime:   c.SubmitEndTime,
```
- In `ContestToModel`, add after `EndTime: c.EndTime,`:
```go
		SubmitStartTime: c.SubmitStartTime,
		SubmitEndTime:   c.SubmitEndTime,
```

- [ ] **Step 4: Update `problemFromModel` + `ProblemToModel`**

In `problemFromModel`, add after `EndTime: p.EndTime,`:
```go
		SubmitStartTime: p.SubmitStartTime,
		SubmitEndTime:   p.SubmitEndTime,
```

In `ProblemToModel`, add after `EndTime: p.EndTime,` in the returned `models.Problem`:
```go
		SubmitStartTime: p.SubmitStartTime,
		SubmitEndTime:   p.SubmitEndTime,
```

- [ ] **Step 5: Verify**

Run: `go build ./internal/database/... ./internal/judger/`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
git add internal/database/models/models.go internal/judger/loader.go internal/judger/db.go
git commit -m "feat(db): add SubmitStartTime/SubmitEndTime to Contest + Problem"
```

---

## Task 2: Backend — submitToProblem + getProblem time checks

**Files:**
- Modify: `internal/api/user/problem.go`

- [ ] **Step 1: Update `submitToProblem` time checks**

In `internal/api/user/problem.go`, in the `submitToProblem` handler, find the time-check block (around line 41). Replace it with:

```go
			now := time.Now()
			// Contest visibility check
			if now.Before(parentContest.StartTime) {
				util.Error(c, http.StatusForbidden, fmt.Errorf("contest has not started yet"))
				h.appState.RUnlock()
				return
			}
			if now.After(parentContest.EndTime) {
				util.Error(c, http.StatusForbidden, fmt.Errorf("contest has ended"))
				h.appState.RUnlock()
				return
			}
			// Contest submission window (nil = fall back to visibility)
			contestSubmitStart := parentContest.SubmitStartTime
			if contestSubmitStart == nil {
				contestSubmitStart = &parentContest.StartTime
			}
			contestSubmitEnd := parentContest.SubmitEndTime
			if contestSubmitEnd == nil {
				contestSubmitEnd = &parentContest.EndTime
			}
			if now.Before(*contestSubmitStart) {
				util.Error(c, http.StatusForbidden, fmt.Errorf("contest submission has not opened yet"))
				h.appState.RUnlock()
				return
			}
			if now.After(*contestSubmitEnd) {
				util.Error(c, http.StatusForbidden, fmt.Errorf("contest submission has ended"))
				h.appState.RUnlock()
				return
			}
			// Problem visibility check
			if now.Before(problem.StartTime) {
				util.Error(c, http.StatusForbidden, fmt.Errorf("problem has not started yet"))
				h.appState.RUnlock()
				return
			}
			if now.After(problem.EndTime) {
				util.Error(c, http.StatusForbidden, fmt.Errorf("problem has ended"))
				h.appState.RUnlock()
				return
			}
			// Problem submission window (nil = fall back to visibility)
			probSubmitStart := problem.SubmitStartTime
			if probSubmitStart == nil {
				probSubmitStart = &problem.StartTime
			}
			probSubmitEnd := problem.SubmitEndTime
			if probSubmitEnd == nil {
				probSubmitEnd = &problem.EndTime
			}
			if now.Before(*probSubmitStart) {
				util.Error(c, http.StatusForbidden, fmt.Errorf("problem submission has not opened yet"))
				h.appState.RUnlock()
				return
			}
			if now.After(*probSubmitEnd) {
				util.Error(c, http.StatusForbidden, fmt.Errorf("problem submission has ended"))
				h.appState.RUnlock()
				return
			}
```

- [ ] **Step 2: Add submit times to `ProblemResponse`**

Find the `ProblemResponse` struct in `internal/api/user/problem.go`. Add:
```go
	SubmitStartTime  *time.Time `json:"submit_start_time,omitempty"`
	SubmitEndTime    *time.Time `json:"submit_end_time,omitempty"`
```

In the response construction, add:
```go
		SubmitStartTime: problem.SubmitStartTime,
		SubmitEndTime:   problem.SubmitEndTime,
```

- [ ] **Step 3: Verify**

Run: `go build ./...`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/api/user/problem.go
git commit -m "feat(api): submitToProblem checks visibility + submission windows"
```

---

## Task 3: Frontend — types + contest form + problem form

**Files:**
- Modify: `frontend/lib/types.ts`
- Modify: `frontend/components/admin/contest-actions.tsx`
- Modify: `frontend/components/admin/problem-actions.tsx`

- [ ] **Step 1: Update types**

In `frontend/lib/types.ts`:
- Add to `Contest` interface: `submit_start_time?: string | null;` + `submit_end_time?: string | null;`
- Add to `Problem` interface: `submit_start_time?: string | null;` + `submit_end_time?: string | null;`

- [ ] **Step 2: Update contest form**

In `frontend/components/admin/contest-actions.tsx`:
- Add `submit_start_time` and `submit_end_time` to the zod schema (both `z.string().optional()`).
- Add to form defaults: `submit_start_time: contest?.submit_start_time ? format(new Date(contest.submit_start_time), "yyyy-MM-dd'T'HH:mm") : ''` (same for end).
- Add to form reset (same pattern).
- On submit: `submit_start_time: values.submit_start_time ? new Date(values.submit_start_time).toISOString() : null` (same for end).
- Replace the single "Start Time" + "End Time" pair with a grouped layout:
```tsx
<div className="border p-4 rounded-md space-y-4">
    <h3 className="font-semibold">Time Windows</h3>
    <div className="grid grid-cols-2 gap-4">
        <FormField control={form.control} name="starttime" render={({ field }) => (<FormItem><FormLabel>Visible Start</FormLabel><FormControl><Input type="datetime-local" {...field} /></FormControl><FormMessage /></FormItem>)} />
        <FormField control={form.control} name="endtime" render={({ field }) => (<FormItem><FormLabel>Visible End</FormLabel><FormControl><Input type="datetime-local" {...field} /></FormControl><FormMessage /></FormItem>)} />
    </div>
    <p className="text-xs text-muted-foreground">Submission window (optional — leave empty to use the same as visibility):</p>
    <div className="grid grid-cols-2 gap-4">
        <FormField control={form.control} name="submit_start_time" render={({ field }) => (<FormItem><FormLabel>Submit Start (optional)</FormLabel><FormControl><Input type="datetime-local" {...field} /></FormControl><FormMessage /></FormItem>)} />
        <FormField control={form.control} name="submit_end_time" render={({ field }) => (<FormItem><FormLabel>Submit End (optional)</FormLabel><FormControl><Input type="datetime-local" {...field} /></FormControl><FormMessage /></FormItem>)} />
    </div>
</div>
```

- [ ] **Step 3: Update problem form**

In `frontend/components/admin/problem-actions.tsx`:
- Same changes as the contest form: add `submit_start_time` + `submit_end_time` to the zod schema, form defaults, form reset, and submit serialization.
- Replace the "Start Time" + "End Time" pair with the same grouped layout.

- [ ] **Step 4: Verify**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 5: Commit**

```bash
git add frontend/lib/types.ts frontend/components/admin/contest-actions.tsx frontend/components/admin/problem-actions.tsx
git commit -m "feat(frontend): submit time window fields in contest + problem forms"
```

---

## Task 4: Frontend — user contest list + problem detail status

**Files:**
- Modify: `frontend/app/(main)/contests/page.tsx`
- Modify: `frontend/app/(main)/problems/page.tsx`

- [ ] **Step 1: Update contest list status logic**

In `frontend/app/(main)/contests/page.tsx`, the `ContestCard` component computes `statusText` from `hasStarted`/`hasEnded`. Update it to also check the submission window:

- If `contest.submit_start_time` is non-null and `now < new Date(contest.submit_start_time)` → status = "submission opens at {time}" (or a localized "pending submission" label).
- If `contest.submit_end_time` is non-null and `now > new Date(contest.submit_end_time)` but `now <= new Date(contest.endtime)` → status = "submission closed".
- `canRegister` stays based on visibility times (starttime ≤ now ≤ endtime).

For the contest detail view (`ContestDetailView`), same logic for the status display.

- [ ] **Step 2: Update problem detail submit button**

In `frontend/app/(main)/problems/page.tsx`:
- Read `problem.submit_start_time` and `problem.submit_end_time`.
- If `submit_start_time` is non-null and `now < new Date(submit_start_time)` → show "Submission opens at {time}" + disable the submit form.
- If `submit_end_time` is non-null and `now > new Date(submit_end_time)` → show "Submission has ended" + disable.
- Otherwise → normal submit form.

- [ ] **Step 3: Verify**

Run: `cd frontend && pnpm build`
Expected: succeeds, 19 pages.

- [ ] **Step 4: Commit**

```bash
git add "frontend/app/(main)/contests/page.tsx" "frontend/app/(main)/problems/page.tsx"
git commit -m "feat(frontend): submission window status on user pages"
```

---

## Task 5: Build + verify

- [ ] **Step 1: Full build**

Run: `go build ./... && go vet ./... && cd frontend && pnpm build`
Expected: Go clean, 19 pages.

Run: `make build`
Expected: binary produced.

- [ ] **Step 2: Manual smoke test**

Create a contest with `submit_start_time` set to 1 hour in the future. Verify the contest is visible but the submit button is disabled with "submission opens at...". Set `submit_start_time` to null → submit works normally.

- [ ] **Step 3: No commit**

---

## Self-Review notes

- The `*time.Time` fields are nullable: `nil` in Go = `null` in JSON = "not set" in the frontend form. The backend's nil-check (`if submitStart == nil { submitStart = &contest.StartTime }`) handles the fallback.
- The zod schema uses `z.string().optional()` for the submit time fields — an empty string in the form serializes to `null` on submit. The backend accepts `null` (Go `*time.Time` unmarshals `null` to `nil`).
- `datetime-local` inputs return local time; `new Date(...).toISOString()` converts to UTC for the backend. This matches the existing `starttime`/`endtime` pattern.
- The admin detail pages already show `starttime`/`endtime` — they should also show `submit_start_time`/`submit_end_time` if non-null. This is a minor display addition in the admin contest/problem detail views.
- `registerForContest` checks visibility times (`StartTime`/`EndTime`), NOT submission times. This is correct — registration opens when the contest becomes visible, not when submission opens.
- The `getProblem` endpoint relaxes its visibility check: currently it blocks on `contest.StartTime` + `problem.StartTime`. The spec says to relax this — the problem is visible once its own `StartTime` has passed, regardless of the submission window. However, the current `getProblem` only blocks on `contest.StartTime` + `problem.StartTime`, NOT on submission times. So the relaxation is already the current behavior for visibility; the new check is only in `submitToProblem`. No change needed to `getProblem`'s visibility logic — it already returns the problem when visible. The only addition is returning `submit_start_time`/`submit_end_time` in the response.
