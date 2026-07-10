package devpods

import (
	"testing"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/ZJUSCT/CSOJ/internal/database"
)

func TestGatewaySetting_RoundTrip(t *testing.T) {
	db, err := database.Init(":memory:")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	s := config.NewSettingsStore(db)
	gw := Gateway{Host: "devpod.example.com", Port: 2222}
	if err := SetGateway(s, gw); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := GetGateway(s)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Host != "devpod.example.com" || got.Port != 2222 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestSSHCommand(t *testing.T) {
	gw := Gateway{Host: "devpod.example.com", Port: 2222}
	got := gw.SSHCommand("alice", "alice-gpu-8c-a1b2")
	want := "ssh alice+alice-gpu-8c-a1b2@devpod.example.com -p 2222"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
