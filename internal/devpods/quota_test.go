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
	// alice has 1 pod; per-user limit 2 means she is under her own
	// quota. If bob's pod were incorrectly counted, alice would be at
	// 2/2 and blocked — which is exactly what this test guards against.
	tpl := &models.DevPodTemplate{ID: "x", DefaultPerUser: 2, DefaultGlobal: 5}
	if err := CheckQuota(context.Background(), c, "alice", tpl); err != nil {
		t.Errorf("bob's pod must not count against alice's per-user quota: %v", err)
	}
}

// silence unused import in case metav1 is referenced later
var _ = metav1.GetOptions{}
