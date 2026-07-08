package judger

import (
	"bufio"
	"context"
	"fmt"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/judger/podspec"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// ptrInt64 returns a pointer to v. Used by the judger package (KubeManager,
// recovery) for K8s API fields like *int64 (GracePeriodSeconds,
// ActiveDeadlineSeconds). The podspec package has its own copy.
func ptrInt64(v int64) *int64 { return &v }

// probeMPIOperator checks whether the mpi-operator CRD is installed in the cluster.
func probeMPIOperator(cs kubernetes.Interface, ns string) bool {
	apiRes, err := cs.Discovery().ServerResourcesForGroupVersion("kubeflow.org/v2beta1")
	if err != nil {
		zap.S().Warnf("MPI operator CRD not detected (kubeflow.org/v2beta1): %v — MPI steps will fail", err)
		return false
	}
	for _, r := range apiRes.APIResources {
		if r.Kind == "MPIJob" {
			return true
		}
	}
	zap.S().Warnf("MPIJob kind not found in kubeflow.org/v2beta1; MPI steps will fail")
	return false
}

// probeKueue checks whether the Kueue CRD (Workload kind in kueue.x-k8s.io/v1)
// is installed in the cluster. Used to validate the cluster's queue_mode=kueue
// config at startup and on reload.
func probeKueue(cs kubernetes.Interface) bool {
	apiRes, err := cs.Discovery().ServerResourcesForGroupVersion("kueue.x-k8s.io/v1")
	if err != nil {
		return false
	}
	for _, r := range apiRes.APIResources {
		if r.Kind == "Workload" {
			return true
		}
	}
	return false
}

// KubeManager wraps a cluster's clientset + dynamic client for judger operations.
type KubeManager struct {
	cs  kubernetes.Interface
	dyn dynamic.Interface
	ns  string
}

func NewKubeManager(cs kubernetes.Interface, dyn dynamic.Interface, ns string) *KubeManager {
	return &KubeManager{cs: cs, dyn: dyn, ns: ns}
}

// CreatePod creates the pod and returns its name.
func (k *KubeManager) CreatePod(ctx context.Context, pod *corev1.Pod) error {
	_, err := k.cs.CoreV1().Pods(k.ns).Create(ctx, pod, metav1.CreateOptions{})
	return err
}

// WaitForPod waits for the pod to reach a terminal phase (Succeeded/Failed) or ctx cancel.
// Returns the terminal phase.
func (k *KubeManager) WaitForPod(ctx context.Context, name string) (corev1.PodPhase, error) {
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		p, err := k.cs.CoreV1().Pods(k.ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
			return p.Status.Phase, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// StreamPodLogs tails the pod's "main" container logs, calling onChunk for each
// chunk of output. Blocks until the stream closes (pod completion) or ctx cancel.
func (k *KubeManager) StreamPodLogs(ctx context.Context, name string, onChunk func(string)) error {
	opts := &corev1.PodLogOptions{Follow: true, Container: "main"}
	stream, err := k.cs.CoreV1().Pods(k.ns).GetLogs(name, opts).Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		onChunk(scanner.Text() + "\n")
	}
	return scanner.Err()
}

// DeletePod force-deletes a single pod.
func (k *KubeManager) DeletePod(ctx context.Context, name string) error {
	return k.cs.CoreV1().Pods(k.ns).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: ptrInt64(0),
	})
}

// DeleteSubmissionPods deletes all pods labeled csoj-submission=<subID>.
func (k *KubeManager) DeleteSubmissionPods(ctx context.Context, subID string) error {
	return k.cs.CoreV1().Pods(k.ns).DeleteCollection(ctx, metav1.DeleteOptions{
		GracePeriodSeconds: ptrInt64(0),
	}, metav1.ListOptions{LabelSelector: fmt.Sprintf("csoj-submission=%s", subID)})
}

// CreateMPIJob creates an MPIJob via the dynamic client.
func (k *KubeManager) CreateMPIJob(ctx context.Context, obj *unstructured.Unstructured) error {
	gvr := schema.GroupVersionResource{Group: "kubeflow.org", Version: "v2beta1", Resource: "mpijobs"}
	_, err := k.dyn.Resource(gvr).Namespace(k.ns).Create(ctx, obj, metav1.CreateOptions{})
	return err
}

// WaitForMPIJob waits for the MPIJob's launcher to reach Succeeded/Failed.
// It polls the .status.replicaStatuses.Launcher field.
func (k *KubeManager) WaitForMPIJob(ctx context.Context, name string) (corev1.PodPhase, error) {
	gvr := schema.GroupVersionResource{Group: "kubeflow.org", Version: "v2beta1", Resource: "mpijobs"}
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		obj, err := k.dyn.Resource(gvr).Namespace(k.ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		launcher, _, _ := unstructured.NestedMap(obj.Object, "status", "replicaStatuses", "Launcher")
		if launcher != nil {
			if s, ok := launcher["succeeded"].(int64); ok && s > 0 {
				return corev1.PodSucceeded, nil
			}
			if f, ok := launcher["failed"].(int64); ok && f > 0 {
				return corev1.PodFailed, nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// DeleteSubmissionMPIJobs deletes all MPIJobs labeled csoj-submission=<subID>.
func (k *KubeManager) DeleteSubmissionMPIJobs(ctx context.Context, subID string) error {
	gvr := schema.GroupVersionResource{Group: "kubeflow.org", Version: "v2beta1", Resource: "mpijobs"}
	return k.dyn.Resource(gvr).Namespace(k.ns).DeleteCollection(ctx, metav1.DeleteOptions{
		GracePeriodSeconds: ptrInt64(0),
	}, metav1.ListOptions{LabelSelector: fmt.Sprintf("csoj-submission=%s", subID)})
}

// ListJudgerPods lists pods labeled app=csoj-judger (for recovery).
func (k *KubeManager) ListJudgerPods(ctx context.Context) ([]string, error) {
	list, err := k.cs.CoreV1().Pods(k.ns).List(ctx, metav1.ListOptions{LabelSelector: "app=csoj-judger"})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list.Items))
	for _, p := range list.Items {
		names = append(names, p.Name)
	}
	return names, nil
}

// ListJudgerMPIJobs lists MPIJobs labeled app=csoj-judger.
func (k *KubeManager) ListJudgerMPIJobs(ctx context.Context) ([]string, error) {
	gvr := schema.GroupVersionResource{Group: "kubeflow.org", Version: "v2beta1", Resource: "mpijobs"}
	list, err := k.dyn.Resource(gvr).Namespace(k.ns).List(ctx, metav1.ListOptions{LabelSelector: "app=csoj-judger"})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		names = append(names, item.GetName())
	}
	return names, nil
}

// DeleteAllJudgerPods force-deletes all pods labeled app=csoj-judger (for recovery).
func (k *KubeManager) DeleteAllJudgerPods(ctx context.Context) error {
	return k.cs.CoreV1().Pods(k.ns).DeleteCollection(ctx, metav1.DeleteOptions{
		GracePeriodSeconds: ptrInt64(0),
	}, metav1.ListOptions{LabelSelector: "app=csoj-judger"})
}

// DeleteAllJudgerMPIJobs force-deletes all MPIJobs labeled app=csoj-judger (for recovery).
func (k *KubeManager) DeleteAllJudgerMPIJobs(ctx context.Context) error {
	return k.dyn.Resource(podspec.MPIJobGVR()).Namespace(k.ns).DeleteCollection(ctx, metav1.DeleteOptions{
		GracePeriodSeconds: ptrInt64(0),
	}, metav1.ListOptions{LabelSelector: "app=csoj-judger"})
}
