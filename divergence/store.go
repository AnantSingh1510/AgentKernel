package divergence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/AnantSingh1510/agentd/negotiation"
)

const (
	keyPrefix   = "dissent"
	indexKey    = "dissent:index"
)

type Record struct {
	RoundID   string    `json:"round_id"`
	TaskID    string    `json:"task_id"`
	Prompt    string    `json:"prompt"`
	Dissent   negotiation.Dissent `json:"dissent"`
	Verdict   string    `json:"verdict"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	client *redis.Client
}

func New(addr string) (*Store, error) {
	client := redis.NewClient(&redis.Options{
		Addr:        addr,
		DialTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &Store{client: client}, nil
}

func (s *Store) SaveRound(ctx context.Context, round *negotiation.Round) error {
	if round.Verdict == nil {
		return nil
	}

	for _, d := range round.Verdict.Dissents {
		rec := Record{
			RoundID:   round.ID,
			TaskID:    round.TaskID,
			Prompt:    string(round.Prompt),
			Dissent:   d,
			Verdict:   string(round.Verdict.FinalAnswer),
			CreatedAt: time.Now(),
		}

		data, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("marshal failed: %w", err)
		}

		key := recordKey(round.ID, d.AgentID)

		pipe := s.client.Pipeline()
		pipe.Set(ctx, key, data, 0)
		pipe.LPush(ctx, indexKey, key)
		_, err = pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("save failed for %s: %w", key, err)
		}
	}

	return nil
}

func (s *Store) All(ctx context.Context) ([]Record, error) {
	keys, err := s.client.LRange(ctx, indexKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("index read failed: %w", err)
	}

	records := make([]Record, 0, len(keys))
	for _, key := range keys {
		data, err := s.client.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}

		var rec Record
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}

	return records, nil
}

func (s *Store) ByModel(ctx context.Context, modelName string) ([]Record, error) {
	all, err := s.All(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]Record, 0)
	for _, r := range all {
		if r.Dissent.ModelName == modelName {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

func (s *Store) ByRound(ctx context.Context, roundID string) ([]Record, error) {
	all, err := s.All(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]Record, 0)
	for _, r := range all {
		if r.RoundID == roundID {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

func (s *Store) Stats(ctx context.Context) (map[string]int, error) {
	all, err := s.All(ctx)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, r := range all {
		counts[r.Dissent.ModelName]++
	}
	return counts, nil
}

func (s *Store) Flush(ctx context.Context) error {
	keys, err := s.client.LRange(ctx, indexKey, 0, -1).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		s.client.Del(ctx, keys...)
	}
	return s.client.Del(ctx, indexKey).Err()
}

// Close closes the Redis connection
func (s *Store) Close() error {
	return s.client.Close()
}

func recordKey(roundID, agentID string) string {
	return fmt.Sprintf("%s:%s:%s", keyPrefix, roundID[:8], agentID)
}