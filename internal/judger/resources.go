package judger

import (
	"fmt"

	"github.com/ZJUSCT/CSOJ/internal/kubeutil"
)

// NormalizeProblemGPUResources validates and normalizes GPU configuration on
// every workflow step before an admin-created problem is persisted.
func NormalizeProblemGPUResources(problem *Problem) error {
	for i := range problem.Workflow {
		resources := problem.Workflow[i].Resources
		if resources == nil {
			continue
		}
		if resources.GPUCount < 0 {
			return fmt.Errorf("workflow step %d: gpu_count must be non-negative", i+1)
		}
		if resources.GPUCount == 0 {
			continue
		}
		name, err := kubeutil.NormalizeGPUResourceName(resources.GPUResource)
		if err != nil {
			return fmt.Errorf("workflow step %d: %w", i+1, err)
		}
		resources.GPUResource = name
	}
	return nil
}
