package kubeutil

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

const DefaultGPUResource = "nvidia.com/gpu"

// NormalizeGPUResourceName validates a Kubernetes extended resource name.
// GPU device plugins advertise resources such as nvidia.com/gpu,
// amd.com/gpu, or nvidia.com/mig-1g.10gb.
func NormalizeGPUResourceName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultGPUResource
	}
	if !strings.Contains(name, "/") {
		return "", fmt.Errorf("GPU resource name %q must be a qualified extended resource such as %s", name, DefaultGPUResource)
	}
	if errors := validation.IsQualifiedName(name); len(errors) > 0 {
		return "", fmt.Errorf("invalid GPU resource name %q: %s", name, strings.Join(errors, "; "))
	}
	return name, nil
}
