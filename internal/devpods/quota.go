package devpods

import (
	"context"
	"fmt"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
)

// QuotaError reports which limit was hit and the current count.
type QuotaError struct {
	Scope string // "per_user" | "global"
	Used  int
	Limit int
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("%s quota reached (%d/%d)", e.Scope, e.Used, e.Limit)
}

// CheckQuota returns nil if the user may create one more DevPod of the
// given template; otherwise a *QuotaError.
func CheckQuota(ctx context.Context, c *Client, owner string, tpl *models.DevPodTemplate) error {
	perUser := tpl.DefaultPerUser
	global := tpl.DefaultGlobal

	if perUser > 0 {
		mine, err := c.ListDevPods(ctx, owner, tpl.ID)
		if err != nil {
			return fmt.Errorf("count per-user: %w", err)
		}
		if len(mine) >= perUser {
			return &QuotaError{Scope: "per_user", Used: len(mine), Limit: perUser}
		}
	}
	if global > 0 {
		all, err := c.ListDevPods(ctx, "", tpl.ID)
		if err != nil {
			return fmt.Errorf("count global: %w", err)
		}
		if len(all) >= global {
			return &QuotaError{Scope: "global", Used: len(all), Limit: global}
		}
	}
	return nil
}
