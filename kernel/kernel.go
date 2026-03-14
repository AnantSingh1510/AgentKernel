package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/AnantSingh1510/agentd/kernel/broker"
	"github.com/AnantSingh1510/agentd/kernel/memory"
	"github.com/AnantSingh1510/agentd/kernel/scheduler"
	"github.com/AnantSingh1510/agentd/kernel/types"
)

type Config struct {
	NATSUrl   string
	RedisAddr string
}

func DefaultConfig() Config {
	return Config{
		NATSUrl:   "nats://127.0.0.1:4222",
		RedisAddr: "localhost:6379",
	}
}

type Kernel struct {
	Scheduler *scheduler.Scheduler
	Broker    *broker.Broker
	Memory    *memory.Store
}

func New(cfg Config) (*Kernel, error) {
	sched := scheduler.New()

	b, err := broker.New(cfg.NATSUrl)
	if err != nil {
		return nil, fmt.Errorf("broker init failed: %w", err)
	}

	mem, err := memory.New(cfg.RedisAddr)
	if err != nil {
		b.Drain()
		return nil, fmt.Errorf("memory init failed: %w", err)
	}

	return &Kernel{Scheduler: sched, Broker: b, Memory: mem}, nil
}

func (k *Kernel) Shutdown() error {
	if err := k.Broker.Drain(); err != nil {
		return fmt.Errorf("broker drain failed: %w", err)
	}
	if err := k.Memory.Close(); err != nil {
		return fmt.Errorf("memory close failed: %w", err)
	}
	return nil
}

func (k *Kernel) SubmitTask(payload []byte) (*types.Task, error) {
	return k.Scheduler.Submit(payload)
}

func (k *Kernel) Publish(subject string, payload []byte, from string) error {
	return k.Broker.Publish(subject, payload, from)
}

func (k *Kernel) Subscribe(subject string, handler broker.Handler) error {
	return k.Broker.Subscribe(subject, handler)
}

func (k *Kernel) Remember(ctx context.Context, agentID, key string, value []byte, ttl time.Duration) error {
	return k.Memory.Set(ctx, agentID, key, value, ttl)
}

func (k *Kernel) Recall(ctx context.Context, agentID, key string) ([]byte, error) {
	return k.Memory.Get(ctx, agentID, key)
}
