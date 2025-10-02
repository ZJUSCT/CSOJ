package pubsub

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// Broker a simple in-memory pub/sub system.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[string][]chan []byte // topic -> list of subscriber channels
}

var (
	once   sync.Once
	broker *Broker
)

// GetBroker returns the singleton instance of the Broker.
func GetBroker() *Broker {
	once.Do(func() {
		broker = &Broker{
			subscribers: make(map[string][]chan []byte),
		}
	})
	return broker
}

// Subscribe subscribes to a topic and returns a channel to receive messages.
// It also returns an unsubscribe function to be called when the client disconnects.
func (b *Broker) Subscribe(topic string) (<-chan []byte, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan []byte, 128)
	b.subscribers[topic] = append(b.subscribers[topic], ch)

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		subscribers := b.subscribers[topic]
		for i, sub := range subscribers {
			if sub == ch {
				// Remove the channel from the slice
				b.subscribers[topic] = append(subscribers[:i], subscribers[i+1:]...)
				close(ch)
				break
			}
		}
		zap.S().Debugf("unsubscribed from topic %s", topic)
	}

	zap.S().Debugf("new subscription to topic %s", topic)
	return ch, unsubscribe
}

// Publish publishes a message to all subscribers of a topic.
func (b *Broker) Publish(topic string, msg []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Non-blocking send to all subscribers
	for _, ch := range b.subscribers[topic] {
		select {
		case ch <- msg:
		default:
			// If the channel is full, drop the message for this subscriber.
			// This prevents a slow client from blocking the publisher.
		}
	}
}

// CloseTopic closes all subscriber channels for a given topic.
// This should be called when a submission is finished.
func (b *Broker) CloseTopic(topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subscribers, ok := b.subscribers[topic]; ok {
		for _, ch := range subscribers {
			close(ch)
		}
		delete(b.subscribers, topic)
		zap.S().Infof("closed pubsub topic %s", topic)
	}
}

// Helper to format stream messages
func FormatMessage(streamType string, data string) []byte {
	// Simple JSON format for the client to parse
	return []byte(fmt.Sprintf(`{"stream": "%s", "data": "%s"}`, streamType, data))
}
