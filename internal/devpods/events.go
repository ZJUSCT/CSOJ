package devpods

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvrEvents = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

// Event is the stable API representation of a Kubernetes Event associated
// with a DevPod or a same-named related object.
type Event struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Reason     string    `json:"reason"`
	Message    string    `json:"message"`
	ObjectKind string    `json:"object_kind"`
	ObjectName string    `json:"object_name"`
	Source     string    `json:"source"`
	Count      int32     `json:"count"`
	Timestamp  time.Time `json:"timestamp"`
}

// ListEvents returns Events whose involved object has the DevPod's name. The
// API server applies the field selector; the local check is a defensive guard
// and also keeps dynamic fake-client tests faithful.
func (c *Client) ListEvents(ctx context.Context, devPodName string) ([]Event, error) {
	list, err := c.dyn.Resource(gvrEvents).Namespace(Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", devPodName),
	})
	if err != nil {
		return nil, fmt.Errorf("list events for DevPod %q: %w", devPodName, err)
	}

	events := make([]Event, 0, len(list.Items))
	for i := range list.Items {
		var item corev1.Event
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[i].Object, &item); err != nil {
			continue
		}
		if item.InvolvedObject.Name != devPodName {
			continue
		}
		count := item.Count
		if item.Series != nil && item.Series.Count > count {
			count = item.Series.Count
		}
		if count < 1 {
			count = 1
		}
		events = append(events, Event{
			Name:       item.Name,
			Type:       item.Type,
			Reason:     item.Reason,
			Message:    item.Message,
			ObjectKind: item.InvolvedObject.Kind,
			ObjectName: item.InvolvedObject.Name,
			Source:     eventSource(item),
			Count:      count,
			Timestamp:  eventTimestamp(item),
		})
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})
	return events, nil
}

func eventTimestamp(event corev1.Event) time.Time {
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	if event.Series != nil && !event.Series.LastObservedTime.IsZero() {
		return event.Series.LastObservedTime.Time
	}
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	if !event.FirstTimestamp.IsZero() {
		return event.FirstTimestamp.Time
	}
	return event.CreationTimestamp.Time
}

func eventSource(event corev1.Event) string {
	source := event.ReportingController
	if source == "" {
		source = event.Source.Component
	}
	instance := event.ReportingInstance
	if instance == "" {
		instance = event.Source.Host
	}
	if source != "" && instance != "" && instance != source {
		return source + "/" + instance
	}
	return source
}
