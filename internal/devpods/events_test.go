package devpods

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

func TestListEventsFiltersAndSortsNewestFirst(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kubernetes scheme: %v", err)
	}
	oldTime := time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Minute)
	old := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "dev-old", Namespace: Namespace},
		InvolvedObject: corev1.ObjectReference{Kind: "DevPod", Name: "alice-day2"},
		Reason:         "SessionConnected",
		Message:        "connected",
		Type:           corev1.EventTypeNormal,
		EventTime:      metav1.NewMicroTime(oldTime),
		Count:          1,
	}
	newer := &corev1.Event{
		ObjectMeta:          metav1.ObjectMeta{Name: "dev-new", Namespace: Namespace},
		InvolvedObject:      corev1.ObjectReference{Kind: "DevPod", Name: "alice-day2"},
		Reason:              "SessionDisconnected",
		Message:             "disconnected",
		Type:                corev1.EventTypeNormal,
		EventTime:           metav1.NewMicroTime(newTime),
		ReportingController: "devpod-gateway",
		ReportingInstance:   "gateway-1",
		Count:               2,
	}
	unrelated := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "other", Namespace: Namespace},
		InvolvedObject: corev1.ObjectReference{Kind: "DevPod", Name: "bob-day2"},
		Reason:         "Unrelated",
		EventTime:      metav1.NewMicroTime(newTime.Add(time.Minute)),
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		gvrEvents: "EventList",
	}, old, newer, unrelated)

	events, err := NewClient(dyn).ListEvents(context.Background(), "alice-day2")
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	if events[0].Name != "dev-new" || events[1].Name != "dev-old" {
		t.Fatalf("event order = %q, %q; want dev-new, dev-old", events[0].Name, events[1].Name)
	}
	if events[0].Source != "devpod-gateway/gateway-1" || events[0].Count != 2 {
		t.Fatalf("new event source/count = %q/%d", events[0].Source, events[0].Count)
	}
}
