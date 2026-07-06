package podspec

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestBuildPodSpec_Basic(t *testing.T) {
	pod := buildPodSpec(PodSpecInput{
		Name:       "sub1-0",
		Namespace:  "csoj-judger",
		Image:      "gcc:13",
		Script:     "#!/bin/sh\nset -e\necho hi\n",
		CPU:        2,
		MemoryMi:   512,
		NodeSel:    map[string]string{"pool": "cpu"},
		Env:        []corev1.EnvVar{{Name: "CSOJ_SUBMIT_DIR", Value: "/mnt/work"}},
		SubID:      "sub1",
		Step:       0,
		AsRoot:     false,
		Network:    true,
		TimeoutSec: 30,
	})
	if pod.Name != "sub1-0" {
		t.Errorf("name: %s", pod.Name)
	}
	if pod.Spec.Containers[0].Image != "gcc:13" {
		t.Errorf("image: %s", pod.Spec.Containers[0].Image)
	}
	// Non-root: runAsUser 1000.
	uid := int64(1000)
	if pod.Spec.Containers[0].SecurityContext.RunAsUser == nil || *pod.Spec.Containers[0].SecurityContext.RunAsUser != uid {
		t.Errorf("expected runAsUser 1000 for non-root step")
	}
	// Resources: cpu=2, memory=512Mi on both requests and limits.
	lim := pod.Spec.Containers[0].Resources.Limits[corev1.ResourceCPU]
	if lim.Cmp(resource.MustParse("2")) != 0 {
		t.Errorf("cpu limit: %s", lim.String())
	}
	if pod.Spec.Containers[0].Resources.Limits[corev1.ResourceMemory] != resource.MustParse("512Mi") {
		t.Errorf("memory limit not 512Mi")
	}
	// nodeSelector.
	if pod.Spec.NodeSelector["pool"] != "cpu" {
		t.Errorf("nodeSelector: %v", pod.Spec.NodeSelector)
	}
	// subPath volume mount at /mnt/work.
	found := false
	for _, vm := range pod.Spec.Containers[0].VolumeMounts {
		if vm.MountPath == "/mnt/work" && vm.SubPath == "sub1" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected /mnt/work mount with subPath sub1")
	}
	// Labels.
	if pod.Labels["csoj-submission"] != "sub1" || pod.Labels["csoj-step"] != "0" {
		t.Errorf("labels: %v", pod.Labels)
	}
}

func TestBuildMPIJobSpec_LauncherWorker(t *testing.T) {
	gvr := MPIJobGVR()
	if gvr.Resource != "mpijobs" || gvr.Group != "kubeflow.org" {
		t.Errorf("GVR: %v", gvr)
	}
	obj := buildMPIJobSpec(MPIJobSpecInput{
		Name:           "sub1-1",
		Namespace:      "csoj-judger",
		Image:          "openmpi:4",
		LauncherScript: "mpirun -np 4 ./a.out",
		WorkerReplicas: 2,
		CPU:            2,
		MemoryMi:       1024,
		NodeSel:        map[string]string{"pool": "gpu"},
		SubID:          "sub1",
		Step:           1,
	})
	// Launcher command is the mpirun script.
	launcher := getReplicaContainer(obj, "Launcher")
	if launcher == nil {
		t.Fatal("no launcher container")
	}
	if launcher.Image != "openmpi:4" {
		t.Errorf("launcher image: %s", launcher.Image)
	}
	if len(launcher.Command) < 2 || launcher.Command[0] != "/bin/sh" || launcher.Command[1] != "-c" {
		t.Errorf("launcher command: %v", launcher.Command)
	}
	// Worker: sleep infinity.
	worker := getReplicaContainer(obj, "Worker")
	if worker == nil {
		t.Fatal("no worker container")
	}
	if len(worker.Command) != 2 || worker.Command[0] != "sleep" || worker.Command[1] != "infinity" {
		t.Errorf("worker command: %v", worker.Command)
	}
	// Labels.
	if obj.GetLabels()["csoj-step"] != "1" {
		t.Errorf("labels: %v", obj.GetLabels())
	}
}

func getReplicaContainer(obj *unstructured.Unstructured, role string) *corev1.Container {
	specs, ok, _ := unstructured.NestedMap(obj.Object, "spec", "mpiReplicaSpecs", role)
	if !ok {
		return nil
	}
	tmpl, ok, _ := unstructured.NestedMap(specs, "template", "spec")
	if !ok {
		return nil
	}
	var ps corev1.PodSpec
	_ = runtime.DefaultUnstructuredConverter.FromUnstructured(tmpl, &ps)
	if len(ps.Containers) == 0 {
		return nil
	}
	return &ps.Containers[0]
}
