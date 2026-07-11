package judger

import (
	"testing"

	"github.com/ZJUSCT/CSOJ/internal/kubeutil"
)

func TestNormalizeProblemGPUResources(t *testing.T) {
	problem := &Problem{Workflow: []WorkflowStep{{Resources: &StepResources{GPUCount: 1}}}}
	if err := NormalizeProblemGPUResources(problem); err != nil {
		t.Fatalf("normalize default GPU resource: %v", err)
	}
	if got := problem.Workflow[0].Resources.GPUResource; got != kubeutil.DefaultGPUResource {
		t.Fatalf("GPU resource = %q, want %q", got, kubeutil.DefaultGPUResource)
	}

	problem.Workflow[0].Resources.GPUCount = -1
	if err := NormalizeProblemGPUResources(problem); err == nil {
		t.Fatal("negative GPU count must be rejected")
	}

	problem.Workflow[0].Resources.GPUCount = 1
	problem.Workflow[0].Resources.GPUResource = "gpu"
	if err := NormalizeProblemGPUResources(problem); err == nil {
		t.Fatal("unqualified GPU resource name must be rejected")
	}
}
