package devpods

import (
	"context"
	"fmt"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
)

// QuotaError reports which limit was hit and the current count.
type QuotaError struct {
	Scope string // "user_running" | "per_user" | "global_running"
	Used  int
	Limit int
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("%s quota reached (%d/%d)", e.Scope, e.Used, e.Limit)
}

// CheckUserRunningQuota enforces one global per-user running limit across
// every supplied DevPod cluster and template. A limit of zero disables it.
func CheckUserRunningQuota(ctx context.Context, clients []*Client, owner string, limit int) error {
	if limit <= 0 {
		return nil
	}
	used := 0
	for _, c := range clients {
		items, err := c.ListDevPods(ctx, owner, "")
		if err != nil {
			return fmt.Errorf("count global per-user running quota: %w", err)
		}
		for _, item := range items {
			if Running(item) {
				used++
			}
		}
	}
	if used >= limit {
		return &QuotaError{Scope: "user_running", Used: used, Limit: limit}
	}
	return nil
}

// CheckQuota returns nil if the user may create and start one more DevPod of
// the given template; otherwise a *QuotaError. Stopped DevPods still count
// against the per-user creation limit, but do not consume the global running
// quota.
func CheckQuota(ctx context.Context, c *Client, owner string, tpl *models.DevPodTemplate) error {
	perUser := tpl.DefaultPerUser

	if perUser > 0 {
		mine, err := c.ListDevPods(ctx, owner, tpl.ID)
		if err != nil {
			return fmt.Errorf("count per-user: %w", err)
		}
		if len(mine) >= perUser {
			return &QuotaError{Scope: "per_user", Used: len(mine), Limit: perUser}
		}
	}
	return CheckGlobalRunningQuota(ctx, c, tpl.ID, tpl.DefaultGlobal, "")
}

// CheckGlobalRunningQuota enforces the template-wide running limit. It counts
// desired state (spec.running), so DevPods reserve a slot while starting and
// release it as soon as they are stopped. excludeName is used when starting an
// existing DevPod so an idempotent start does not count the target twice.
func CheckGlobalRunningQuota(ctx context.Context, c *Client, templateID string, limit int, excludeName string) error {
	if limit <= 0 {
		return nil
	}
	all, err := c.ListDevPods(ctx, "", templateID)
	if err != nil {
		return fmt.Errorf("count global running quota: %w", err)
	}
	used := 0
	for _, item := range all {
		if item.GetName() != excludeName && Running(item) {
			used++
		}
	}
	if used >= limit {
		return &QuotaError{Scope: "global_running", Used: used, Limit: limit}
	}
	return nil
}
