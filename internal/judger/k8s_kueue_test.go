package judger

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestProbeKueueSupportedVersions(t *testing.T) {
	for _, groupVersion := range []string{"kueue.x-k8s.io/v1", "kueue.x-k8s.io/v1beta1"} {
		t.Run(groupVersion, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			discovery := client.Discovery().(*fakediscovery.FakeDiscovery)
			discovery.Resources = []*metav1.APIResourceList{{
				GroupVersion: groupVersion,
				APIResources: []metav1.APIResource{{Name: "workloads", Kind: "Workload"}},
			}}
			if !probeKueue(client) {
				t.Fatalf("probeKueue did not detect Workload in %s", groupVersion)
			}
		})
	}
}

func TestProbeKueueMissing(t *testing.T) {
	if probeKueue(fake.NewSimpleClientset()) {
		t.Fatal("probeKueue detected Kueue without a Workload resource")
	}
}
