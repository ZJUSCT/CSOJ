package database_test

import (
	"encoding/json"
	"testing"

	"github.com/ZJUSCT/CSOJ/internal/database"
	"github.com/ZJUSCT/CSOJ/internal/database/models"
)

func TestDevPodTemplate_CRUD(t *testing.T) {
	db, err := database.Init(":memory:")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	tpl := &models.DevPodTemplate{
		ID: "gpu-8c", Name: "8c GPU", ClusterName: "c1", Image: "ubuntu:24.04",
		Cores: 8, Memory: 16 << 30, NodeSelector: models.JSONMap{"numa-node": "0"},
		GPUCount:       2,
		GPUResource:    "nvidia.com/gpu",
		AllowedTags:    models.StringArray{"gpu", "hpc"},
		Tolerations:    models.RawJSON(`[{"key":"dedicated","operator":"Equal","value":"gpu","effect":"NoSchedule"}]`),
		DefaultPerUser: 1, DefaultGlobal: 5,
	}
	if err := database.CreateDevPodTemplate(db, tpl); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := database.GetDevPodTemplate(db, "gpu-8c")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Cores != 8 {
		t.Errorf("cores = %d, want 8", got.Cores)
	}
	if got.GPUCount != 2 || got.GPUResource != "nvidia.com/gpu" {
		t.Errorf("GPU config not round-tripped: count=%d resource=%q", got.GPUCount, got.GPUResource)
	}
	if len(got.AllowedTags) != 2 || got.AllowedTags[0] != "gpu" || got.AllowedTags[1] != "hpc" {
		t.Errorf("allowed_tags not round-tripped: %v", got.AllowedTags)
	}
	if got.NodeSelector["numa-node"] != "0" {
		t.Errorf("numa selector not round-tripped: %v", got.NodeSelector)
	}
	if got.DefaultGlobal != 5 {
		t.Errorf("default_global = %d, want 5", got.DefaultGlobal)
	}
	var tolerations []map[string]string
	if err := json.Unmarshal(got.Tolerations, &tolerations); err != nil {
		t.Fatalf("unmarshal tolerations: %v", err)
	}
	if len(tolerations) != 1 || tolerations[0]["key"] != "dedicated" || tolerations[0]["effect"] != "NoSchedule" {
		t.Errorf("tolerations not round-tripped: %v", tolerations)
	}
	got.DefaultPerUser = 3
	if err := database.UpdateDevPodTemplate(db, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, err := database.GetDevPodTemplate(db, "gpu-8c")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got2.DefaultPerUser != 3 {
		t.Errorf("update did not persist")
	}
	rows, err := database.ListDevPodTemplates(db)
	if err != nil || len(rows) != 1 {
		t.Errorf("list: %v len=%d", err, len(rows))
	}
	if err := database.DeleteDevPodTemplate(db, "gpu-8c"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := database.GetDevPodTemplate(db, "gpu-8c"); err == nil {
		t.Errorf("delete did not remove row")
	}
}

func TestValidateDevPodTemplateID(t *testing.T) {
	good := []string{"a", "gpu-8c", "x-y-z"}
	bad := []string{"", "GPU", "toolongid", "gpu_8c", "gpu.8c"}
	for _, id := range good {
		if !database.ValidateDevPodTemplateID(id) {
			t.Errorf("expected %q valid", id)
		}
	}
	for _, id := range bad {
		if database.ValidateDevPodTemplateID(id) {
			t.Errorf("expected %q invalid", id)
		}
	}
}
