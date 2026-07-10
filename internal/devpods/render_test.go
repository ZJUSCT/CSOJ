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
		NodeSelector:   models.JSONMap{"numa-node": "0"},
		Tolerations:    models.RawJSON(`[{"key":"dedicated","operator":"Equal","value":"gpu","effect":"NoSchedule"}]`),
		DefaultPerUser: 1, DefaultGlobal: 5,
	}
	if persist {
		t.PersistenceSize = "20Gi"
	}
	return t
}

// mustNested walks u.Object along path, where each element is either a
// string (map key) or an int (slice index). It fatals on any miss.
func mustNested(t *testing.T, u *unstructured.Unstructured, path ...interface{}) interface{} {
	t.Helper()
	var v interface{} = u.Object
	for _, p := range path {
		switch pp := p.(type) {
		case string:
			m, ok := v.(map[string]interface{})
			if !ok {
				t.Fatalf("expected map at %q, got %T", pp, v)
			}
			v, ok = m[pp]
			if !ok {
				t.Fatalf("missing nested %v: key %q absent", path, pp)
			}
		case int:
			s, ok := v.([]interface{})
			if !ok {
				t.Fatalf("expected slice at %d, got %T", pp, v)
			}
			if pp < 0 || pp >= len(s) {
				t.Fatalf("index %d out of range (len %d)", pp, len(s))
			}
			v = s[pp]
		default:
			t.Fatalf("unsupported path element %v (%T)", p, p)
		}
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
	tol, ok, _ := unstructured.NestedSlice(u.Object, "spec", "pod", "spec", "tolerations")
	if !ok || len(tol) != 1 {
		t.Errorf("tolerations missing: %v ok=%v", tol, ok)
	}
}

func TestRenderDevPod_Persistence(t *testing.T) {
	u, _ := RenderDevPod("alice", "alice-gpu-8c-a1b2", tpl("gpu-8c", true))
	ps, ok, _ := unstructured.NestedMap(u.Object, "spec", "persistence")
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
	if img, _ := mustNested(t, u, "spec", "pod", "spec", "containers", 0, "image").(string); img != "ubuntu:24.04" {
		t.Errorf("image = %q", img)
	}
}

func TestRenderDevPod_NoShareProcessNamespace(t *testing.T) {
	u, _ := RenderDevPod("alice", "alice-gpu-8c-a1b2", tpl("gpu-8c", false))
	// CSOJ does NOT set shareProcessNamespace; devpods' own render decides.
	if _, ok, _ := unstructured.NestedBool(u.Object, "spec", "pod", "spec", "shareProcessNamespace"); ok {
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
		ok              bool
	}{
		{"alice", "gpu-8c", true},            // 5+1+6+1+4 = 17
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
