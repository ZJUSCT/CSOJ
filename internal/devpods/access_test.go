package devpods

import (
	"reflect"
	"testing"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
)

func TestNormalizeAllowedTags(t *testing.T) {
	got := NormalizeAllowedTags(models.StringArray{" gpu ", "", "hpc", "gpu"})
	want := models.StringArray{"gpu", "hpc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeAllowedTags() = %v, want %v", got, want)
	}
}

func TestTemplateAllowedForUser(t *testing.T) {
	tests := []struct {
		name        string
		userTags    string
		allowedTags models.StringArray
		want        bool
	}{
		{name: "public template", userTags: "", allowedTags: nil, want: true},
		{name: "one matching tag", userTags: "student, gpu", allowedTags: models.StringArray{"gpu", "hpc"}, want: true},
		{name: "trim user tags", userTags: " student , hpc ", allowedTags: models.StringArray{"hpc"}, want: true},
		{name: "no matching tag", userTags: "student", allowedTags: models.StringArray{"gpu", "hpc"}, want: false},
		{name: "exact tag match", userTags: "gpu-user", allowedTags: models.StringArray{"gpu"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TemplateAllowedForUser(tt.userTags, tt.allowedTags); got != tt.want {
				t.Fatalf("TemplateAllowedForUser(%q, %v) = %v, want %v", tt.userTags, tt.allowedTags, got, tt.want)
			}
		})
	}
}
