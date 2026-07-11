package user

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/devpods"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// devpodInstance is the JSON shape returned to the frontend.
type devpodInstance struct {
	Name       string    `json:"name"`
	Template   string    `json:"template"`
	Phase      string    `json:"phase"`
	Running    bool      `json:"running"`
	Endpoint   string    `json:"endpoint"`
	SSHCommand string    `json:"ssh_command"`
	CreatedAt  time.Time `json:"created_at"`
}

// devpodListResponse wraps the list + the gateway (so the frontend can
// build ssh commands without a second round-trip).
type devpodListResponse struct {
	Items      []devpodInstance `json:"items"`
	MaxPerUser int              `json:"max_per_user"`
	Gateway    struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"gateway"`
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Handler) listDevPodTemplates(c *gin.Context) {
	user, err := database.GetUserByID(h.db, c.GetString("userID"))
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	rows, err := database.ListDevPodTemplates(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	visible := make([]models.DevPodTemplate, 0, len(rows))
	for _, tpl := range rows {
		if devpods.TemplateAllowedForUser(user.Tags, tpl.AllowedTags) {
			visible = append(visible, tpl)
		}
	}
	util.Success(c, visible, "Templates retrieved")
}

func (h *Handler) listDevPods(c *gin.Context) {
	userID := c.GetString("userID")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	owner := devpods.OwnerName(user.Username)
	var maxPerUser int
	if err := h.settings.Get("devpods.max_per_user", &maxPerUser); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("read devpod user limit: %w", err))
		return
	}
	gw, err := devpods.GetGateway(h.settings)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("read gateway setting: %w", err))
		return
	}

	// DevPods may live in any registered cluster; list across all clusters and filter by owner label.
	resp := devpodListResponse{Items: []devpodInstance{}}
	resp.MaxPerUser = maxPerUser
	resp.Gateway.Host = gw.Host
	resp.Gateway.Port = gw.Port

	seen := map[string]bool{}
	for _, cl := range h.scheduler.GetClusterNames() {
		dyn, _, err := h.scheduler.DynamicClientForCluster(cl)
		if err != nil {
			continue
		}
		cli := devpods.NewClient(dyn)
		items, err := cli.ListDevPods(c.Request.Context(), owner, "")
		if err != nil {
			continue
		}
		for _, u := range items {
			name := u.GetName()
			if seen[name] {
				continue
			}
			seen[name] = true
			tpl := u.GetLabels()["csoj.io/template"]
			inst := devpodInstance{
				Name:      name,
				Template:  tpl,
				Phase:     devpods.Phase(u),
				Running:   devpods.Running(u),
				Endpoint:  devpods.Endpoint(u),
				CreatedAt: devpods.CreatedAt(u),
			}
			if inst.Phase == "Running" && inst.Endpoint != "" {
				inst.SSHCommand = gw.SSHCommand(owner, name)
			}
			resp.Items = append(resp.Items, inst)
		}
	}
	util.Success(c, resp, "DevPods retrieved")
}

func (h *Handler) createDevPod(c *gin.Context) {
	userID := c.GetString("userID")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	owner := devpods.OwnerName(user.Username)
	if !devpods.ValidOwner(owner) {
		util.Error(c, http.StatusBadRequest, "devpod owner not valid for naming (h + username must match [a-z0-9-]{1,32}, no '+')")
		return
	}
	var req struct {
		TemplateID string `json:"template_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	tpl, err := database.GetDevPodTemplate(h.db, req.TemplateID)
	if err != nil {
		util.Error(c, http.StatusNotFound, "template not found")
		return
	}
	if !devpods.TemplateAllowedForUser(user.Tags, tpl.AllowedTags) {
		util.Error(c, http.StatusForbidden, "your user tags do not allow access to this DevPod template")
		return
	}
	if err := devpods.CheckNameBudget(owner, tpl.ID); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	dyn, _, err := h.scheduler.DynamicClientForCluster(tpl.ClusterName)
	if err != nil {
		util.Error(c, http.StatusBadGateway, fmt.Errorf("cluster %q not loaded: %w", tpl.ClusterName, err))
		return
	}
	cli := devpods.NewClient(dyn)
	var maxPerUser int
	if err := h.settings.Get("devpods.max_per_user", &maxPerUser); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("read devpod user limit: %w", err))
		return
	}
	if maxPerUser > 0 {
		clients, err := h.devPodQuotaClients()
		if err != nil {
			util.Error(c, http.StatusBadGateway, err)
			return
		}
		if err := devpods.CheckUserRunningQuota(c.Request.Context(), clients, owner, maxPerUser); err != nil {
			if qe, ok := err.(*devpods.QuotaError); ok {
				util.Error(c, http.StatusConflict, qe)
				return
			}
			util.Error(c, http.StatusInternalServerError, err)
			return
		}
	}

	if err := devpods.CheckQuota(c.Request.Context(), cli, owner, tpl); err != nil {
		if qe, ok := err.(*devpods.QuotaError); ok {
			util.Error(c, http.StatusConflict, qe)
			return
		}
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	podName, err := devpods.GenerateDevPodName(owner, tpl.ID, randHex(2))
	if err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	u, err := devpods.RenderDevPod(owner, podName, tpl)
	if err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if err := cli.CreateDevPod(c.Request.Context(), u); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("create devpod: %w", err))
		return
	}
	util.Success(c, gin.H{"name": podName}, "DevPod created")
}

// devPodQuotaClients returns one client for each distinct cluster referenced
// by a DevPod template. These are the clusters that contribute to the global
// per-user running DevPod count.
func (h *Handler) devPodQuotaClients() ([]*devpods.Client, error) {
	templates, err := database.ListDevPodTemplates(h.db)
	if err != nil {
		return nil, fmt.Errorf("list devpod templates for quota: %w", err)
	}
	seen := make(map[string]struct{}, len(templates))
	clients := make([]*devpods.Client, 0, len(templates))
	for _, tpl := range templates {
		if _, ok := seen[tpl.ClusterName]; ok {
			continue
		}
		seen[tpl.ClusterName] = struct{}{}
		dyn, _, err := h.scheduler.DynamicClientForCluster(tpl.ClusterName)
		if err != nil {
			return nil, fmt.Errorf("cluster %q not loaded while checking devpod quota: %w", tpl.ClusterName, err)
		}
		clients = append(clients, devpods.NewClient(dyn))
	}
	return clients, nil
}

// requireOwnedDevPod fetches the DevPod and verifies the owner label
// matches h + the caller's username. Returns the CR, the client, the loaded
// user, and true on success; on error it has already responded and returns
// false.
func (h *Handler) requireOwnedDevPod(c *gin.Context, name string) (*unstructured.Unstructured, *devpods.Client, *models.User, bool) {
	userID := c.GetString("userID")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return nil, nil, nil, false
	}
	ownerName := devpods.OwnerName(user.Username)
	// search every cluster for the named DevPod
	for _, cl := range h.scheduler.GetClusterNames() {
		dyn, _, err := h.scheduler.DynamicClientForCluster(cl)
		if err != nil {
			continue
		}
		cli := devpods.NewClient(dyn)
		u, err := cli.GetDevPod(c.Request.Context(), name)
		if err != nil {
			continue
		}
		owner := u.GetLabels()["devpod.io/owner"]
		if owner != ownerName {
			util.Error(c, http.StatusForbidden, "not your devpod")
			return nil, nil, nil, false
		}
		return u, cli, user, true
	}
	util.Error(c, http.StatusNotFound, "devpod not found")
	return nil, nil, nil, false
}

func (h *Handler) getDevPod(c *gin.Context) {
	name := c.Param("name")
	u, _, user, ok := h.requireOwnedDevPod(c, name)
	if !ok {
		return
	}
	gw, err := devpods.GetGateway(h.settings)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("read gateway setting: %w", err))
		return
	}
	inst := devpodInstance{
		Name:      u.GetName(),
		Template:  u.GetLabels()["csoj.io/template"],
		Phase:     devpods.Phase(u),
		Running:   devpods.Running(u),
		Endpoint:  devpods.Endpoint(u),
		CreatedAt: devpods.CreatedAt(u),
	}
	if inst.Phase == "Running" && inst.Endpoint != "" {
		inst.SSHCommand = gw.SSHCommand(devpods.OwnerName(user.Username), name)
	}
	util.Success(c, inst, "DevPod found")
}

func (h *Handler) startDevPod(c *gin.Context) {
	u, cli, _, ok := h.requireOwnedDevPod(c, c.Param("name"))
	if !ok {
		return
	}
	if devpods.Running(u) {
		util.Success(c, nil, "DevPod already starting or running")
		return
	}
	var maxPerUser int
	if err := h.settings.Get("devpods.max_per_user", &maxPerUser); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("read devpod user running limit: %w", err))
		return
	}
	if maxPerUser > 0 {
		clients, err := h.devPodQuotaClients()
		if err != nil {
			util.Error(c, http.StatusBadGateway, err)
			return
		}
		if err := devpods.CheckUserRunningQuota(c.Request.Context(), clients, u.GetLabels()["devpod.io/owner"], maxPerUser); err != nil {
			if qe, ok := err.(*devpods.QuotaError); ok {
				util.Error(c, http.StatusConflict, qe)
				return
			}
			util.Error(c, http.StatusInternalServerError, err)
			return
		}
	}
	tpl, err := database.GetDevPodTemplate(h.db, u.GetLabels()["csoj.io/template"])
	if err == nil {
		if err := devpods.CheckGlobalRunningQuota(c.Request.Context(), cli, tpl.ID, tpl.DefaultGlobal, u.GetName()); err != nil {
			if qe, ok := err.(*devpods.QuotaError); ok {
				util.Error(c, http.StatusConflict, qe)
				return
			}
			util.Error(c, http.StatusInternalServerError, err)
			return
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("read devpod template for running quota: %w", err))
		return
	}
	if err := cli.PatchRunning(c.Request.Context(), c.Param("name"), true); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "DevPod starting")
}

func (h *Handler) stopDevPod(c *gin.Context) {
	_, cli, _, ok := h.requireOwnedDevPod(c, c.Param("name"))
	if !ok {
		return
	}
	if err := cli.PatchRunning(c.Request.Context(), c.Param("name"), false); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "DevPod stopping")
}

func (h *Handler) deleteDevPod(c *gin.Context) {
	_, cli, _, ok := h.requireOwnedDevPod(c, c.Param("name"))
	if !ok {
		return
	}
	if err := cli.DeleteDevPod(c.Request.Context(), c.Param("name")); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "DevPod deleted")
}
