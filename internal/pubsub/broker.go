package pubsub

import (
	"encoding/json"
	"sync"

	"go.uber.org/zap"
)

// Broker a simple in-memory pub/sub system.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[string][]chan []byte // topic -> list of subscriber channels
	cache       map[string][][]byte      // topic -> list of cached messages
}

type WsMessage struct {
	Stream string `json:"stream"`
	Data   string `json:"data"`
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
			cache:       make(map[string][][]byte),
		}
	})
	return broker
}

// Subscribe subscribes to a topic. It first sends all cached messages to the new
// subscriber, then adds the subscriber to receive live messages.
func (b *Broker) Subscribe(topic string) (<-chan []byte, func()) {
	b.mu.Lock()

	ch := make(chan []byte, 128) // Use a buffered channel

	// Send cached history to the new subscriber.
	// We do this inside the lock to get a consistent snapshot.
	// The actual sending happens in a goroutine to avoid blocking the broker.
	history := b.cache[topic]

	go func() {
		for _, msg := range history {
			ch <- msg
		}
	}()

	b.subscribers[topic] = append(b.subscribers[topic], ch)
	b.mu.Unlock() // Unlock after modifying subscribers map

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

	zap.S().Debugf("new subscription to topic %s, sent %d cached messages", topic, len(history))
	return ch, unsubscribe
}

// Publish publishes a message to all subscribers of a topic and caches it.
func (b *Broker) Publish(topic string, msg []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Add message to cache.
	// For production, you might want to add a cache size limit per topic to prevent memory exhaustion.
	b.cache[topic] = append(b.cache[topic], msg)

	// Broadcast to live subscribers (non-blocking).
	for _, ch := range b.subscribers[topic] {
		select {
		case ch <- msg:
		default:
			// If a subscriber's channel is full, drop the message for them.
			// This prevents a slow client from blocking the publisher.
		}
	}
}

// CloseTopic closes all subscriber channels and clears the cache for a given topic.
func (b *Broker) CloseTopic(topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if subscribers, ok := b.subscribers[topic]; ok {
		for _, ch := range subscribers {
			close(ch)
		}
		delete(b.subscribers, topic)
		// Crucially, delete the cache to free up memory
		delete(b.cache, topic)
		zap.S().Infof("closed pubsub topic %s and cleared cache", topic)
	}
}

// Helper to format stream messages
func FormatMessage(streamType string, data string) []byte {
	msg := WsMessage{Stream: streamType, Data: data}
	bytes, err := json.Marshal(msg)
	if err != nil {
		return []byte(`{"stream": "error", "data": "json format error"}`)
	}
	return bytes
}
