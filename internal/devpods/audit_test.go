package devpods

import (
	"strings"
	"testing"
)

func TestParseGatewayAuditLog(t *testing.T) {
	log := strings.Join([]string{
		`{"time":"2026-07-12T10:00:00Z","msg":"session_open","session_id":"sid-1","user":"h3240104995","devpod":"m7-debb9c0","client_ip":"192.0.2.1:1234","auth_path":"trusted_proxy"}`,
		`{"time":"2026-07-12T10:01:00Z","msg":"dial_failed","id":1,"endpoint":"10.0.0.1:22","err":"connection refused"}`,
		`{"time":"2026-07-12T10:02:00Z","msg":"session_close","session_id":"sid-1","duration_seconds":120,"bytes_in":10,"bytes_out":20,"close_reason":"client_disconnect"}`,
		`{"time":"2026-07-12T10:03:00Z","msg":"auth_failure","user":"h3240104995","devpod":"h3240104995-m7-debb9c0","reason":"pubkey_mismatch","auth_path":"direct"}`,
		`{"time":"2026-07-12T10:04:00Z","msg":"auth_failure","user":"h0000000000","devpod":"m7-debb9c0","reason":"pubkey_mismatch"}`,
		`{"time":"2026-07-12T10:05:00Z","msg":"session_open","session_id":"sid-2","user":"h3240104995","devpod":"another-pod","client_ip":"192.0.2.2:1234"}`,
	}, "\n")

	events := parseGatewayAuditLog(strings.NewReader(log), "gateway-0", "h3240104995-m7-debb9c0", "h3240104995")
	if len(events) != 4 {
		t.Fatalf("got %d events, want 4: %+v", len(events), events)
	}
	wantReasons := []string{"SessionOpen", "DialFailed", "SessionClose", "AuthRejected"}
	for i, want := range wantReasons {
		if events[i].Reason != want {
			t.Errorf("event[%d].Reason = %q, want %q", i, events[i].Reason, want)
		}
		if events[i].ObjectName != "h3240104995-m7-debb9c0" {
			t.Errorf("event[%d].ObjectName = %q", i, events[i].ObjectName)
		}
		if events[i].Source != "devpod-gateway/audit" {
			t.Errorf("event[%d].Source = %q", i, events[i].Source)
		}
	}
}

func TestAuditTargetMatches(t *testing.T) {
	const owner = "h3240104995"
	const fullName = "h3240104995-m7-debb9c0"
	const shortName = "m7-debb9c0"
	for _, pod := range []string{shortName, fullName} {
		if !auditTargetMatches(owner, pod, owner, fullName, shortName) {
			t.Errorf("expected pod %q to match", pod)
		}
	}
	if auditTargetMatches("another-user", shortName, owner, fullName, shortName) {
		t.Error("unexpected different owner match")
	}
	if auditTargetMatches(owner, "another-pod", owner, fullName, shortName) {
		t.Error("unexpected different DevPod match")
	}
}
