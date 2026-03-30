package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient wraps the go-redis client for async job queue operations.
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new Redis client from URL.
// URL format: redis://[:password@]host:port/db
func NewRedisClient(url string) (*RedisClient, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis url: %w", err)
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisClient{client: client}, nil
}

// Close closes the Redis connection.
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// RPush pushes a value to the right of a list.
// This is used by the PRODUCER to add results to the queue.
//
// Pattern: After background processing completes, we RPUSH the result JSON
// to a key like "speaking:reply:{request_id}". The consumer can then BLPOP
// this key to get the result.
func (r *RedisClient) RPush(ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	return r.client.RPush(ctx, key, data).Err()
}

// SetExpiry sets TTL on a key.
// Called after RPUSH to ensure keys don't persist forever.
func (r *RedisClient) SetExpiry(ctx context.Context, key string, ttl time.Duration) error {
	return r.client.Expire(ctx, key, ttl).Err()
}

// BLPop performs a blocking left pop on the specified key.
// This is used by the CONSUMER to wait for results from the producer.
//
// Pattern: The client calls GET /speaking/reply which uses BLPOP to wait
// for up to `timeout` duration. If a result arrives (via RPUSH from the
// background goroutine), BLPOP returns immediately with the value.
// If timeout expires, returns redis.Nil error.
//
// Returns the raw JSON bytes of the popped value.
func (r *RedisClient) BLPop(ctx context.Context, timeout time.Duration, key string) ([]byte, error) {
	result, err := r.client.BLPop(ctx, timeout, key).Result()
	if err != nil {
		return nil, err
	}

	// BLPop returns [key, value] pair
	if len(result) < 2 {
		return nil, fmt.Errorf("unexpected blpop result format")
	}

	return []byte(result[1]), nil
}

// HSet sets fields in a Redis Hash.
func (r *RedisClient) HSet(ctx context.Context, key string, values ...interface{}) error {
	return r.client.HSet(ctx, key, values...).Err()
}

// HGetAll returns all fields and values of a Redis Hash.
func (r *RedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.client.HGetAll(ctx, key).Result()
}

// Ping checks Redis connectivity.
func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}
