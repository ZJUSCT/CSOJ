package podspec

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestBuildPodSpec_Basic(t *testing.T) {
	pod := BuildPodSpec(PodSpecInput{
		Name:          "sub1-0",
		Namespace:     "csoj-judger",
		Image:         "gcc:13",
		Script:        "#!/bin/sh\nset -e\necho hi\n",
		CPURequest:    "2",
		CPULimit:      "2",
		MemoryRequest: "512Mi",
		MemoryLimit:   "512Mi",
		NodeSel:       map[string]string{"pool": "cpu"},
		Env:           []corev1.EnvVar{{Name: "CSOJ_SUBMIT_DIR", Value: "/mnt/work"}},
		SubID:         "sub1",
		Step:          0,
		AsRoot:        false,
		Network:       true,
		TimeoutSec:    30,
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

func TestBuildPodSpec_Burstable(t *testing.T) {
	pod := BuildPodSpec(PodSpecInput{
		Name:          "test",
		Namespace:     "ns",
		Image:         "img",
		Script:        "echo",
		CPURequest:    "500m",
		CPULimit:      "2",
		MemoryRequest: "256Mi",
		MemoryLimit:   "1Gi",
		SubID:         "s1",
		Step:          0,
		TimeoutSec:    30,
	})
	req := pod.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	if req.Cmp(resource.MustParse("500m")) != 0 {
		t.Errorf("cpu request: %s", req.String())
	}
	lim := pod.Spec.Containers[0].Resources.Limits[corev1.ResourceCPU]
	if lim.Cmp(resource.MustParse("2")) != 0 {
		t.Errorf("cpu limit: %s", lim.String())
	}
	memReq := pod.Spec.Containers[0].Resources.Requests[corev1.ResourceMemory]
	if memReq.Cmp(resource.MustParse("256Mi")) != 0 {
		t.Errorf("memory request: %s", memReq.String())
	}
	memLim := pod.Spec.Containers[0].Resources.Limits[corev1.ResourceMemory]
	if memLim.Cmp(resource.MustParse("1Gi")) != 0 {
		t.Errorf("memory limit: %s", memLim.String())
	}
}

func TestBuildPodSpec_Defaults(t *testing.T) {
	pod := BuildPodSpec(PodSpecInput{
		Name:       "test",
		Namespace:  "ns",
		Image:      "img",
		Script:     "echo",
		SubID:      "s1",
		Step:       0,
		TimeoutSec: 30,
	})
	req := pod.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU]
	if req.Cmp(resource.MustParse("1")) != 0 {
		t.Errorf("default cpu request: %s", req.String())
	}
	memReq := pod.Spec.Containers[0].Resources.Requests[corev1.ResourceMemory]
	if memReq.Cmp(resource.MustParse("256Mi")) != 0 {
		t.Errorf("default memory request: %s", memReq.String())
	}
	// Defaults apply to limits too.
	lim := pod.Spec.Containers[0].Resources.Limits[corev1.ResourceCPU]
	if lim.Cmp(resource.MustParse("1")) != 0 {
		t.Errorf("default cpu limit: %s", lim.String())
	}
}

func TestBuildPodSpec_Scheduling(t *testing.T) {
	pod := BuildPodSpec(PodSpecInput{
		Name:             "test",
		Namespace:        "ns",
		Image:            "img",
		Script:           "echo",
		SubID:            "s1",
		Step:             0,
		TimeoutSec:       30,
		NodeAffinity:     []NodeAffinityTerm{{Key: "cpu-manager", Operator: "In", Values: []string{"static"}}},
		Tolerations:      []Toleration{{Key: "dedicated", Operator: "Equal", Value: "cpu-pinning", Effect: "NoSchedule"}},
		PriorityClassName: "latency-critical",
		RuntimeClassName:  "runc",
	})
	if pod.Spec.Affinity == nil || pod.Spec.Affinity.NodeAffinity == nil {
		t.Fatal("expected node affinity")
	}
	terms := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) != 1 || len(terms[0].MatchExpressions) != 1 {
		t.Fatalf("node affinity terms: %+v", terms)
	}
	me := terms[0].MatchExpressions[0]
	if me.Key != "cpu-manager" || string(me.Operator) != "In" || len(me.Values) != 1 || me.Values[0] != "static" {
		t.Errorf("node affinity expr: %+v", me)
	}
	if len(pod.Spec.Tolerations) != 1 {
		t.Fatalf("tolerations: %v", pod.Spec.Tolerations)
	}
	tol := pod.Spec.Tolerations[0]
	if tol.Key != "dedicated" || string(tol.Operator) != "Equal" || tol.Value != "cpu-pinning" || string(tol.Effect) != "NoSchedule" {
		t.Errorf("toleration: %+v", tol)
	}
	if pod.Spec.PriorityClassName != "latency-critical" {
		t.Errorf("priority class: %s", pod.Spec.PriorityClassName)
	}
	if pod.Spec.RuntimeClassName == nil || *pod.Spec.RuntimeClassName != "runc" {
		t.Errorf("runtime class: %+v", pod.Spec.RuntimeClassName)
	}
}

func TestBuildPodSpec_NoScheduling(t *testing.T) {
	pod := BuildPodSpec(PodSpecInput{
		Name:       "test",
		Namespace:  "ns",
		Image:      "img",
		Script:     "echo",
		SubID:      "s1",
		Step:       0,
		TimeoutSec: 30,
	})
	if pod.Spec.Affinity != nil {
		t.Errorf("expected nil affinity, got %+v", pod.Spec.Affinity)
	}
	if pod.Spec.Tolerations != nil {
		t.Errorf("expected nil tolerations, got %+v", pod.Spec.Tolerations)
	}
	if pod.Spec.PriorityClassName != "" {
		t.Errorf("expected empty priority class, got %s", pod.Spec.PriorityClassName)
	}
	if pod.Spec.RuntimeClassName != nil {
		t.Errorf("expected nil runtime class, got %+v", pod.Spec.RuntimeClassName)
	}
}

func TestBuildMPIJobSpec_LauncherWorker(t *testing.T) {
	gvr := MPIJobGVR()
	if gvr.Resource != "mpijobs" || gvr.Group != "kubeflow.org" {
		t.Errorf("GVR: %v", gvr)
	}
	obj := BuildMPIJobSpec(MPIJobSpecInput{
		Name:           "sub1-1",
		Namespace:      "csoj-judger",
		Image:          "openmpi:4",
		LauncherScript: "mpirun -np 4 ./a.out",
		WorkerReplicas: 2,
		CPURequest:     "2",
		CPULimit:       "2",
		MemoryRequest:  "1Gi",
		MemoryLimit:    "1Gi",
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
	// Launcher resources.
	lim := launcher.Resources.Limits[corev1.ResourceCPU]
	if lim.Cmp(resource.MustParse("2")) != 0 {
		t.Errorf("launcher cpu limit: %s", lim.String())
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
