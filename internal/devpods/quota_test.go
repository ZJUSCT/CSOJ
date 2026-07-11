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
	return NewClient(dyn)
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
		"spec": map[string]interface{}{"running": true},
	}}
}

func mkStoppedDevPod(name, owner, tpl string) *unstructured.Unstructured {
	u := mkDevPod(name, owner, tpl)
	_ = unstructured.SetNestedField(u.Object, false, "spec", "running")
	return u
}

func TestCreateDevPod_UsesDedicatedNamespace(t *testing.T) {
	cli := fakeClient(t)
	u, err := RenderDevPod("alice", "alice-gpu-8ca1b2", tpl("gpu-8c"))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := cli.CreateDevPod(context.Background(), u); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := cli.GetDevPod(context.Background(), u.GetName())
	if err != nil {
		t.Fatalf("get from %q namespace: %v", Namespace, err)
	}
	if got.GetNamespace() != Namespace {
		t.Fatalf("namespace = %q, want %q", got.GetNamespace(), Namespace)
	}
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
	err := CheckQuota(context.Background(), c, "alice", tpl)
	qe, ok := err.(*QuotaError)
	if !ok {
		t.Fatalf("expected global running QuotaError, got %T (%v)", err, err)
	}
	if qe.Scope != "global_running" || qe.Used != 2 || qe.Limit != 2 {
		t.Fatalf("unexpected global running quota error: %+v", qe)
	}
}

func TestCheckQuota_StoppedDevPodsDoNotConsumeGlobalRunningQuota(t *testing.T) {
	c := fakeClient(t,
		mkDevPod("alice-x-a1", "alice", "x"),
		mkStoppedDevPod("bob-x-a1", "bob", "x"),
	)
	tpl := &models.DevPodTemplate{ID: "x", DefaultPerUser: 5, DefaultGlobal: 2}
	if err := CheckQuota(context.Background(), c, "carol", tpl); err != nil {
		t.Fatalf("stopped DevPod must not consume global running quota: %v", err)
	}
}

func TestCheckGlobalRunningQuota_ExcludesTarget(t *testing.T) {
	c := fakeClient(t,
		mkDevPod("alice-x-a1", "alice", "x"),
		mkDevPod("bob-x-a1", "bob", "x"),
	)
	if err := CheckGlobalRunningQuota(context.Background(), c, "x", 2, "alice-x-a1"); err != nil {
		t.Fatalf("target must be excluded from an idempotent start check: %v", err)
	}
}

func TestCheckQuota_OtherUsersDoNotCountForPerUser(t *testing.T) {
	c := fakeClient(t,
		mkDevPod("alice-x-a1", "alice", "x"),
		mkDevPod("bob-x-a1", "bob", "x"),
	)
	// alice has 1 pod; per-user limit 2 means she is under her own
	// quota. If bob's pod were incorrectly counted, alice would be at
	// 2/2 and blocked — which is exactly what this test guards against.
	tpl := &models.DevPodTemplate{ID: "x", DefaultPerUser: 2, DefaultGlobal: 5}
	if err := CheckQuota(context.Background(), c, "alice", tpl); err != nil {
		t.Errorf("bob's pod must not count against alice's per-user quota: %v", err)
	}
}

func TestCheckUserRunningQuota_AcrossClusters(t *testing.T) {
	clients := []*Client{
		fakeClient(t, mkDevPod("alice-x-a1", "alice", "x"), mkDevPod("bob-x-a1", "bob", "x"), mkStoppedDevPod("alice-z-a1", "alice", "z")),
		fakeClient(t, mkDevPod("alice-y-a1", "alice", "y")),
	}
	if err := CheckUserRunningQuota(context.Background(), clients, "alice", 3); err != nil {
		t.Fatalf("expected room under global per-user quota: %v", err)
	}
	err := CheckUserRunningQuota(context.Background(), clients, "alice", 2)
	qe, ok := err.(*QuotaError)
	if !ok {
		t.Fatalf("expected QuotaError, got %T (%v)", err, err)
	}
	if qe.Scope != "user_running" || qe.Used != 2 || qe.Limit != 2 {
		t.Fatalf("unexpected quota error: %+v", qe)
	}
}

func TestCheckUserRunningQuota_ZeroMeansUnlimited(t *testing.T) {
	c := fakeClient(t, mkDevPod("alice-x-a1", "alice", "x"))
	if err := CheckUserRunningQuota(context.Background(), []*Client{c}, "alice", 0); err != nil {
		t.Fatalf("zero limit must be unlimited: %v", err)
	}
}

// silence unused import in case metav1 is referenced later
var _ = metav1.GetOptions{}
