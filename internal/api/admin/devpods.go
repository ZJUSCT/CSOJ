package admin

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/devpods"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type adminDevPodInstance struct {
	Name        string    `json:"name"`
	Owner       string    `json:"owner"`
	Template    string    `json:"template"`
	ClusterName string    `json:"cluster_name"`
	Namespace   string    `json:"namespace"`
	Phase       string    `json:"phase"`
	Running     bool      `json:"running"`
	Endpoint    string    `json:"endpoint"`
	Message     string    `json:"message"`
	CreatedAt   time.Time `json:"created_at"`
}

// listAllDevPods returns a live cross-cluster view of every DevPod CR.
func (h *Handler) listAllDevPods(c *gin.Context) {
	items := make([]adminDevPodInstance, 0)
	warnings := make([]string, 0)
	for _, clusterName := range h.scheduler.GetClusterNames() {
		dyn, _, err := h.scheduler.DynamicClientForCluster(clusterName)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("cluster %s: %v", clusterName, err))
			continue
		}
		client := devpods.NewClient(dyn)
		clusterItems, err := client.ListDevPods(c.Request.Context(), "", "")
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("cluster %s: %v", clusterName, err))
			continue
		}
		for _, item := range clusterItems {
			items = append(items, adminDevPodInstance{
				Name:        item.GetName(),
				Owner:       item.GetLabels()["devpod.io/owner"],
				Template:    item.GetLabels()["csoj.io/template"],
				ClusterName: clusterName,
				Namespace:   devpods.Namespace,
				Phase:       devpods.Phase(item),
				Running:     devpods.Running(item),
				Endpoint:    devpods.Endpoint(item),
				Message:     devpods.Message(item),
				CreatedAt:   devpods.CreatedAt(item),
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			if items[i].ClusterName == items[j].ClusterName {
				return items[i].Name < items[j].Name
			}
			return items[i].ClusterName < items[j].ClusterName
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	util.Success(c, gin.H{"items": items, "warnings": warnings}, "DevPods retrieved")
}

// requireAdminDevPod resolves a DevPod in the explicitly selected cluster.
// Cluster is part of the resource identity because different clusters may
// contain DevPods with the same name.
func (h *Handler) requireAdminDevPod(c *gin.Context) (*unstructured.Unstructured, *devpods.Client, bool) {
	clusterName := c.Param("cluster")
	name := c.Param("name")
	dyn, _, err := h.scheduler.DynamicClientForCluster(clusterName)
	if err != nil {
		util.Error(c, http.StatusBadGateway, err)
		return nil, nil, false
	}
	client := devpods.NewClient(dyn)
	u, err := client.GetDevPod(c.Request.Context(), name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			util.Error(c, http.StatusNotFound, fmt.Sprintf("DevPod %q not found in cluster %q", name, clusterName))
		} else {
			util.Error(c, http.StatusBadGateway, fmt.Errorf("get DevPod %q from cluster %q: %w", name, clusterName, err))
		}
		return nil, nil, false
	}
	return u, client, true
}

func (h *Handler) startAdminDevPod(c *gin.Context) {
	u, client, ok := h.requireAdminDevPod(c)
	if !ok {
		return
	}
	if devpods.Running(u) {
		util.Success(c, gin.H{"cluster_name": c.Param("cluster"), "name": c.Param("name"), "running": true}, "DevPod already starting or running")
		return
	}
	var maxPerUser int
	if err := h.settings.Get("devpods.max_per_user", &maxPerUser); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("read DevPod user running limit: %w", err))
		return
	}
	if maxPerUser > 0 {
		clients := []*devpods.Client{client}
		seen := map[string]struct{}{c.Param("cluster"): {}}
		templates, err := database.ListDevPodTemplates(h.db)
		if err != nil {
			util.Error(c, http.StatusInternalServerError, fmt.Errorf("list DevPod templates for running quota: %w", err))
			return
		}
		for _, template := range templates {
			if _, ok := seen[template.ClusterName]; ok {
				continue
			}
			seen[template.ClusterName] = struct{}{}
			dyn, _, err := h.scheduler.DynamicClientForCluster(template.ClusterName)
			if err != nil {
				util.Error(c, http.StatusBadGateway, err)
				return
			}
			clients = append(clients, devpods.NewClient(dyn))
		}
		if err := devpods.CheckUserRunningQuota(c.Request.Context(), clients, u.GetLabels()["devpod.io/owner"], maxPerUser); err != nil {
			if qe, ok := err.(*devpods.QuotaError); ok {
				util.Error(c, http.StatusConflict, qe)
				return
			}
			util.Error(c, http.StatusBadGateway, err)
			return
		}
	}
	tpl, err := database.GetDevPodTemplate(h.db, u.GetLabels()["csoj.io/template"])
	if err == nil {
		if err := devpods.CheckGlobalRunningQuota(c.Request.Context(), client, tpl.ID, tpl.DefaultGlobal, u.GetName()); err != nil {
			if qe, ok := err.(*devpods.QuotaError); ok {
				util.Error(c, http.StatusConflict, qe)
				return
			}
			util.Error(c, http.StatusBadGateway, err)
			return
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("read DevPod template for running quota: %w", err))
		return
	}
	if err := client.PatchRunning(c.Request.Context(), c.Param("name"), true); err != nil {
		util.Error(c, http.StatusBadGateway, fmt.Errorf("start DevPod %q in cluster %q: %w", c.Param("name"), c.Param("cluster"), err))
		return
	}
	util.Success(c, gin.H{"cluster_name": c.Param("cluster"), "name": c.Param("name"), "running": true}, "DevPod starting")
}

func (h *Handler) listAdminDevPodEvents(c *gin.Context) {
	u, client, ok := h.requireAdminDevPod(c)
	if !ok {
		return
	}
	name := c.Param("name")
	events, err := client.ListEvents(c.Request.Context(), name)
	if err != nil {
		util.Error(c, http.StatusBadGateway, err)
		return
	}
	warnings := make([]string, 0)
	gateway, err := devpods.GetGateway(h.settings)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("read DevPod gateway audit setting: %v", err))
	} else {
		kube, err := h.scheduler.KubernetesClientForCluster(c.Param("cluster"))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("read DevPod gateway audit client: %v", err))
		} else {
			auditEvents, auditWarnings := devpods.ListGatewayAuditEvents(
				c.Request.Context(), kube, gateway.EffectiveAuditNamespace(), name, u.GetLabels()["devpod.io/owner"],
			)
			events = append(events, auditEvents...)
			warnings = append(warnings, auditWarnings...)
		}
	}
	util.Success(c, gin.H{"items": devpods.SortEvents(events), "warnings": warnings}, "DevPod events retrieved")
}

func (h *Handler) stopAdminDevPod(c *gin.Context) {
	_, client, ok := h.requireAdminDevPod(c)
	if !ok {
		return
	}
	if err := client.PatchRunning(c.Request.Context(), c.Param("name"), false); err != nil {
		util.Error(c, http.StatusBadGateway, fmt.Errorf("stop DevPod %q in cluster %q: %w", c.Param("name"), c.Param("cluster"), err))
		return
	}
	util.Success(c, gin.H{"cluster_name": c.Param("cluster"), "name": c.Param("name"), "running": false}, "DevPod stopping")
}

func (h *Handler) deleteAdminDevPod(c *gin.Context) {
	_, client, ok := h.requireAdminDevPod(c)
	if !ok {
		return
	}
	if err := client.DeleteDevPod(c.Request.Context(), c.Param("name")); err != nil {
		util.Error(c, http.StatusBadGateway, fmt.Errorf("delete DevPod %q from cluster %q: %w", c.Param("name"), c.Param("cluster"), err))
		return
	}
	util.Success(c, gin.H{"cluster_name": c.Param("cluster"), "name": c.Param("name")}, "DevPod deleted")
}
