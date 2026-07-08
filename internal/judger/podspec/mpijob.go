package podspec

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NodeAffinityTerm mirrors judger.NodeAffinityTerm in the podspec package.
// A node-affinity match expression: key + operator (In, NotIn, Exists, etc.)
// + values.
type NodeAffinityTerm struct {
	Key      string
	Operator string
	Values   []string
}

// Toleration mirrors judger.Toleration in the podspec package.
type Toleration struct {
	Key      string
	Operator string
	Value    string
	Effect   string
}

type PodSpecInput struct {
	Name              string
	Namespace         string
	Image             string
	Script            string
	CPURequest        string
	CPULimit          string
	MemoryRequest     string
	MemoryLimit       string
	NodeSel           map[string]string
	NodeAffinity      []NodeAffinityTerm
	Tolerations       []Toleration
	PriorityClassName string
	RuntimeClassName  string
	Env               []corev1.EnvVar
	SubID             string
	Step              int
	AsRoot            bool
	Network           bool
	TimeoutSec        int64
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
				corev1.ResourceCPU:    parseResource(in.CPURequest, "1"),
				corev1.ResourceMemory: parseResource(in.MemoryRequest, "256Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    parseResource(in.CPULimit, "1"),
				corev1.ResourceMemory: parseResource(in.MemoryLimit, "256Mi"),
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
			RestartPolicy:         corev1.RestartPolicyNever,
			Containers:            []corev1.Container{container},
			NodeSelector:          in.NodeSel,
			Affinity:              buildNodeAffinity(in.NodeAffinity),
			Tolerations:           buildTolerations(in.Tolerations),
			PriorityClassName:     in.PriorityClassName,
			RuntimeClassName:      runtimeClassNamePtr(in.RuntimeClassName),
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
	Name              string
	Namespace         string
	Image             string
	LauncherScript    string
	WorkerReplicas    int
	CPURequest        string
	CPULimit          string
	MemoryRequest     string
	MemoryLimit       string
	NodeSel           map[string]string
	NodeAffinity      []NodeAffinityTerm
	Tolerations       []Toleration
	PriorityClassName string
	RuntimeClassName  string
	SubID             string
	Step              int
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
			corev1.ResourceCPU:    parseResource(in.CPURequest, "1"),
			corev1.ResourceMemory: parseResource(in.MemoryRequest, "256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    parseResource(in.CPULimit, "1"),
			corev1.ResourceMemory: parseResource(in.MemoryLimit, "256Mi"),
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
		RestartPolicy:     corev1.RestartPolicyNever,
		Containers:        []corev1.Container{c},
		NodeSelector:      in.NodeSel,
		Affinity:          buildNodeAffinity(in.NodeAffinity),
		Tolerations:       buildTolerations(in.Tolerations),
		PriorityClassName: in.PriorityClassName,
		RuntimeClassName:  runtimeClassNamePtr(in.RuntimeClassName),
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

// parseResource parses a K8s resource quantity string (e.g. "2", "500m", "1Gi"),
// falling back to `fallback` when s is empty. Panics on invalid input — callers
// (admin/frontend) are responsible for validating strings before persistence.
func parseResource(s, fallback string) resource.Quantity {
	if s == "" {
		return resource.MustParse(fallback)
	}
	return resource.MustParse(s)
}

// buildNodeAffinity translates the podspec NodeAffinityTerm list into a
// *corev1.Affinity with required node affinity. Returns nil when no terms.
func buildNodeAffinity(terms []NodeAffinityTerm) *corev1.Affinity {
	if len(terms) == 0 {
		return nil
	}
	var nodeSelectorTerms []corev1.NodeSelectorTerm
	for _, t := range terms {
		vals := t.Values
		if vals == nil {
			vals = []string{}
		}
		nodeSelectorTerms = append(nodeSelectorTerms, corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key:      t.Key,
				Operator: corev1.NodeSelectorOperator(t.Operator),
				Values:   vals,
			}},
		})
	}
	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: nodeSelectorTerms,
			},
		},
	}
}

// buildTolerations translates the podspec Toleration list into corev1.Toleration
// entries. Returns nil when no tolerations (so the field is omitted from the Pod).
func buildTolerations(tols []Toleration) []corev1.Toleration {
	if len(tols) == 0 {
		return nil
	}
	result := make([]corev1.Toleration, 0, len(tols))
	for _, t := range tols {
		result = append(result, corev1.Toleration{
			Key:      t.Key,
			Operator: corev1.TolerationOperator(t.Operator),
			Value:    t.Value,
			Effect:   corev1.TaintEffect(t.Effect),
		})
	}
	return result
}

// runtimeClassNamePtr returns a pointer to name when non-empty, else nil (so the
// runtimeClassName field is omitted from the Pod spec).
func runtimeClassNamePtr(name string) *string {
	if name == "" {
		return nil
	}
	return &name
}

// helpers
func ptrInt64(v int64) *int64 { return &v }

func ptrInt32(v int32) *int32 { return &v }

// JobSpecInput wraps PodSpecInput with a Kueue queue name. When QueueName is
// non-empty, the resulting Job gets a `kueue.x-k8s.io/queue-name` label so
// Kueue's webhook admits it into the named LocalQueue.
type JobSpecInput struct {
	PodSpecInput
	QueueName string
}

// BuildJobSpec wraps the PodSpec built from in.PodSpecInput into a batchv1.Job
// suitable for Kueue admission. The Job's pod template carries the same labels
// as the Pod (app=csoj-judger, csoj-submission, csoj-step), plus the Kueue
// queue-name label when QueueName is set. BackoffLimit=0 (no retries),
// TTLSecondsAfterFinished=60 (auto-cleanup), ActiveDeadlineSeconds from the
// step timeout.
func BuildJobSpec(in JobSpecInput) *batchv1.Job {
	pod := BuildPodSpec(in.PodSpecInput)
	labels := make(map[string]string, len(pod.Labels)+1)
	for k, v := range pod.Labels {
		labels[k] = v
	}
	if in.QueueName != "" {
		labels["kueue.x-k8s.io/queue-name"] = in.QueueName
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.Name,
			Namespace: in.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: pod.Labels,
				},
				Spec: pod.Spec,
			},
			BackoffLimit:            ptrInt32(0),
			TTLSecondsAfterFinished: ptrInt32(60),
			ActiveDeadlineSeconds:   ptrInt64(in.TimeoutSec),
		},
	}
}

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
