package podspec

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type PodSpecInput struct {
	Name       string
	Namespace  string
	Image      string
	Script     string
	CPU        int
	MemoryMi   int64
	NodeSel    map[string]string
	Env        []corev1.EnvVar
	SubID      string
	Step       int
	AsRoot     bool
	Network    bool
	TimeoutSec int64
}

// BuildPodSpec constructs the *corev1.Pod for a single non-MPI workflow step.
func BuildPodSpec(in PodSpecInput) *corev1.Pod {
	uid := int64(1000)
	container := corev1.Container{
		Name:            "main",
		Image:           in.Image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-c", in.Script},
		Env:             in.Env,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewQuantity(int64(in.CPU), resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(in.MemoryMi*1024*1024, resource.BinarySI),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewQuantity(int64(in.CPU), resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(in.MemoryMi*1024*1024, resource.BinarySI),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "submission", MountPath: "/mnt/work", SubPath: in.SubID},
		},
	}
	if !in.AsRoot {
		container.SecurityContext = &corev1.SecurityContext{
			RunAsUser:  &uid,
			RunAsGroup: &uid,
		}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.Name,
			Namespace: in.Namespace,
			Labels: map[string]string{
				"app":             "csoj-judger",
				"csoj-submission": in.SubID,
				"csoj-step":       intToString(in.Step),
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:        corev1.RestartPolicyNever,
			Containers:           []corev1.Container{container},
			NodeSelector:         in.NodeSel,
			ActiveDeadlineSeconds: ptrInt64(in.TimeoutSec),
			Volumes: []corev1.Volume{
				{
					Name: "submission",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "csoj-submissions",
						},
					},
				},
			},
		},
	}
}

type MPIJobSpecInput struct {
	Name           string
	Namespace      string
	Image          string
	LauncherScript string
	WorkerReplicas int
	CPU            int
	MemoryMi       int64
	NodeSel        map[string]string
	SubID          string
	Step           int
}

// MPIJobGVR is the GroupVersionResource for the mpi-operator MPIJob CRD.
func MPIJobGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "kubeflow.org", Version: "v2beta1", Resource: "mpijobs"}
}

// BuildMPIJobSpec constructs an unstructured MPIJob object (launcher + workers).
// It returns a *unstructured.Unstructured so the dispatcher can Create it via the
// dynamic client without importing a specific CRD Go type.
func BuildMPIJobSpec(in MPIJobSpecInput) *unstructured.Unstructured {
	replicas := int64(in.WorkerReplicas)
	launcher, worker := mpiContainerTemplates(in)
	spec := map[string]interface{}{
		"mpiReplicaSpecs": map[string]interface{}{
			"Launcher": map[string]interface{}{
				"replicas": int64(1),
				"template": podTemplate(in, launcher),
			},
			"Worker": map[string]interface{}{
				"replicas": replicas,
				"template": podTemplate(in, worker),
			},
		},
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(MPIJobGVR().GroupVersion().WithKind("MPIJob"))
	obj.SetName(in.Name)
	obj.SetNamespace(in.Namespace)
	obj.SetLabels(map[string]string{
		"app":             "csoj-judger",
		"csoj-submission": in.SubID,
		"csoj-step":       intToString(in.Step),
	})
	obj.Object["spec"] = spec
	return obj
}

func mpiContainerTemplates(in MPIJobSpecInput) (corev1.Container, corev1.Container) {
	res := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    *resource.NewQuantity(int64(in.CPU), resource.DecimalSI),
			corev1.ResourceMemory: *resource.NewQuantity(in.MemoryMi*1024*1024, resource.BinarySI),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    *resource.NewQuantity(int64(in.CPU), resource.DecimalSI),
			corev1.ResourceMemory: *resource.NewQuantity(in.MemoryMi*1024*1024, resource.BinarySI),
		},
	}
	vm := corev1.VolumeMount{Name: "submission", MountPath: "/mnt/work", SubPath: in.SubID}
	launcher := corev1.Container{
		Name: "launcher", Image: in.Image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-c", in.LauncherScript},
		Resources:       res,
		VolumeMounts:    []corev1.VolumeMount{vm},
	}
	worker := corev1.Container{
		Name: "worker", Image: in.Image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"sleep", "infinity"},
		Resources:       res,
		VolumeMounts:    []corev1.VolumeMount{vm},
	}
	return launcher, worker
}

func podTemplate(in MPIJobSpecInput, c corev1.Container) map[string]interface{} {
	podSpec := corev1.PodSpec{
		RestartPolicy:   corev1.RestartPolicyNever,
		Containers:      []corev1.Container{c},
		NodeSelector:    in.NodeSel,
		Volumes: []corev1.Volume{{
			Name: "submission",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "csoj-submissions"},
			},
		}},
	}
	raw, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(&podSpec)
	tmpl := map[string]interface{}{"spec": raw}
	return tmpl
}

// helpers
func ptrInt64(v int64) *int64 { return &v }

func intToString(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		return "-" + string(b)
	}
	if len(b) == 0 {
		return "0"
	}
	return string(b)
}
