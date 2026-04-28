package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// EventQueue provides a persistent queue for events using Redis Streams
type EventQueue interface {
	Enqueue(ctx context.Context, event *QueuedEvent) error
	Dequeue(ctx context.Context, consumerGroup string, consumerName string, count int) ([]*QueuedEvent, error)
	Ack(ctx context.Context, consumerGroup string, id string) error
	CreateConsumerGroup(ctx context.Context, groupName string) error
}

// RedisEventQueue implements EventQueue using Redis Streams
type RedisEventQueue struct {
	client  *redis.Client
	stream  string
	group   string
	enabled bool
}

// NewRedisEventQueue creates a new Redis-based event queue
func NewRedisEventQueue(redisURL string, streamName string, groupName string) (EventQueue, error) {
	if redisURL == "" {
		// Return in-memory fallback if Redis not configured
		return NewInMemoryEventQueue(), nil
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		// Try as simple host:port
		opts = &redis.Options{
			Addr: redisURL,
		}
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	queue := &RedisEventQueue{
		client:  client,
		stream:  streamName,
		group:   groupName,
		enabled: true,
	}

	// Create consumer group if it doesn't exist
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	if err := queue.CreateConsumerGroup(ctx2, groupName); err != nil {
		// Group might already exist, which is fine
	}

	return queue, nil
}

// Enqueue adds an event to the Redis stream
func (r *RedisEventQueue) Enqueue(ctx context.Context, event *QueuedEvent) error {
	if !r.enabled {
		return fmt.Errorf("redis queue not enabled")
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	// Add to stream with event data
	args := redis.XAddArgs{
		Stream: r.stream,
		Values: map[string]interface{}{
			"source_system": event.SourceSystem,
			"event_type":    event.EventType,
			"source_user_id": event.SourceUserID,
			"event_time":    event.EventTime.Format(time.RFC3339),
			"event_data":    string(eventData),
		},
	}

	_, err = r.client.XAdd(ctx, &args).Result()
	return err
}

// Dequeue reads events from the Redis stream
func (r *RedisEventQueue) Dequeue(ctx context.Context, consumerGroup string, consumerName string, count int) ([]*QueuedEvent, error) {
	if !r.enabled {
		return nil, fmt.Errorf("redis queue not enabled")
	}

	// Read from consumer group
	streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    consumerGroup,
		Consumer: consumerName,
		Streams:  []string{r.stream, ">"},
		Count:    int64(count),
		Block:    time.Second, // Block for 1 second if no messages
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return []*QueuedEvent{}, nil // No messages
		}
		return nil, err
	}

	var events []*QueuedEvent
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			event, err := r.parseMessage(msg)
			if err != nil {
				continue // Skip invalid messages
			}
			event.StreamID = msg.ID // Store ID for ACK
			events = append(events, event)
		}
	}

	return events, nil
}

// Ack acknowledges a processed message
func (r *RedisEventQueue) Ack(ctx context.Context, consumerGroup string, id string) error {
	if !r.enabled {
		return nil
	}
	return r.client.XAck(ctx, r.stream, consumerGroup, id).Err()
}

// CreateConsumerGroup creates a consumer group for the stream
func (r *RedisEventQueue) CreateConsumerGroup(ctx context.Context, groupName string) error {
	if !r.enabled {
		return nil
	}
	// Create group starting from the beginning if it doesn't exist
	// First try to create the stream if it doesn't exist
	err := r.client.XGroupCreateMkStream(ctx, r.stream, groupName, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		// If stream exists, try without MkStream
		err = r.client.XGroupCreate(ctx, r.stream, groupName, "0").Err()
		if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
			return err
		}
	}
	return nil
}

// parseMessage converts a Redis stream message to QueuedEvent
func (r *RedisEventQueue) parseMessage(msg redis.XMessage) (*QueuedEvent, error) {
	event := &QueuedEvent{}

	// Extract fields
	sourceSystem, ok := msg.Values["source_system"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid source_system")
	}
	event.SourceSystem = sourceSystem

	eventType, ok := msg.Values["event_type"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid event_type")
	}
	event.EventType = eventType

	if sourceUserID, ok := msg.Values["source_user_id"].(string); ok {
		event.SourceUserID = sourceUserID
	}

	if eventTimeStr, ok := msg.Values["event_time"].(string); ok {
		if t, err := time.Parse(time.RFC3339, eventTimeStr); err == nil {
			event.EventTime = t
		}
	}

	// Parse event_data JSON
	if eventDataStr, ok := msg.Values["event_data"].(string); ok {
		if err := json.Unmarshal([]byte(eventDataStr), &event.EventData); err != nil {
			return nil, fmt.Errorf("unmarshal event_data: %w", err)
		}
	}

	return event, nil
}

// InMemoryEventQueue is a fallback implementation using Go channels
type InMemoryEventQueue struct {
	queue chan *QueuedEvent
}

// NewInMemoryEventQueue creates an in-memory event queue
func NewInMemoryEventQueue() EventQueue {
	return &InMemoryEventQueue{
		queue: make(chan *QueuedEvent, 1000),
	}
}

// Enqueue adds an event to the in-memory queue
func (i *InMemoryEventQueue) Enqueue(ctx context.Context, event *QueuedEvent) error {
	select {
	case i.queue <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("queue full")
	}
}

// Dequeue reads events from the in-memory queue
func (i *InMemoryEventQueue) Dequeue(ctx context.Context, consumerGroup string, consumerName string, count int) ([]*QueuedEvent, error) {
	var events []*QueuedEvent
	for len(events) < count {
		select {
		case event := <-i.queue:
			events = append(events, event)
		case <-ctx.Done():
			return events, ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Timeout after 100ms if no messages
			return events, nil
		}
	}
	return events, nil
}

// Ack is a no-op for in-memory queue
func (i *InMemoryEventQueue) Ack(ctx context.Context, consumerGroup string, id string) error {
	return nil
}

// CreateConsumerGroup is a no-op for in-memory queue
func (i *InMemoryEventQueue) CreateConsumerGroup(ctx context.Context, groupName string) error {
	return nil
}
