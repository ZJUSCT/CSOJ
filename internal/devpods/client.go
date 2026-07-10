// Package devpods is CSOJ's thin client for the devpod.io/v1alpha1
// DevPod and User custom resources. It uses the dynamic client so CSOJ
// does not take a build-time dependency on the devpods Go types.
package devpods

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	gvrDevPod = schema.GroupVersionResource{Group: "devpod.io", Version: "v1alpha1", Resource: "devpods"}
	gvrUser   = schema.GroupVersionResource{Group: "devpod.io", Version: "v1alpha1", Resource: "users"}

	// devpods requires ^[a-z0-9-]{1,32}$ and no '+' (the SSH separator).
	ownerRE = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
)

const (
	labelOwner    = "devpod.io/owner"
	labelTemplate = "csoj.io/template"
	// devpods CEL: len(metadata.name) <= 22
	maxDevPodNameLen = 22
	randSuffixLen    = 4
)

// ValidOwner reports whether a username is acceptable as a devpod owner.
func ValidOwner(username string) bool {
	if strings.Contains(username, "+") {
		return false
	}
	return ownerRE.MatchString(username)
}

// CheckNameBudget returns an error if <username>-<tplID>-<rand4> would
// exceed the 22-char DevPod name budget.
func CheckNameBudget(username, tplID string) error {
	need := len(username) + 1 + len(tplID) + 1 + randSuffixLen
	if need > maxDevPodNameLen {
		return fmt.Errorf("username %q + template %q too long for devpod naming (%d > %d)", username, tplID, need, maxDevPodNameLen)
	}
	return nil
}

// Client talks to devpods CRDs in one cluster/namespace.
type Client struct {
	dyn dynamic.Interface
	ns  string
}

func NewClient(dyn dynamic.Interface, namespace string) *Client {
	return &Client{dyn: dyn, ns: namespace}
}

// RenderDevPod builds an unstructured DevPod CR. podName must already be
// the final resource name (<username>-<tplID>-<rand4>).
func RenderDevPod(owner, podName string, tpl *models.DevPodTemplate) (*unstructured.Unstructured, error) {
	if err := CheckNameBudget(owner, tpl.ID); err != nil {
		return nil, err
	}
	resources := map[string]interface{}{
		"cpu":    fmt.Sprintf("%d", tpl.Cores),
		"memory": fmt.Sprintf("%d", tpl.Memory),
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
	if tpl.PersistenceSize != "" {
		spec["persistence"] = map[string]interface{}{
			"size":      tpl.PersistenceSize,
			"mountPath": "/home/" + owner,
		}
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

// GenerateDevPodName builds <username>-<tplID>-<rand4> using a
// caller-supplied random source (so it is deterministic in tests).
func GenerateDevPodName(username, tplID string, rand4 string) (string, error) {
	if err := CheckNameBudget(username, tplID); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%s", username, tplID, rand4), nil
}

// EnsureUser idempotently creates an empty-pubkeys devpods User CR.
// devpods authenticates via LDAP; the empty User CR exists only so the
// owner name resolves. A pre-existing User is left untouched.
func (c *Client) EnsureUser(ctx context.Context, username string) error {
	usr := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "devpod.io/v1alpha1",
		"kind":       "User",
		"metadata":   map[string]interface{}{"name": username},
		"spec":       map[string]interface{}{"pubkeys": []interface{}{}},
	}}
	_, err := c.dyn.Resource(gvrUser).Create(ctx, usr, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return fmt.Errorf("ensure user %q: %w", username, err)
}

// CreateDevPod creates the DevPod CR (namespace taken from the Client).
func (c *Client) CreateDevPod(ctx context.Context, u *unstructured.Unstructured) error {
	unstructured.SetNestedField(u.Object, c.ns, "metadata", "namespace")
	_, err := c.dyn.Resource(gvrDevPod).Namespace(c.ns).Create(ctx, u, metav1.CreateOptions{})
	return err
}

// GetDevPod returns one DevPod by name.
func (c *Client) GetDevPod(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return c.dyn.Resource(gvrDevPod).Namespace(c.ns).Get(ctx, name, metav1.GetOptions{})
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
	list, err := c.dyn.Resource(gvrDevPod).Namespace(c.ns).List(ctx, metav1.ListOptions{
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
	_, err := c.dyn.Resource(gvrDevPod).Namespace(c.ns).Patch(ctx, name, "application/merge-patch+json", []byte(patch), metav1.PatchOptions{})
	return err
}

// DeleteDevPod deletes a DevPod CR.
func (c *Client) DeleteDevPod(ctx context.Context, name string) error {
	err := c.dyn.Resource(gvrDevPod).Namespace(c.ns).Delete(ctx, name, metav1.DeleteOptions{})
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

// CreatedAt extracts metadata.creationTimestamp as a Go time.
func CreatedAt(u *unstructured.Unstructured) time.Time {
	ts, _, _ := unstructured.NestedString(u.Object, "metadata", "creationTimestamp")
	t, _ := time.Parse(time.RFC3339, ts)
	return t
}
