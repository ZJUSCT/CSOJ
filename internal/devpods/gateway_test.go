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
	gw := Gateway{Host: "devpod.example.com", Port: 2222, AuditNamespace: "gateway-system"}
	if err := SetGateway(s, gw); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := GetGateway(s)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Host != "devpod.example.com" || got.Port != 2222 || got.AuditNamespace != "gateway-system" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestSSHCommand(t *testing.T) {
	gw := Gateway{Host: "devpod.example.com", Port: 2222}
	got := gw.SSHCommand("alice", "alice-gpu-8ca1b2")
	want := "ssh alice+gpu-8ca1b2@devpod.example.com -p 2222"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSSHCommand_LegacyBareDevPodName(t *testing.T) {
	gw := Gateway{Host: "devpod.example.com", Port: 2222}
	got := gw.SSHCommand("alice", "hello")
	want := "ssh alice+hello@devpod.example.com -p 2222"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSSHCommand_WithHostnameSuffix(t *testing.T) {
	gw := Gateway{Host: "clusters.zju.edu.cn", Port: 443, HostnameSuffix: "hpc101"}
	got := gw.SSHCommand("h3240104995", "h3240104995-m7-debb9c0")
	want := "ssh h3240104995+m7-debb9c0+hpc101@clusters.zju.edu.cn -p 443"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestGatewayValidate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		gateway Gateway
		valid   bool
	}{
		{name: "valid", gateway: Gateway{Host: "devpod.example.com", Port: 22}, valid: true},
		{name: "valid audit namespace", gateway: Gateway{Host: "devpod.example.com", Port: 22, AuditNamespace: "devpod-system"}, valid: true},
		{name: "empty host", gateway: Gateway{Port: 22}},
		{name: "blank host", gateway: Gateway{Host: "  ", Port: 22}},
		{name: "zero port", gateway: Gateway{Host: "devpod.example.com"}},
		{name: "port too large", gateway: Gateway{Host: "devpod.example.com", Port: 65536}},
		{name: "valid hostname suffix", gateway: Gateway{Host: "clusters.zju.edu.cn", Port: 443, HostnameSuffix: "hpc101"}, valid: true},
		{name: "invalid hostname suffix", gateway: Gateway{Host: "clusters.zju.edu.cn", Port: 443, HostnameSuffix: "HPC+101"}},
		{name: "invalid audit namespace", gateway: Gateway{Host: "clusters.zju.edu.cn", Port: 443, AuditNamespace: "DevPod_System"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.gateway.Validate()
			if tc.valid && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatal("Validate() unexpectedly succeeded")
			}
		})
	}
}

func TestGatewayEffectiveAuditNamespace(t *testing.T) {
	if got := (Gateway{}).EffectiveAuditNamespace(); got != DefaultGatewayAuditNamespace {
		t.Fatalf("empty audit namespace = %q, want %q", got, DefaultGatewayAuditNamespace)
	}
	if got := (Gateway{AuditNamespace: "gateway-system"}).EffectiveAuditNamespace(); got != "gateway-system" {
		t.Fatalf("custom audit namespace = %q, want gateway-system", got)
	}
}
