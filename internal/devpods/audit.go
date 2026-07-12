package devpods

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const gatewayPodSelector = "app.kubernetes.io/name=devpod-gateway"

type gatewayAuditRecord struct {
	Time            time.Time       `json:"time"`
	Message         string          `json:"msg"`
	ID              json.RawMessage `json:"id"`
	SessionID       string          `json:"session_id"`
	User            string          `json:"user"`
	DevPod          string          `json:"devpod"`
	ClientIP        string          `json:"client_ip"`
	AuthPath        string          `json:"auth_path"`
	Reason          string          `json:"reason"`
	CloseReason     string          `json:"close_reason"`
	LastSourceError string          `json:"last_source_err"`
	Endpoint        string          `json:"endpoint"`
	Error           string          `json:"err"`
	DurationSeconds float64         `json:"duration_seconds"`
	BytesIn         int64           `json:"bytes_in"`
	BytesOut        int64           `json:"bytes_out"`
}

type auditSession struct {
	user string
}

// ListGatewayAuditEvents reads bounded recent JSON logs from every current
// devpod-gateway Pod and converts matching audit rows into the Event shape used
// by the UI. Failures are warnings so ordinary Kubernetes Events remain usable.
func ListGatewayAuditEvents(ctx context.Context, client kubernetes.Interface, namespace, devPodName, owner string) ([]Event, []string) {
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: gatewayPodSelector})
	if err != nil {
		return nil, []string{fmt.Sprintf("read DevPod gateway audit Pods in namespace %q: %v", namespace, err)}
	}
	if len(pods.Items) == 0 {
		return nil, []string{fmt.Sprintf("no DevPod gateway Pods found in namespace %q", namespace)}
	}

	events := make([]Event, 0)
	warnings := make([]string, 0)
	tailLines := int64(10000)
	for i := range pods.Items {
		pod := &pods.Items[i]
		raw, err := client.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container: "gateway",
			TailLines: &tailLines,
		}).DoRaw(ctx)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("read audit logs from gateway Pod %q: %v", pod.Name, err))
			continue
		}
		events = append(events, parseGatewayAuditLog(strings.NewReader(string(raw)), pod.Name, devPodName, owner)...)
	}
	return SortEvents(events), warnings
}

func parseGatewayAuditLog(reader io.Reader, gatewayPod, devPodName, owner string) []Event {
	shortName := strings.TrimPrefix(devPodName, owner+"-")
	matchingSessions := make(map[string]auditSession)
	events := make([]Event, 0)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var record gatewayAuditRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil || record.Time.IsZero() {
			continue
		}
		name := fmt.Sprintf("audit-%s-%d", gatewayPod, lineNumber)
		switch record.Message {
		case "session_open":
			if !auditTargetMatches(record.User, record.DevPod, owner, devPodName, shortName) {
				continue
			}
			matchingSessions[record.SessionID] = auditSession{user: record.User}
			events = append(events, Event{
				Name: name, Type: corev1.EventTypeNormal, Reason: "SessionOpen",
				Message:    fmt.Sprintf("SSH session opened from %s by %s (%s)", record.ClientIP, record.User, record.AuthPath),
				ObjectKind: "DevPod", ObjectName: devPodName, Source: "devpod-gateway/audit", Count: 1, Timestamp: record.Time,
			})
		case "session_close":
			session, ok := matchingSessions[record.SessionID]
			if !ok {
				continue
			}
			events = append(events, Event{
				Name: name, Type: corev1.EventTypeNormal, Reason: "SessionClose",
				Message:    fmt.Sprintf("SSH session ended for %s: %s (%.1fs, %d bytes in, %d bytes out)", session.user, record.CloseReason, record.DurationSeconds, record.BytesIn, record.BytesOut),
				ObjectKind: "DevPod", ObjectName: devPodName, Source: "devpod-gateway/audit", Count: 1, Timestamp: record.Time,
			})
		case "auth_failure":
			if !auditTargetMatches(record.User, record.DevPod, owner, devPodName, shortName) {
				continue
			}
			message := fmt.Sprintf("SSH authentication rejected for %s: %s", record.User, record.Reason)
			if record.LastSourceError != "" {
				message += " (" + record.LastSourceError + ")"
			}
			events = append(events, Event{
				Name: name, Type: corev1.EventTypeWarning, Reason: "AuthRejected", Message: message,
				ObjectKind: "DevPod", ObjectName: devPodName, Source: "devpod-gateway/audit", Count: 1, Timestamp: record.Time,
			})
		case "dial_failed":
			sessionID := auditSessionID(record.ID)
			if _, ok := matchingSessions[sessionID]; !ok {
				continue
			}
			events = append(events, Event{
				Name: name, Type: corev1.EventTypeWarning, Reason: "DialFailed",
				Message:    fmt.Sprintf("Failed to dial DevPod backend %s: %s", record.Endpoint, record.Error),
				ObjectKind: "DevPod", ObjectName: devPodName, Source: "devpod-gateway/audit", Count: 1, Timestamp: record.Time,
			})
		}
	}
	return events
}

func auditTargetMatches(user, pod, owner, fullName, shortName string) bool {
	return user == owner && (pod == shortName || pod == fullName)
}

func auditSessionID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var number uint64
	if err := json.Unmarshal(raw, &number); err == nil {
		return "sid-" + strconv.FormatUint(number, 10)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		if strings.HasPrefix(value, "sid-") {
			return value
		}
		return "sid-" + value
	}
	return ""
}

func SortEvents(events []Event) []Event {
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})
	return events
}
