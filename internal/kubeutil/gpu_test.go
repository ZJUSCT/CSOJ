package kubeutil

import "testing"

func TestNormalizeGPUResourceName(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{"", DefaultGPUResource, false},
		{"nvidia.com/gpu", "nvidia.com/gpu", false},
		{"amd.com/gpu", "amd.com/gpu", false},
		{"nvidia.com/mig-1g.10gb", "nvidia.com/mig-1g.10gb", false},
		{"gpu", "", true},
		{"NVIDIA.COM/gpu", "", true},
	}
	for _, test := range tests {
		got, err := NormalizeGPUResourceName(test.name)
		if (err != nil) != test.wantErr {
			t.Errorf("NormalizeGPUResourceName(%q) error = %v, wantErr %v", test.name, err, test.wantErr)
		}
		if got != test.want {
			t.Errorf("NormalizeGPUResourceName(%q) = %q, want %q", test.name, got, test.want)
		}
	}
}
