// Package devpods is CSOJ's thin client for the devpod.io/v1alpha1
// DevPod and User custom resources. It uses the dynamic client so CSOJ
// does not take a build-time dependency on the devpods Go types.
package devpods

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/kubeutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	gvrDevPod = schema.GroupVersionResource{Group: "devpod.io", Version: "v1alpha1", Resource: "devpods"}

	// devpods requires ^[a-z0-9-]{1,32}$ and no '+' (the SSH separator).
	ownerRE = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
)

const (
	// Namespace is watched by the devpods controller and is intentionally
	// independent from a CSOJ judger cluster's namespace.
	Namespace     = "devpods"
	labelOwner    = "devpod.io/owner"
	labelTemplate = "csoj.io/template"
	// devpods CEL: len(metadata.name) <= 22
	maxDevPodNameLen = 22
	randSuffixLen    = 4
)

// OwnerName returns the name of the pre-provisioned devpods User associated
// with a CSOJ username/student ID. CSOJ only creates DevPod resources; User
// resources are managed externally.
func OwnerName(username string) string {
	return "h" + username
}

// ValidOwner reports whether a username is acceptable as a devpod owner.
func ValidOwner(username string) bool {
	if strings.Contains(username, "+") {
		return false
	}
	return ownerRE.MatchString(username)
}

// CheckNameBudget returns an error if <owner>-<tplID><rand4> would
// exceed the 22-char DevPod name budget.
func CheckNameBudget(owner, tplID string) error {
	need := len(owner) + 1 + len(tplID) + randSuffixLen
	if need > maxDevPodNameLen {
		return fmt.Errorf("owner %q + template %q too long for devpod naming (%d > %d)", owner, tplID, need, maxDevPodNameLen)
	}
	return nil
}

// Client talks to DevPod CRs in the dedicated devpods namespace of a cluster.
type Client struct {
	dyn dynamic.Interface
}

func NewClient(dyn dynamic.Interface) *Client {
	return &Client{dyn: dyn}
}

// RenderDevPod builds an unstructured DevPod CR. podName must already be
// the final resource name (<owner>-<tplID><rand4>).
func RenderDevPod(owner, podName string, tpl *models.DevPodTemplate) (*unstructured.Unstructured, error) {
	if err := CheckNameBudget(owner, tpl.ID); err != nil {
		return nil, err
	}
	resources := map[string]interface{}{
		"cpu":    fmt.Sprintf("%d", tpl.Cores),
		"memory": fmt.Sprintf("%d", tpl.Memory),
	}
	if tpl.GPUCount < 0 {
		return nil, fmt.Errorf("gpu_count must be non-negative")
	}
	if tpl.GPUCount > 0 {
		gpuResource, err := kubeutil.NormalizeGPUResourceName(tpl.GPUResource)
		if err != nil {
			return nil, err
		}
		resources[gpuResource] = strconv.Itoa(tpl.GPUCount)
	}
	container := map[string]interface{}{
		"name":      "dev",
		"image":     tpl.Image,
		"resources": map[string]interface{}{"requests": resources, "limits": resources},
	}
	podSpec := map[string]interface{}{
		"containers":   []interface{}{container},
		"nodeSelector": map[string]interface{}(tpl.NodeSelector),
	}
	if len(tpl.Tolerations) > 0 {
		var tol []interface{}
		if err := json.Unmarshal(tpl.Tolerations, &tol); err != nil {
			return nil, fmt.Errorf("parse tolerations: %w", err)
		}
		podSpec["tolerations"] = tol
	}
	spec := map[string]interface{}{
		"owner":   owner,
		"running": true,
		"pod":     map[string]interface{}{"spec": podSpec},
	}
	if tpl.Shell != "" {
		spec["shell"] = tpl.Shell
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "devpod.io/v1alpha1",
		"kind":       "DevPod",
		"metadata": map[string]interface{}{
			"name":      podName,
			"namespace": "", // set by Client on create
			"labels": map[string]interface{}{
				labelOwner:    owner,
				labelTemplate: tpl.ID,
			},
		},
		"spec": spec,
	}}
	return u, nil
}

// GenerateDevPodName builds <owner>-<tplID><rand4> using a
// caller-supplied random source (so it is deterministic in tests).
func GenerateDevPodName(owner, tplID string, rand4 string) (string, error) {
	if err := CheckNameBudget(owner, tplID); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s%s", owner, tplID, rand4), nil
}

// CreateDevPod creates the DevPod CR in Namespace.
func (c *Client) CreateDevPod(ctx context.Context, u *unstructured.Unstructured) error {
	unstructured.SetNestedField(u.Object, Namespace, "metadata", "namespace")
	_, err := c.dyn.Resource(gvrDevPod).Namespace(Namespace).Create(ctx, u, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create devpod %q: %w", u.GetName(), err)
	}
	return nil
}

// GetDevPod returns one DevPod by name.
func (c *Client) GetDevPod(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return c.dyn.Resource(gvrDevPod).Namespace(Namespace).Get(ctx, name, metav1.GetOptions{})
}

// ListDevPods lists DevPods filtered by owner and/or template label.
// Either label value may be "" to skip it.
func (c *Client) ListDevPods(ctx context.Context, owner, templateID string) ([]*unstructured.Unstructured, error) {
	labels := []string{}
	if owner != "" {
		labels = append(labels, fmt.Sprintf("%s=%s", labelOwner, owner))
	}
	if templateID != "" {
		labels = append(labels, fmt.Sprintf("%s=%s", labelTemplate, templateID))
	}
	list, err := c.dyn.Resource(gvrDevPod).Namespace(Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: strings.Join(labels, ","),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, &list.Items[i])
	}
	return out, nil
}

// PatchRunning flips spec.running. Uses a JSON merge patch.
func (c *Client) PatchRunning(ctx context.Context, name string, running bool) error {
	patch := fmt.Sprintf(`{"spec":{"running":%t}}`, running)
	_, err := c.dyn.Resource(gvrDevPod).Namespace(Namespace).Patch(ctx, name, "application/merge-patch+json", []byte(patch), metav1.PatchOptions{})
	return err
}

// DeleteDevPod deletes a DevPod CR.
func (c *Client) DeleteDevPod(ctx context.Context, name string) error {
	err := c.dyn.Resource(gvrDevPod).Namespace(Namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// Phase extracts spec phase from a DevPod CR ("" if unset).
func Phase(u *unstructured.Unstructured) string {
	s, _, _ := unstructured.NestedString(u.Object, "status", "phase")
	return s
}

// Endpoint extracts status.endpoint.
func Endpoint(u *unstructured.Unstructured) string {
	s, _, _ := unstructured.NestedString(u.Object, "status", "endpoint")
	return s
}

// Running extracts the desired running state from spec.running.
func Running(u *unstructured.Unstructured) bool {
	running, _, _ := unstructured.NestedBool(u.Object, "spec", "running")
	return running
}

// Message extracts status.message (empty when the controller has no message).
func Message(u *unstructured.Unstructured) string {
	message, _, _ := unstructured.NestedString(u.Object, "status", "message")
	return message
}

// CreatedAt extracts metadata.creationTimestamp as a Go time.
func CreatedAt(u *unstructured.Unstructured) time.Time {
	ts, _, _ := unstructured.NestedString(u.Object, "metadata", "creationTimestamp")
	t, _ := time.Parse(time.RFC3339, ts)
	return t
}
