package memory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrEmptyKey   = errors.New("key cannot be empty")
	ErrEmptyAgent = errors.New("agentID cannot be empty")
	ErrNotFound   = errors.New("key not found")
)

// Store provides namespaced key-value storage per agent
type Store struct {
	client *redis.Client
}

// New connects to Redis at the given address (e.g. "localhost:6379")
func New(addr string) (*Store, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &Store{client: client}, nil
}

// Set stores a value under a key scoped to the given agentID
func (s *Store) Set(ctx context.Context, agentID, key string, value []byte, ttl time.Duration) error {
	if agentID == "" {
		return ErrEmptyAgent
	}
	if key == "" {
		return ErrEmptyKey
	}
	return s.client.Set(ctx, s.ns(agentID, key), value, ttl).Err()
}

// Get retrieves a value scoped to the given agentID
func (s *Store) Get(ctx context.Context, agentID, key string) ([]byte, error) {
	if agentID == "" {
		return nil, ErrEmptyAgent
	}
	if key == "" {
		return nil, ErrEmptyKey
	}

	val, err := s.client.Get(ctx, s.ns(agentID, key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return val, nil
}

// Delete removes a key scoped to the given agentID
func (s *Store) Delete(ctx context.Context, agentID, key string) error {
	if agentID == "" {
		return ErrEmptyAgent
	}
	if key == "" {
		return ErrEmptyKey
	}
	return s.client.Del(ctx, s.ns(agentID, key)).Err()
}

// Keys returns all keys belonging to an agent
func (s *Store) Keys(ctx context.Context, agentID string) ([]string, error) {
	if agentID == "" {
		return nil, ErrEmptyAgent
	}

	pattern := fmt.Sprintf("agent:%s:*", agentID)
	keys, err := s.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	// strip the namespace prefix before returning
	stripped := make([]string, len(keys))
	for i, k := range keys {
		prefix := fmt.Sprintf("agent:%s:", agentID)
		stripped[i] = k[len(prefix):]
	}
	return stripped, nil
}

// Flush deletes all keys belonging to an agent
func (s *Store) Flush(ctx context.Context, agentID string) error {
	if agentID == "" {
		return ErrEmptyAgent
	}

	keys, err := s.client.Keys(ctx, fmt.Sprintf("agent:%s:*", agentID)).Result()
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return s.client.Del(ctx, keys...).Err()
}

// Close closes the Redis connection
func (s *Store) Close() error {
	return s.client.Close()
}

// ns builds the namespaced key: agent:<agentID>:<key>
func (s *Store) ns(agentID, key string) string {
	return fmt.Sprintf("agent:%s:%s", agentID, key)
}