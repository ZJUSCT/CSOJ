# DevPods Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a top-level "DevPods" tab to CSOJ where users create DevPod instances from admin-defined templates (fixed cluster/cpu/memory/NUMA+cpuset), with per-user and global per-template quotas; CSOJ drives the external `devpods` system by creating `DevPod` CRs over the Kubernetes API.

**Architecture:** CSOJ stores only templates (new `DevPodTemplate` table) + a gateway setting. DevPod instances are not mirrored — listings and quota counts are live `List` calls against `DevPod` CRs filtered by label. A new `internal/devpods` package wraps the dynamic client for the `devpod.io/v1alpha1` DevPod+User GVRs. CSOJ reuses the existing eval `Cluster` table's kubeconfig (devpods lives in one of those clusters) via a new `Scheduler.DynamicClientForCluster` accessor. `owner` = the CSOJ **username** (not nickname). SSH auth stays in devpods (LDAP / User CRD); CSOJ lazily creates an empty-pubkeys devpods `User` CR on first DevPod.

**Tech Stack:** Go 1.24, Gin, GORM/SQLite, k8s client-go (dynamic, already a dep), Next.js 14, React 18, TypeScript, shadcn/ui, next-intl.

**Reference spec:** `docs/superpowers/specs/2026-07-10-devpods-integration-design.md`

**Branch:** `devpods-integration`

---

## File map

**Backend — create:**
- `internal/devpods/client.go` — dynamic-client wrapper for DevPod/User GVRs: `RenderDevPod`, `EnsureUser`, `CreateDevPod`, `GetDevPod`, `ListDevPods`, `PatchRunning`, `DeleteDevPod`.
- `internal/devpods/render_test.go` — unit tests for `RenderDevPod` + name budget.
- `internal/devpods/quota.go` — `CheckQuota` (per-user + global) using `ListDevPods`.
- `internal/devpods/quota_test.go` — quota threshold tests.
- `internal/api/user/devpods.go` — user-facing DevPod handlers (list/create/get/start/stop/delete).
- `internal/api/admin/devpod_templates.go` — admin template CRUD handlers.
- `internal/database/devpod_template.go` — DB CRUD for `DevPodTemplate`.

**Backend — modify:**
- `internal/database/models/models.go` — add `DevPodTemplate` struct.
- `internal/database/database.go` — add `&models.DevPodTemplate{}` to `AutoMigrate`.
- `internal/judger/scheduler.go` — add `DynamicClientForCluster(name)` accessor.
- `internal/api/user/router.go` — register `/devpods` routes.
- `internal/api/admin/router.go` — register `/admin/devpod-templates` routes.

**Frontend — create:**
- `frontend/app/(main)/devpods/page.tsx` — user DevPods page.
- `frontend/app/(admin)/admin/devpod-templates/page.tsx` — admin templates page.
- `frontend/components/admin/devpod-template-actions.tsx` — template create/edit/delete dialog + form.
- `frontend/components/devpods/devpod-card.tsx` — template card with create button.
- `frontend/components/devpods/devpod-row.tsx` — table row with ssh command + actions.

**Frontend — modify:**
- `frontend/lib/types.ts` — add `DevPodTemplate`, `DevPodInstance`, `DevPodGateway` types.
- `frontend/lib/api.ts` — (no change needed; existing axios instance reused).
- `frontend/components/layout/main-nav.tsx` — add `/devpods` route.
- `frontend/components/layout/admin-sub-nav.tsx` — add "DevPod Templates" route.
- `frontend/public/messages/en.json` + `zh.json` — i18n keys.

---

## Task 1: `DevPodTemplate` model + DB migration + CRUD

**Files:**
- Modify: `internal/database/models/models.go` (append struct)
- Modify: `internal/database/database.go` (AutoMigrate line)
- Create: `internal/database/devpod_template.go`
- Create: `internal/database/devpod_template_test.go`

- [ ] **Step 1: Add the `DevPodTemplate` struct**

In `internal/database/models/models.go`, append after the `Cluster` struct:

```go
// DevPodTemplate is an admin-defined fixed recipe for a user DevPod.
// The ID is kept short ([a-z0-9-]{1,6}) so the derived DevPod name
// <username>-<id>-<rand4> fits devpods' 22-char name budget.
type DevPodTemplate struct {
	ID              string    `gorm:"primaryKey" json:"id"`
	Name            string    `json:"name"`
	ClusterName     string    `json:"cluster_name"`
	Image           string    `json:"image"`
	Shell           string    `json:"shell"`
	Cores           int       `json:"cores"`
	Memory          int64     `json:"memory"` // bytes
	NodeSelector    JSONMap   `gorm:"type:text" json:"node_selector"`
	Tolerations     RawJSON   `gorm:"type:text" json:"tolerations"`
	DefaultPerUser  int       `gorm:"default:1" json:"default_per_user"`
	DefaultGlobal   int       `gorm:"default:5" json:"default_global"`
	PersistenceSize string    `json:"persistence_size"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
```

- [ ] **Step 2: Register it in AutoMigrate**

In `internal/database/database.go`, inside `Init`'s `db.AutoMigrate(...)` call, add `&models.DevPodTemplate{},` after the `&models.ContestRegistration{},` line.

- [ ] **Step 3: Write the CRUD layer + failing test**

Create `internal/database/devpod_template.go`:

```go
package database

import (
	"regexp"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"gorm.io/gorm"
)

var templateIDRE = regexp.MustCompile(`^[a-z0-9-]{1,6}$`)

// ValidateDevPodTemplateID returns nil if id is a valid template ID.
func ValidateDevPodTemplateID(id string) bool {
	return templateIDRE.MatchString(id)
}

func ListDevPodTemplates(db *gorm.DB) ([]models.DevPodTemplate, error) {
	var rows []models.DevPodTemplate
	err := db.Order("id").Find(&rows).Error
	return rows, err
}

func GetDevPodTemplate(db *gorm.DB, id string) (*models.DevPodTemplate, error) {
	var tpl models.DevPodTemplate
	if err := db.Where("id = ?", id).First(&tpl).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

func CreateDevPodTemplate(db *gorm.DB, tpl *models.DevPodTemplate) error {
	return db.Create(tpl).Error
}

func UpdateDevPodTemplate(db *gorm.DB, tpl *models.DevPodTemplate) error {
	return db.Save(tpl).Error
}

func DeleteDevPodTemplate(db *gorm.DB, id string) error {
	return db.Where("id = ?", id).Delete(&models.DevPodTemplate{}).Error
}
```

Create `internal/database/devpod_template_test.go`:

```go
package database

import (
	"testing"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
)

func TestDevPodTemplate_CRUD(t *testing.T) {
	db, err := database.Init(":memory:")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	tpl := &models.DevPodTemplate{
		ID: "gpu-8c", Name: "8c GPU", ClusterName: "c1", Image: "ubuntu:24.04",
		Cores: 8, Memory: 16 << 30, NodeSelector: models.JSONMap{"numa-node": "0"},
		DefaultPerUser: 1, DefaultGlobal: 5,
	}
	if err := database.CreateDevPodTemplate(db, tpl); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := database.GetDevPodTemplate(db, "gpu-8c")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Cores != 8 {
		t.Errorf("cores = %d, want 8", got.Cores)
	}
	if got.NodeSelector["numa-node"] != "0" {
		t.Errorf("numa selector not round-tripped: %v", got.NodeSelector)
	}
	got.DefaultPerUser = 3
	if err := database.UpdateDevPodTemplate(db, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := database.GetDevPodTemplate(db, "gpu-8c")
	if got2.DefaultPerUser != 3 {
		t.Errorf("update did not persist")
	}
	rows, err := database.ListDevPodTemplates(db)
	if err != nil || len(rows) != 1 {
		t.Errorf("list: %v len=%d", err, len(rows))
	}
	if err := database.DeleteDevPodTemplate(db, "gpu-8c"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := database.GetDevPodTemplate(db, "gpu-8c"); err == nil {
		t.Errorf("delete did not remove row")
	}
}

func TestValidateDevPodTemplateID(t *testing.T) {
	good := []string{"a", "gpu-8c", "x-y-z"}
	bad := []string{"", "GPU", "toolongid", "gpu_8c", "gpu.8c"}
	for _, id := range good {
		if !database.ValidateDevPodTemplateID(id) {
			t.Errorf("expected %q valid", id)
		}
	}
	for _, id := range bad {
		if database.ValidateDevPodTemplateID(id) {
			t.Errorf("expected %q invalid", id)
		}
	}
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./internal/database/ -run TestDevPodTemplate_CRUD -v && go test ./internal/database/ -run TestValidateDevPodTemplateID -v`
Expected: both PASS.

- [ ] **Step 5: Build + vet**

Run: `go build ./... && go vet ./...`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
git add internal/database/models/models.go internal/database/database.go internal/database/devpod_template.go internal/database/devpod_template_test.go
git commit -m "feat(db): add DevPodTemplate model + CRUD"
```

---

## Task 2: `devpods.gateway` setting + accessor

**Files:**
- Create: `internal/devpods/gateway.go`
- Create: `internal/devpods/gateway_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/devpods/gateway_test.go`:

```go
package devpods

import (
	"testing"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
)

func TestGatewaySetting_RoundTrip(t *testing.T) {
	db, err := database.Init(":memory:")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	s := config.NewSettingsStore(db)
	gw := Gateway{Host: "devpod.example.com", Port: 2222}
	if err := SetGateway(s, gw); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := GetGateway(s)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Host != "devpod.example.com" || got.Port != 2222 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestSSHCommand(t *testing.T) {
	gw := Gateway{Host: "devpod.example.com", Port: 2222}
	got := gw.SSHCommand("alice", "alice-gpu-8c-a1b2")
	want := "ssh alice+alice-gpu-8c-a1b2@devpod.example.com -p 2222"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
```

- [ ] **Step 2: Run it to verify failure**

Run: `go test ./internal/devpods/ -run TestGatewaySetting_RoundTrip -v`
Expected: FAIL (package doesn't compile — types missing).

- [ ] **Step 3: Implement the gateway setting helper**

Create `internal/devpods/gateway.go`:

```go
package devpods

import (
	"fmt"

	"github.com/ZJUSCT/CSOJ/internal/config"
)

// Gateway is the stored shape of the `devpods.gateway` setting.
type Gateway struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

const gatewaySettingKey = "devpods.gateway"

// GetGateway reads the devpods gateway setting. Returns a zero value
// (no error) when the setting is absent.
func GetGateway(s *config.SettingsStore) (Gateway, error) {
	var g Gateway
	if err := s.Get(gatewaySettingKey, &g); err != nil {
		return Gateway{}, err
	}
	return g, nil
}

// SetGateway writes the devpods gateway setting.
func SetGateway(s *config.SettingsStore, g Gateway) error {
	return s.Set(gatewaySettingKey, g)
}

// SSHCommand builds the user-facing login string. podName is the full
// DevPod resource name (e.g. "alice-gpu-8c-a1b2").
func (g Gateway) SSHCommand(username, podName string) string {
	return fmt.Sprintf("ssh %s+%s@%s -p %d", username, podName, g.Host, g.Port)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/devpods/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/devpods/gateway.go internal/devpods/gateway_test.go
git commit -m "feat(devpods): gateway setting + ssh command builder"
```

---

## Task 3: `Scheduler.DynamicClientForCluster` accessor

**Files:**
- Modify: `internal/judger/scheduler.go`

- [ ] **Step 1: Read the current Scheduler + ClusterState**

Read `internal/judger/scheduler.go` lines 38–75 (structs) and 300–340 (`GetClusterStates`) to mirror the locking style. The `clusters map[string]*ClusterState` is guarded by `mu sync.RWMutex`.

- [ ] **Step 2: Add the accessor**

In `internal/judger/scheduler.go`, add after `GetClusterStates` (around line 335):

```go
// DynamicClientForCluster returns the cached dynamic.Interface + namespace
// for a cluster name. Returns an error if the cluster is not loaded
// (kubeconfig parse failure at startup, or the row was removed). Used by
// the DevPods integration to talk to the devpods CRDs in that cluster.
func (s *Scheduler) DynamicClientForCluster(name string) (dynamic.Interface, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.clusters[name]
	if !ok {
		return nil, "", fmt.Errorf("cluster %q not loaded", name)
	}
	return c.dyn, c.Namespace, nil
}
```

Add `"k8s.io/client-go/dynamic"` to the import block of `scheduler.go` if not already present (it is imported indirectly via `buildClusterState`; check and add if `go vet` complains).

- [ ] **Step 3: Build + vet**

Run: `go build ./... && go vet ./...`
Expected: exit 0.

- [ ] **Step 4: Commit**

```bash
git add internal/judger/scheduler.go
git commit -m "feat(judger): expose DynamicClientForCluster accessor"
```

---

## Task 4: `internal/devpods` CRD client — `RenderDevPod`

**Files:**
- Create: `internal/devpods/client.go`
- Create: `internal/devpods/render_test.go`

- [ ] **Step 1: Write the failing render test**

Create `internal/devpods/render_test.go`:

```go
package devpods

import (
	"testing"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func tpl(id string, persist bool) *models.DevPodTemplate {
	t := &models.DevPodTemplate{
		ID: id, Name: "T", ClusterName: "c1", Image: "ubuntu:24.04",
		Shell: "bash", Cores: 8, Memory: 16 << 30,
		NodeSelector: models.JSONMap{"numa-node": "0"},
		Tolerations:  models.RawJSON(`[{"key":"dedicated","operator":"Equal","value":"gpu","effect":"NoSchedule"}]`),
		DefaultPerUser: 1, DefaultGlobal: 5,
	}
	if persist {
		t.PersistenceSize = "20Gi"
	}
	return t
}

func mustNested(t *testing.T, u *unstructured.Unstructured, path ...string) interface{} {
	t.Helper()
	v, ok, err := unstructured.NestedFieldNoCopy(u.Object, path...)
	if err != nil || !ok {
		t.Fatalf("missing nested %v: ok=%v err=%v", path, ok, err)
	}
	return v
}

func TestRenderDevPod_Labels(t *testing.T) {
	u, err := RenderDevPod("alice", "alice-gpu-8c-a1b2", tpl("gpu-8c", false))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if u.GetName() != "alice-gpu-8c-a1b2" {
		t.Errorf("name = %q", u.GetName())
	}
	if got := u.GetLabels()["devpod.io/owner"]; got != "alice" {
		t.Errorf("owner label = %q", got)
	}
	if got := u.GetLabels()["csoj.io/template"]; got != "gpu-8c" {
		t.Errorf("template label = %q", got)
	}
}

func TestRenderDevPod_ResourcesGuaranteed(t *testing.T) {
	u, _ := RenderDevPod("alice", "alice-gpu-8c-a1b2", tpl("gpu-8c", false))
	req := mustNested(t, u, "spec", "pod", "spec", "containers", 0, "resources", "requests").(map[string]interface{})
	if req["cpu"] != "8" || req["memory"] != "17179869184" {
		t.Errorf("requests = %+v", req)
	}
	lim := mustNested(t, u, "spec", "pod", "spec", "containers", 0, "resources", "limits").(map[string]interface{})
	if lim["cpu"] != "8" || lim["memory"] != "17179869184" {
		t.Errorf("limits = %+v (must equal requests for Guaranteed QoS)", lim)
	}
}

func TestRenderDevPod_NodeSelectorAndTolerations(t *testing.T) {
	u, _ := RenderDevPod("alice", "alice-gpu-8c-a1b2", tpl("gpu-8c", false))
	ns := mustNested(t, u, "spec", "pod", "spec", "nodeSelector").(map[string]interface{})
	if ns["numa-node"] != "0" {
		t.Errorf("nodeSelector = %+v", ns)
	}
	tol, ok := unstructured.NestedSlice(u.Object, "spec", "pod", "spec", "tolerations")
	if !ok || len(tol) != 1 {
		t.Errorf("tolerations missing: %v ok=%v", tol, ok)
	}
}

func TestRenderDevPod_Persistence(t *testing.T) {
	u, _ := RenderDevPod("alice", "alice-gpu-8c-a1b2", tpl("gpu-8c", true))
	ps, ok := unstructured.NestedMap(u.Object, "spec", "persistence")
	if !ok {
		t.Fatalf("persistence not set")
	}
	if ps["size"] != "20Gi" {
		t.Errorf("persistence size = %v", ps["size"])
	}
	if ps["mountPath"] != "/home/alice" {
		t.Errorf("mountPath = %v", ps["mountPath"])
	}
}

func TestRenderDevPod_OwnerAndShell(t *testing.T) {
	u, _ := RenderDevPod("alice", "alice-gpu-8c-a1b2", tpl("gpu-8c", false))
	if o, _, _ := unstructured.NestedString(u.Object, "spec", "owner"); o != "alice" {
		t.Errorf("owner = %q", o)
	}
	if s, _, _ := unstructured.NestedString(u.Object, "spec", "shell"); s != "bash" {
		t.Errorf("shell = %q", s)
	}
	// sanity: container name + image
	if img, _, _ := unstructured.NestedString(u.Object, "spec", "pod", "spec", "containers", 0, "image"); img != "ubuntu:24.04" {
		t.Errorf("image = %q", img)
	}
}

func TestRenderDevPod_NoShareProcessNamespace(t *testing.T) {
	u, _ := RenderDevPod("alice", "alice-gpu-8c-a1b2", tpl("gpu-8c", false))
	// CSOJ does NOT set shareProcessNamespace; devpods' own render decides.
	if _, ok := unstructured.NestedBool(u.Object, "spec", "pod", "spec", "shareProcessNamespace"); ok {
		t.Errorf("shareProcessNamespace must not be set by CSOJ")
	}
	// corev1 import sanity (keeps the import used)
	_ = corev1.PodPending
}

func TestValidateDevPodOwner(t *testing.T) {
	good := []string{"alice", "bob-2", "x"}
	bad := []string{"", "Alice", "a+b", "toolongusernametoolongusernametoolong"}
	for _, o := range good {
		if !ValidOwner(o) {
			t.Errorf("expected %q valid", o)
		}
	}
	for _, o := range bad {
		if ValidOwner(o) {
			t.Errorf("expected %q invalid", o)
		}
	}
}

func TestNameBudget(t *testing.T) {
	cases := []struct {
		username, tplID string
		ok               bool
	}{
		{"alice", "gpu-8c", true},       // 5+1+6+1+4 = 17
		{"toolongusername", "gpu-8c", false}, // 16+1+6+1+4 = 28 > 22
		{"ali", "gpu-8c-toolong", false},
	}
	for _, c := range cases {
		err := CheckNameBudget(c.username, c.tplID)
		if c.ok && err != nil {
			t.Errorf("%q+%q: unexpected err %v", c.username, c.tplID, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%q+%q: expected budget error", c.username, c.tplID)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/devpods/ -run TestRenderDevPod -v`
Expected: FAIL (compile error — `RenderDevPod` undefined).

- [ ] **Step 3: Implement `client.go` (render + validation + name budget)**

Create `internal/devpods/client.go`:

```go
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
		"containers":    []interface{}{container},
		"nodeSelector":  map[string]interface{}(tpl.NodeSelector),
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

// ListDevPodsByOwner lists DevPods owned by owner (via label), optionally
// also filtered by template ID. Either label value may be "" to skip it.
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/devpods/ -v`
Expected: PASS (all render + validation + name budget tests). The `TestGatewaySetting_*` from Task 2 also still pass.

- [ ] **Step 5: Build + vet**

Run: `go build ./... && go vet ./...`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
git add internal/devpods/client.go internal/devpods/render_test.go
git commit -m "feat(devpods): CRD client — RenderDevPod + CRUD + validation"
```

---

## Task 5: Quota check

**Files:**
- Create: `internal/devpods/quota.go`
- Create: `internal/devpods/quota_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/devpods/quota_test.go`:

```go
package devpods

import (
	"context"
	"testing"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func fakeClient(t *testing.T, objs ...runtime.Object) *Client {
	t.Helper()
	scheme := runtime.NewScheme()
	dyn := dynamicfake.NewSimpleDynamicClient(scheme, objs...)
	return NewClient(dyn, "devpods")
}

func mkDevPod(name, owner, tpl string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "devpod.io/v1alpha1",
		"kind":       "DevPod",
		"metadata": map[string]interface{}{
			"name": name, "namespace": "devpods",
			"labels": map[string]interface{}{
				"devpod.io/owner": owner, "csoj.io/template": tpl,
			},
		},
	}}
}

func TestCheckQuota_PerUserOK(t *testing.T) {
	c := fakeClient(t, mkDevPod("alice-x-a1", "alice", "x"))
	tpl := &models.DevPodTemplate{ID: "x", DefaultPerUser: 2, DefaultGlobal: 5}
	if err := CheckQuota(context.Background(), c, "alice", tpl); err != nil {
		t.Fatalf("expected ok: %v", err)
	}
}

func TestCheckQuota_PerUserExceeded(t *testing.T) {
	c := fakeClient(t,
		mkDevPod("alice-x-a1", "alice", "x"),
		mkDevPod("alice-x-a2", "alice", "x"),
	)
	tpl := &models.DevPodTemplate{ID: "x", DefaultPerUser: 2, DefaultGlobal: 5}
	if err := CheckQuota(context.Background(), c, "alice", tpl); err == nil {
		t.Errorf("expected per-user quota error")
	}
}

func TestCheckQuota_GlobalExceeded(t *testing.T) {
	c := fakeClient(t,
		mkDevPod("alice-x-a1", "alice", "x"),
		mkDevPod("bob-x-a1", "bob", "x"),
	)
	tpl := &models.DevPodTemplate{ID: "x", DefaultPerUser: 5, DefaultGlobal: 2}
	if err := CheckQuota(context.Background(), c, "alice", tpl); err == nil {
		t.Errorf("expected global quota error")
	}
}

func TestCheckQuota_OtherUsersDoNotCountForPerUser(t *testing.T) {
	c := fakeClient(t,
		mkDevPod("alice-x-a1", "alice", "x"),
		mkDevPod("bob-x-a1", "bob", "x"),
	)
	tpl := &models.DevPodTemplate{ID: "x", DefaultPerUser: 1, DefaultGlobal: 5}
	if err := CheckQuota(context.Background(), c, "alice", tpl); err != nil {
		t.Errorf("bob's pod must not count against alice's per-user quota: %v", err)
	}
}

// silence unused import in case metav1 is referenced later
var _ = metav1.GetOptions{}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/devpods/ -run TestCheckQuota -v`
Expected: FAIL (`CheckQuota` undefined).

- [ ] **Step 3: Implement `quota.go`**

Create `internal/devpods/quota.go`:

```go
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/devpods/ -v`
Expected: all PASS.

- [ ] **Step 5: Build + vet**

Run: `go build ./... && go vet ./...`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
git add internal/devpods/quota.go internal/devpods/quota_test.go
git commit -m "feat(devpods): per-user + global quota check"
```

---

## Task 6: Admin template API handlers + routes

**Files:**
- Create: `internal/api/admin/devpod_templates.go`
- Modify: `internal/api/admin/router.go`

- [ ] **Step 1: Implement the handlers**

Create `internal/api/admin/devpod_templates.go`:

```go
package admin

import (
	"net/http"
	"regexp"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
)

var templateIDRE = regexp.MustCompile(`^[a-z0-9-]{1,6}$`)

func (h *Handler) listDevPodTemplates(c *gin.Context) {
	rows, err := database.ListDevPodTemplates(h.db)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, rows, "Templates retrieved")
}

func (h *Handler) createDevPodTemplate(c *gin.Context) {
	var tpl models.DevPodTemplate
	if err := c.ShouldBindJSON(&tpl); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if !templateIDRE.MatchString(tpl.ID) {
		util.Error(c, http.StatusBadRequest, "id must match [a-z0-9-]{1,6}")
		return
	}
	if tpl.Name == "" || tpl.ClusterName == "" || tpl.Image == "" {
		util.Error(c, http.StatusBadRequest, "name, cluster_name, image are required")
		return
	}
	if tpl.Cores <= 0 || tpl.Memory <= 0 {
		util.Error(c, http.StatusBadRequest, "cores and memory must be positive")
		return
	}
	if tpl.DefaultPerUser <= 0 {
		tpl.DefaultPerUser = 1
	}
	if tpl.DefaultGlobal <= 0 {
		tpl.DefaultGlobal = 5
	}
	// cluster must exist
	if _, err := database.GetCluster(h.db, tpl.ClusterName); err != nil {
		util.Error(c, http.StatusBadRequest, "referenced cluster does not exist")
		return
	}
	if err := database.CreateDevPodTemplate(h.db, &tpl); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, tpl, "Template created")
}

func (h *Handler) updateDevPodTemplate(c *gin.Context) {
	id := c.Param("id")
	var tpl models.DevPodTemplate
	if err := c.ShouldBindJSON(&tpl); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	if id != tpl.ID {
		util.Error(c, http.StatusBadRequest, "id in path does not match body")
		return
	}
	if _, err := database.GetDevPodTemplate(h.db, id); err != nil {
		util.Error(c, http.StatusNotFound, "template not found")
		return
	}
	if err := database.UpdateDevPodTemplate(h.db, &tpl); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, tpl, "Template updated")
}

func (h *Handler) deleteDevPodTemplate(c *gin.Context) {
	id := c.Param("id")
	if err := database.DeleteDevPodTemplate(h.db, id); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "Template deleted")
}
```

- [ ] **Step 2: Wire routes**

In `internal/api/admin/router.go`, inside the `adminV1` group (after the `clusters` block, before `settings`), add:

```go
		// DevPod Template Management
		devpodTemplates := adminV1.Group("/devpod-templates")
		{
			devpodTemplates.GET("", h.listDevPodTemplates)
			devpodTemplates.POST("", h.createDevPodTemplate)
			devpodTemplates.PUT("/:id", h.updateDevPodTemplate)
			devpodTemplates.DELETE("/:id", h.deleteDevPodTemplate)
		}
```

- [ ] **Step 3: Add the `GetCluster` helper if missing**

Run: `grep -n "func GetCluster" internal/database/crud.go`
If it does not exist, add to `internal/database/crud.go`:

```go
func GetCluster(db *gorm.DB, name string) (*models.Cluster, error) {
	var c models.Cluster
	if err := db.Where("name = ?", name).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}
```

- [ ] **Step 4: Build + vet**

Run: `go build ./... && go vet ./...`
Expected: exit 0.

- [ ] **Step 5: Commit**

```bash
git add internal/api/admin/devpod_templates.go internal/api/admin/router.go internal/database/crud.go
git commit -m "feat(api): admin DevPod template CRUD"
```

---

## Task 7: User DevPod API handlers + routes

**Files:**
- Create: `internal/api/user/devpods.go`
- Modify: `internal/api/user/router.go`

- [ ] **Step 1: Implement the handlers**

Create `internal/api/user/devpods.go`:

```go
package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/devpods"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// devpodInstance is the JSON shape returned to the frontend.
type devpodInstance struct {
	Name       string    `json:"name"`
	Template   string    `json:"template"`
	Phase      string    `json:"phase"`
	Endpoint   string    `json:"endpoint"`
	SSHCommand string    `json:"ssh_command"`
	CreatedAt  time.Time `json:"created_at"`
}

// devpodListResponse wraps the list + the gateway (so the frontend can
// build ssh commands without a second round-trip).
type devpodListResponse struct {
	Items   []devpodInstance `json:"items"`
	Gateway struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"gateway"`
}

// clientForTemplate loads the template + resolves the referenced cluster's
// dynamic client. Returns 4xx/5xx via util.Error on failure.
func (h *Handler) clientForTemplate(c *gin.Context, templateID string) (*devpods.Client, *devpods.Gateway, *database.GetUserResult) {
	// placeholder — replaced in Step 2
	return nil, nil, nil
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Handler) listDevPods(c *gin.Context) {
	userID := c.GetString("userID")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return
	}
	gw, err := devpods.GetGateway(h.settings)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("read gateway setting: %w", err))
		return
	}

	// DevPods may live in any registered cluster; list across each cluster
	// the user's templates reference. Simpler: list across ALL clusters,
	// filter by owner label. Few clusters expected.
	resp := devpodListResponse{Items: []devpodInstance{}}
	resp.Gateway.Host = gw.Host
	resp.Gateway.Port = gw.Port

	seen := map[string]bool{}
	for _, cl := range h.scheduler.GetClusterNames() {
		dyn, ns, err := h.scheduler.DynamicClientForCluster(cl)
		if err != nil {
			continue
		}
		cli := devpods.NewClient(dyn, ns)
		items, err := cli.ListDevPods(c.Request.Context(), user.Username, "")
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
				Endpoint:  devpods.Endpoint(u),
				CreatedAt: devpods.CreatedAt(u),
			}
			if inst.Phase == "Running" && inst.Endpoint != "" {
				inst.SSHCommand = gw.SSHCommand(user.Username, name)
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
	if !devpods.ValidOwner(user.Username) {
		util.Error(c, http.StatusBadRequest, "username not valid for devpod naming (must match [a-z0-9-]{1,32}, no '+')")
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
	if err := devpods.CheckNameBudget(user.Username, tpl.ID); err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	dyn, ns, err := h.scheduler.DynamicClientForCluster(tpl.ClusterName)
	if err != nil {
		util.Error(c, http.StatusBadGateway, fmt.Errorf("cluster %q not loaded: %w", tpl.ClusterName, err))
		return
	}
	cli := devpods.NewClient(dyn, ns)

	if err := devpods.CheckQuota(c.Request.Context(), cli, user.Username, tpl); err != nil {
		if qe, ok := err.(*devpods.QuotaError); ok {
			util.Error(c, http.StatusConflict, qe)
			return
		}
		util.Error(c, http.StatusInternalServerError, err)
		return
	}

	if err := cli.EnsureUser(c.Request.Context(), user.Username); err != nil {
		util.Error(c, http.StatusInternalServerError, fmt.Errorf("ensure user: %w", err))
		return
	}

	podName, err := devpods.GenerateDevPodName(user.Username, tpl.ID, randHex(2))
	if err != nil {
		util.Error(c, http.StatusBadRequest, err)
		return
	}
	u, err := devpods.RenderDevPod(user.Username, podName, tpl)
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

// requireOwnedDevPod fetches the DevPod and verifies the owner label
// matches the caller's username. Returns the CR + cli, or false (already
// responded) on error.
func (h *Handler) requireOwnedDevPod(c *gin.Context, name string) (*unstructured.Unstructured, *devpods.Client, bool) {
	userID := c.GetString("userID")
	user, err := database.GetUserByID(h.db, userID)
	if err != nil {
		util.Error(c, http.StatusNotFound, err)
		return nil, nil, false
	}
	// search every cluster for the named DevPod
	for _, cl := range h.scheduler.GetClusterNames() {
		dyn, ns, err := h.scheduler.DynamicClientForCluster(cl)
		if err != nil {
			continue
		}
		cli := devpods.NewClient(dyn, ns)
		u, err := cli.GetDevPod(c.Request.Context(), name)
		if err != nil {
			continue
		}
		owner := u.GetLabels()["devpod.io/owner"]
		if owner != user.Username {
			util.Error(c, http.StatusForbidden, "not your devpod")
			return nil, nil, false
		}
		return u, cli, true
	}
	util.Error(c, http.StatusNotFound, "devpod not found")
	return nil, nil, false
}

func (h *Handler) getDevPod(c *gin.Context) {
	name := c.Param("name")
	u, cli, ok := h.requireOwnedDevPod(c, name)
	if !ok {
		return
	}
	gw, _ := devpods.GetGateway(h.settings)
	user, _ := database.GetUserByID(h.db, c.GetString("userID"))
	inst := devpodInstance{
		Name:     u.GetName(),
		Template: u.GetLabels()["csoj.io/template"],
		Phase:    devpods.Phase(u),
		Endpoint: devpods.Endpoint(u),
		CreatedAt: devpods.CreatedAt(u),
	}
	if inst.Phase == "Running" && inst.Endpoint != "" {
		inst.SSHCommand = gw.SSHCommand(user.Username, name)
	}
	_ = cli
	util.Success(c, inst, "DevPod found")
}

func (h *Handler) startDevPod(c *gin.Context) {
	_, cli, ok := h.requireOwnedDevPod(c, c.Param("name"))
	if !ok {
		return
	}
	if err := cli.PatchRunning(c.Request.Context(), c.Param("name"), true); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "DevPod starting")
}

func (h *Handler) stopDevPod(c *gin.Context) {
	_, cli, ok := h.requireOwnedDevPod(c, c.Param("name"))
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
	_, cli, ok := h.requireOwnedDevPod(c, c.Param("name"))
	if !ok {
		return
	}
	if err := cli.DeleteDevPod(c.Request.Context(), c.Param("name")); err != nil {
		util.Error(c, http.StatusInternalServerError, err)
		return
	}
	util.Success(c, nil, "DevPod deleted")
}

// Drop the unused-import guard for metav1 if list path no longer needs it.
var _ = metav1.GetOptions{}
var _ = strings.TrimSpace
var _ = context.Background
```

> Note: `clientForTemplate` placeholder is removed — `createDevPod` inlines the lookup. Delete the placeholder method before building (Step 3 covers this). Actually keep the file clean: **remove the `clientForTemplate` stub method** since it is unused.

- [ ] **Step 2: Add `GetClusterNames` to Scheduler**

In `internal/judger/scheduler.go`, add (near `GetClusterStates`):

```go
// GetClusterNames returns the names of all loaded clusters.
func (s *Scheduler) GetClusterNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.clusters))
	for name := range s.clusters {
		out = append(out, name)
	}
	return out
}
```

- [ ] **Step 3: Clean up unused code + imports**

Remove the `clientForTemplate` stub method and any unused imports (`context`, `metav1`, `strings`) the linter flags. Run `goimports -w internal/api/user/devpods.go` if available, else manually ensure only used imports remain: `context` IS used (`c.Request.Context()`), `strings` is NOT used (remove the `var _ = strings.TrimSpace` line + import), `metav1` is NOT used (remove the `var _` line + import).

Final imports should be:
```go
import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/devpods"
	"github.com/ZJUSCT/CSOJ/internal/util"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)
```

- [ ] **Step 4: Wire routes**

In `internal/api/user/router.go`, inside the `authed` group (after the `submissions` block), add:

```go
			// DevPods
			devpodsGroup := authed.Group("/devpods")
			{
				devpodsGroup.GET("", h.listDevPods)
				devpodsGroup.POST("", h.createDevPod)
				devpodsGroup.GET("/:name", h.getDevPod)
				devpodsGroup.POST("/:name/start", h.startDevPod)
				devpodsGroup.POST("/:name/stop", h.stopDevPod)
				devpodsGroup.DELETE("/:name", h.deleteDevPod)
			}
```

- [ ] **Step 5: Build + vet**

Run: `go build ./... && go vet ./...`
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
git add internal/api/user/devpods.go internal/api/user/router.go internal/judger/scheduler.go
git commit -m "feat(api): user DevPod create/list/start/stop/delete"
```

---

## Task 8: Frontend types + nav

**Files:**
- Modify: `frontend/lib/types.ts`
- Modify: `frontend/components/layout/main-nav.tsx`
- Modify: `frontend/components/layout/admin-sub-nav.tsx`

- [ ] **Step 1: Add types**

In `frontend/lib/types.ts`, append:

```ts
export interface DevPodTemplate {
  id: string;
  name: string;
  cluster_name: string;
  image: string;
  shell: string;
  cores: number;
  memory: number;
  node_selector: Record<string, string>;
  tolerations: any[];
  default_per_user: number;
  default_global: number;
  persistence_size: string;
}

export interface DevPodInstance {
  name: string;
  template: string;
  phase: string;
  endpoint: string;
  ssh_command: string;
  created_at: string;
}

export interface DevPodGateway { host: string; port: number; }

export interface DevPodListResponse {
  items: DevPodInstance[];
  gateway: DevPodGateway;
}
```

- [ ] **Step 2: Add the DevPods nav tab**

In `frontend/components/layout/main-nav.tsx`, in `allRoutes`, after the `contests` entry, add:

```ts
    { href: "/devpods", label: t("devpods") },
```

- [ ] **Step 3: Add the admin sub-nav entry**

In `frontend/components/layout/admin-sub-nav.tsx`, add `Boxes` to the lucide-react import and add to `routes`:

```ts
    { href: "/admin/devpod-templates", label: "DevPod Templates", icon: Boxes },
```

- [ ] **Step 4: Build check**

Run: `cd frontend && pnpm build`
Expected: succeeds (routes may 404 until pages exist in Task 9 — that's fine; nav renders).

- [ ] **Step 5: Commit**

```bash
git add frontend/lib/types.ts frontend/components/layout/main-nav.tsx frontend/components/layout/admin-sub-nav.tsx
git commit -m "feat(frontend): DevPod types + nav entries"
```

---

## Task 9: i18n keys

**Files:**
- Modify: `frontend/public/messages/en.json`
- Modify: `frontend/public/messages/zh.json`

- [ ] **Step 1: Add English keys**

In `frontend/public/messages/en.json`, add a top-level `"devpods"` section and an `"home.devpods"` label. Add `"devpods": "DevPods"` to the `"home"` object, and a new top-level key:

```json
  "devpods": {
    "title": "DevPods",
    "create": "Create",
    "createNew": "Create DevPod",
    "myDevPods": "My DevPods",
    "name": "Name",
    "template": "Template",
    "status": "Status",
    "sshCommand": "SSH Command",
    "created": "Created",
    "start": "Start",
    "stop": "Stop",
    "delete": "Delete",
    "noInstances": "No DevPods yet. Create one from a template above.",
    "preparing": "Preparing…",
    "limitReached": "Limit reached",
    "used": "used",
    "cores": "cores",
    "deleteConfirm": "Delete this DevPod? Its home volume is kept by devpods.",
    "createFailed": "Failed to create DevPod",
    "created": "DevPod created"
  },
  "adminDevPodTemplates": {
    "title": "DevPod Templates",
    "new": "New Template",
    "edit": "Edit",
    "id": "ID",
    "name": "Name",
    "cluster": "Cluster",
    "image": "Image",
    "shell": "Shell",
    "cores": "Cores",
    "memory": "Memory (bytes)",
    "nodeSelector": "Node Selector",
    "tolerations": "Tolerations (JSON)",
    "perUser": "Per-user limit",
    "global": "Global limit",
    "persistence": "Persistence size",
    "deleteConfirm": "Delete this template? Existing DevPods are unaffected."
  }
```

Add `"devpods": "DevPods"` inside the existing `"home"` object (next to `"contests"`).

- [ ] **Step 2: Add Chinese keys (mirror)**

In `frontend/public/messages/zh.json`, mirror the same structure with Chinese values:

```json
  "devpods": {
    "title": "DevPods",
    "create": "创建",
    "createNew": "创建 DevPod",
    "myDevPods": "我的 DevPod",
    "name": "名称",
    "template": "模板",
    "status": "状态",
    "sshCommand": "SSH 命令",
    "created": "创建时间",
    "start": "启动",
    "stop": "停止",
    "delete": "删除",
    "noInstances": "还没有 DevPod。从上方模板创建一个吧。",
    "preparing": "准备中…",
    "limitReached": "已达上限",
    "used": "已用",
    "cores": "核",
    "deleteConfirm": "删除这个 DevPod？devpods 会保留它的 home 卷。",
    "createFailed": "创建 DevPod 失败",
    "created": "DevPod 已创建"
  },
  "adminDevPodTemplates": {
    "title": "DevPod 模板",
    "new": "新建模板",
    "edit": "编辑",
    "id": "ID",
    "name": "名称",
    "cluster": "集群",
    "image": "镜像",
    "shell": "Shell",
    "cores": "核数",
    "memory": "内存（字节）",
    "nodeSelector": "Node Selector",
    "tolerations": "Tolerations (JSON)",
    "perUser": "每用户上限",
    "global": "全局上限",
    "persistence": "持久化大小",
    "deleteConfirm": "删除这个模板？已有的 DevPod 不受影响。"
  }
```

And `"devpods": "DevPods"` inside `"home"`.

- [ ] **Step 3: Validate JSON**

Run: `python3 -c "import json; json.load(open('frontend/public/messages/en.json')); json.load(open('frontend/public/messages/zh.json')); print('ok')"`
Expected: `ok`.

- [ ] **Step 4: Commit**

```bash
git add frontend/public/messages/en.json frontend/public/messages/zh.json
git commit -m "feat(frontend): i18n keys for DevPods + templates"
```

---

## Task 10: User DevPods page

**Files:**
- Create: `frontend/app/(main)/devpods/page.tsx`
- Create: `frontend/components/devpods/devpod-card.tsx`
- Create: `frontend/components/devpods/devpod-row.tsx`

- [ ] **Step 1: Create the template card component**

Create `frontend/components/devpods/devpod-card.tsx`:

```tsx
"use client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DevPodTemplate } from "@/lib/types";
import { useTranslations } from "next-intl";

export function DevPodCard({ tpl, used, disabled, onCreate }: {
  tpl: DevPodTemplate; used: number; disabled: boolean; onCreate: () => void;
}) {
  const t = useTranslations("devpods");
  const atLimit = used >= tpl.default_per_user;
  return (
    <Card>
      <CardHeader><CardTitle>{tpl.name}</CardTitle></CardHeader>
      <CardContent className="space-y-2 text-sm text-muted-foreground">
        <div>{tpl.cores} {t("cores")} · {(tpl.memory / (1<<30)).toFixed(0)}Gi</div>
        {tpl.node_selector && Object.keys(tpl.node_selector).length > 0 && (
          <div className="font-mono text-xs">{Object.entries(tpl.node_selector).map(([k,v])=>`${k}=${v}`).join(", ")}</div>
        )}
        <div>{t("used")}: {used}/{tpl.default_per_user}</div>
        <Button className="w-full" disabled={disabled || atLimit} onClick={onSubmit(onCreate)}>
          {atLimit ? t("limitReached") : t("create")}
        </Button>
      </CardContent>
    </Card>
  );
}
function onSubmit(fn: () => void) { return () => fn(); }
```

- [ ] **Step 2: Create the row component**

Create `frontend/components/devpods/devpod-row.tsx`:

```tsx
"use client";
import { Button } from "@/components/ui/button";
import { TableCell, TableRow } from "@/components/ui/table";
import { DevPodInstance } from "@/lib/types";
import { CopyButton } from "@/components/ui/shadcn-io/copy-button";
import { format } from "date-fns";
import { useTranslations } from "next-intl";

export function DevPodRow({ inst, onStart, onStop, onDelete }: {
  inst: DevPodInstance;
  onStart: () => void; onStop: () => void; onDelete: () => void;
}) {
  const t = useTranslations("devpods");
  const running = inst.phase === "Running";
  return (
    <TableRow>
      <TableCell className="font-mono text-xs">{inst.name}</TableCell>
      <TableCell>{inst.template}</TableCell>
      <TableCell>{inst.phase}</TableCell>
      <TableCell className="flex items-center gap-2">
        {inst.ssh_command ? (
          <>
            <code className="font-mono text-xs">{inst.ssh_command}</code>
            <CopyButton content={inst.ssh_command} size="sm" />
          </>
        ) : <span className="text-xs text-muted-foreground">{t("preparing")}</span>}
      </TableCell>
      <TableCell>{inst.created_at ? format(new Date(inst.created_at), "MM/dd HH:mm") : "-"}</TableCell>
      <TableCell className="space-x-1">
        {!running && <Button size="sm" variant="outline" onClick={onStart}>{t("start")}</Button>}
        {running && <Button size="sm" variant="outline" onClick={onStop}>{t("stop")}</Button>}
        <Button size="sm" variant="ghost" className="text-destructive" onClick={onDelete}>{t("delete")}</Button>
      </TableCell>
    </TableRow>
  );
}
```

- [ ] **Step 3: Create the page**

Create `frontend/app/(main)/devpods/page.tsx`:

```tsx
"use client";
import useSWR from "swr";
import api from "@/lib/api";
import { DevPodListResponse, DevPodTemplate, DevPodInstance } from "@/lib/types";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableHead, TableHeader, TableRow, TableCell } from "@/components/ui/table";
import { useTranslations } from "next-intl";
import { useToast } from "@/hooks/use-toast";
import { DevPodCard } from "@/components/devpods/devpod-card";
import { DevPodRow } from "@/components/devpods/devpod-row";
import { useState } from "react";

const fetcher = (url: string) => api.get(url).then(r => r.data.data);

export default function DevPodsPage() {
  const t = useTranslations("devpods");
  const { toast } = useToast();
  const { data: list, mutate } = useSWR<DevPodListResponse>("/devpods", fetcher, { refreshInterval: 5000 });
  const { data: templates } = useSWR<DevPodTemplate[]>("/admin/devpod-templates", fetcher);
  const [busy, setBusy] = useState<string | null>(null);

  const items = list?.items ?? [];
  const usedByTpl = (id: string) => items.filter(i => i.template === id).length;

  const create = async (tplId: string) => {
    setBusy(tplId);
    try {
      await api.post("/devpods", { template_id: tplId });
      toast({ title: t("created") });
      mutate();
    } catch (e: any) {
      toast({ variant: "destructive", title: t("createFailed"), description: e.response?.data?.message });
    } finally {
      setBusy(null);
    }
  };

  const act = async (name: string, op: "start"|"stop"|"delete") => {
    await api[op === "delete" ? "delete" : "post"](`/devpods/${name}/${op}`);
    mutate();
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold mb-4">{t("title")}</h2>
        <div className="grid gap-4 md:grid-cols-3">
          {(templates ?? []).map(tpl => (
            <DevPodCard key={tpl.id} tpl={tpl} used={usedByTpl(tpl.id)} disabled={busy === tpl.id} onCreate={() => create(tpl.id)} />
          ))}
          {templates && templates.length === 0 && (
            <Card><CardContent className="p-4 text-sm text-muted-foreground">No templates configured.</CardContent></Card>
          )}
        </div>
      </div>
      <Card>
        <CardHeader><CardTitle>{t("myDevPods")}</CardTitle></CardHeader>
        <CardContent>
          {items.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("noInstances")}</p>
          ) : (
            <Table>
              <TableHeader><TableRow>
                <TableHead>{t("name")}</TableHead>
                <TableHead>{t("template")}</TableHead>
                <TableHead>{t("status")}</TableHead>
                <TableHead>{t("sshCommand")}</TableHead>
                <TableHead>{t("created")}</TableHead>
                <TableHead></TableHead>
              </TableRow></TableHeader>
              <TableBody>
                {items.map(inst => (
                  <DevPodRow key={inst.name} inst={inst}
                    onStart={() => act(inst.name, "start")}
                    onStop={() => act(inst.name, "stop")}
                    onDelete={() => act(inst.name, "delete")} />
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 4: Build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 5: Commit**

```bash
git add "frontend/app/(main)/devpods/page.tsx" frontend/components/devpods/
git commit -m "feat(frontend): user DevPods page"
```

---

## Task 11: Admin templates page

**Files:**
- Create: `frontend/app/(admin)/admin/devpod-templates/page.tsx`
- Create: `frontend/components/admin/devpod-template-actions.tsx`

- [ ] **Step 1: Create the template actions (create/edit/delete dialog)**

Create `frontend/components/admin/devpod-template-actions.tsx`:

```tsx
"use client";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { FormField, FormItem, FormLabel, FormControl, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import api from "@/lib/api";
import { useToast } from "@/hooks/use-toast";
import { DevPodTemplate } from "@/lib/types";
import useSWR from "swr";

const fetcher = (url: string) => api.get(url).then(r => r.data.data);
const empty: DevPodTemplate = { id:"",name:"",cluster_name:"",image:"ubuntu:24.04",shell:"bash",cores:4,memory:8<<30,node_selector:{},tolerations:[],default_per_user:1,default_global:5,persistence_size:"" };

export function CreateTemplateButton({ onDone }: { onDone: () => void }) {
  const [open, setOpen] = useState(false);
  const [v, setV] = useState<DevPodTemplate>(empty);
  const { toast } = useToast();
  const { data: clusters } = useSWR<{name:string}[]>("/admin/clusters", fetcher);
  const save = async () => {
    try { await api.post("/admin/devpod-templates", v); toast({title:"Created"}); setOpen(false); setV(empty); onDone(); }
    catch(e:any){ toast({variant:"destructive",title:"Failed",description:e.response?.data?.message}); }
  };
  const set = (k: keyof DevPodTemplate, val: any) => setV(s => ({...s, [k]: val}));
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild><Button>New Template</Button></DialogTrigger>
      <DialogContent className="max-w-2xl">
        <DialogHeader><DialogTitle>New Template</DialogTitle></DialogHeader>
        <div className="grid grid-cols-2 gap-3">
          <L label="ID"><Input value={v.id} onChange={e=>set("id",e.target.value)} placeholder="gpu-8c"/></L>
          <L label="Name"><Input value={v.name} onChange={e=>set("name",e.target.value)}/></L>
          <L label="Cluster">
            <Select value={v.cluster_name} onValueChange={x=>set("cluster_name",x)}>
              <SelectTrigger><SelectValue placeholder="cluster"/></SelectTrigger>
              <SelectContent>{(clusters??[]).map(c=><SelectItem key={c.name} value={c.name}>{c.name}</SelectItem>)}</SelectContent>
            </Select>
          </L>
          <L label="Image"><Input value={v.image} onChange={e=>set("image",e.target.value)}/></L>
          <L label="Shell"><Input value={v.shell} onChange={e=>set("shell",e.target.value)} placeholder="bash|zsh|fish"/></L>
          <L label="Cores"><Input type="number" value={v.cores} onChange={e=>set("cores",+e.target.value)}/></L>
          <L label="Memory (bytes)"><Input type="number" value={v.memory} onChange={e=>set("memory",+e.target.value)}/></L>
          <L label="Per-user limit"><Input type="number" value={v.default_per_user} onChange={e=>set("default_per_user",+e.target.value)}/></L>
          <L label="Global limit"><Input type="number" value={v.default_global} onChange={e=>set("default_global",+e.target.value)}/></L>
          <L label="Persistence size"><Input value={v.persistence_size} onChange={e=>set("persistence_size",e.target.value)} placeholder="20Gi (blank=off)"/></L>
          <div className="col-span-2"><L label="Node Selector (JSON)"><Textarea className="font-mono text-xs" rows={2} value={JSON.stringify(v.node_selector)} onChange={e=>{try{set("node_selector",JSON.parse(e.target.value))}catch{}}} placeholder='{"numa-node":"0"}'/></L></div>
          <div className="col-span-2"><L label="Tolerations (JSON)"><Textarea className="font-mono text-xs" rows={3} value={JSON.stringify(v.tolerations)} onChange={e=>{try{set("tolerations",JSON.parse(e.target.value))}catch{}}}/></L></div>
        </div>
        <DialogFooter><Button onClick={save}>Save</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
function L({label,children}:{label:string;children:React.ReactNode}){return(<div><label className="text-xs">{label}</label>{children}</div>);}
```

- [ ] **Step 2: Create the admin page**

Create `frontend/app/(admin)/admin/devpod-templates/page.tsx`:

```tsx
"use client";
import useSWR from "swr";
import api from "@/lib/api";
import { DevPodTemplate } from "@/lib/types";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { CreateTemplateButton } from "@/components/admin/devpod-template-actions";

const fetcher = (url: string) => api.get(url).then(r => r.data.data);

export default function DevPodTemplatesPage() {
  const { data: templates, mutate } = useSWR<DevPodTemplate[]>("/admin/devpod-templates", fetcher);
  const del = async (id: string) => { if (confirm("Delete template?")) { await api.delete(`/admin/devpod-templates/${id}`); mutate(); } };
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between"><CardTitle>DevPod Templates</CardTitle><CreateTemplateButton onDone={() => mutate()} /></CardHeader>
      <CardContent>
        <Table>
          <TableHeader><TableRow>
            <TableHead>ID</TableHead><TableHead>Name</TableHead><TableHead>Cluster</TableHead>
            <TableHead>Cores</TableHead><TableHead>Mem</TableHead><TableHead>PerUser</TableHead><TableHead>Global</TableHead><TableHead></TableHead>
          </TableRow></TableHeader>
          <TableBody>
            {(templates ?? []).map(t => (
              <TableRow key={t.id}>
                <TableCell className="font-mono text-xs">{t.id}</TableCell>
                <TableCell>{t.name}</TableCell>
                <TableCell>{t.cluster_name}</TableCell>
                <TableCell>{t.cores}</TableCell>
                <TableCell>{(t.memory/(1<<30)).toFixed(0)}Gi</TableCell>
                <TableCell>{t.default_per_user}</TableCell>
                <TableCell>{t.default_global}</TableCell>
                <TableCell><Button size="sm" variant="ghost" className="text-destructive" onClick={() => del(t.id)}>Delete</Button></TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
```

- [ ] **Step 3: Build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
git add "frontend/app/(admin)/admin/devpod-templates/page.tsx" frontend/components/admin/devpod-template-actions.tsx
git commit -m "feat(frontend): admin DevPod templates page"
```

---

## Task 12: Full build + manual verify

**Files:** none (verification only)

- [ ] **Step 1: Backend build + vet**

Run: `go build ./... && go vet ./...`
Expected: exit 0.

- [ ] **Step 2: Run all unit tests**

Run: `go test ./internal/devpods/ ./internal/database/ ./internal/config/ -v`
Expected: all PASS.

- [ ] **Step 3: Frontend build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 4: Boot CSOJ against an isolated DB**

```bash
WD=$(mktemp -d /tmp/csoj-devpod-XXXXXX)
mkdir -p "$WD/data/submissions" "$WD/data/logs" "$WD/data/avatars"
cat > "$WD/config.yaml" <<EOF
listen: "127.0.0.1:18099"
storage:
  database: "$WD/data/csoj.db"
  user_avatar: "$WD/data/avatars"
  submission_content: "$WD/data/submissions"
  submission_log: "$WD/data/logs"
auth:
  jwt:
    secret: "devpod-verify-secret"
    expire_hours: 72
EOF
go build -o "$WD/csoj" ./cmd/CSOJ
"$WD/csoj" -c "$WD/config.yaml" &
sleep 4
```

- [ ] **Step 5: Register admin + create a template**

```bash
B=http://127.0.0.1:18099/api/v1
curl -s -X POST $B/auth/local/register -H 'Content-Type: application/json' -d '{"username":"admin","password":"adminpass","nickname":"A"}'
TK=$(curl -s -X POST $B/auth/local/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"adminpass"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])")
AUTH="Authorization: Bearer $TK"
# need a cluster row that points at a real devpods cluster; for the smoke test,
# just create one and expect create-template to succeed; create-devpod will fail
# at the cluster step (expected without a live devpods cluster).
curl -s -X POST $B/admin/clusters -H "$AUTH" -H 'Content-Type: application/json' -d '{"name":"c1","kubeconfig":"","context":"","namespace":"devpods","concurrency":1,"heartbeat_ttl":30,"queue_mode":"channel"}'
curl -s -X POST $B/admin/devpod-templates -H "$AUTH" -H 'Content-Type: application/json' -d '{"id":"gpu-8c","name":"8c GPU","cluster_name":"c1","image":"ubuntu:24.04","shell":"bash","cores":8,"memory":17179869184,"node_selector":{"numa-node":"0"},"tolerations":[],"default_per_user":1,"default_global":5,"persistence_size":"20Gi"}'
curl -s $B/admin/devpod-templates -H "$AUTH"
```
Expected: template listed. Set the gateway setting:
```bash
curl -s -X PUT $B/admin/settings/devpods.gateway -H "$AUTH" -H 'Content-Type: application/json' -d '{"host":"devpod.example.com","port":2222}'
```

- [ ] **Step 6: Verify user endpoints respond (create will fail without a live devpods cluster — that's expected and acceptable for this smoke test)**

```bash
# register a user
curl -s -X POST $B/auth/local/register -H 'Content-Type: application/json' -d '{"username":"alice","password":"pass12345","nickname":"alice"}'
UTK=$(curl -s -X POST $B/auth/local/login -H 'Content-Type: application/json' -d '{"username":"alice","password":"pass12345"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])")
UAUTH="Authorization: Bearer $UTK"
curl -s $B/devpods -H "$UAUTH"   # expect {items:[], gateway:{host,port}}
# create will return 502 (cluster c1 kubeconfig empty) — acceptable:
curl -s -X POST $B/devpods -H "$UAUTH" -H 'Content-Type: application/json' -d '{"template_id":"gpu-8c"}'
```
Expected: `GET /devpods` returns `{items:[], gateway:{host:"devpod.example.com",port:2222}}`; `POST /devpods` returns 502 with a cluster-not-loaded message (no live devpods cluster wired). This confirms the wiring is correct end-to-end short of a real devpods cluster.

- [ ] **Step 7: Stop the server + cleanup**

```bash
kill %1 2>/dev/null
rm -rf "$WD"
```

- [ ] **Step 8: Final commit (if any lint/test fixes were made)**

```bash
git add -A
git commit -m "chore: build + verify DevPods integration" 2>/dev/null || echo "nothing to commit"
```

---

## Self-Review notes

- **Spec coverage:** model (T1), gateway setting (T2), cluster accessor (T3), CRD client render+CRUD (T4), quota per-user+global (T5), admin template CRUD API (T6), user devpod API list/create/get/start/stop/delete (T7), frontend types+nav (T8), i18n (T9), user page (T10), admin page (T11), build+verify (T12). Username validation at create-time (T7 `ValidOwner`). Name budget (T4 `CheckNameBudget`). Owner check (T7 `requireOwnedDevPod`). Persistence (T4 render). No CSOJ-side instance table — confirmed (list goes to CRs). No `shareProcessNamespace` — confirmed (T4 test). All spec sections mapped.
- **Naming consistency:** `Client`, `RenderDevPod`, `GenerateDevPodName`, `CheckNameBudget`, `ValidOwner`, `CheckQuota`/`QuotaError`, `GetGateway`/`SetGateway`, `Gateway.SSHCommand`, `Phase`/`Endpoint`/`CreatedAt` — used consistently across T2–T7.
- **Live devpods cluster:** the smoke test (T12) cannot fully exercise create without a real devpods cluster wired into CSOJ's Cluster table. The plan accepts this and verifies wiring up to the cluster step. Full e2e requires an operator to register a real devpods cluster kubeconfig and is a deploy-time action, not a code task.
