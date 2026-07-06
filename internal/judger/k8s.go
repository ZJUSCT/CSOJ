package judger

import (
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
)

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
