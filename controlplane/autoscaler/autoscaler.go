package autoscaler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/AnantSingh1510/agentd/kernel"
	"github.com/AnantSingh1510/agentd/worker"
)

type Config struct {
	Interval           time.Duration
	WorkerCapacity     int
	ScaleUpThreshold   int
	ScaleDownThreshold int
	MinWorkers         int
	MaxWorkers         int
}

func DefaultConfig() Config {
	return Config{
		Interval:           2 * time.Second,
		WorkerCapacity:     4,
		ScaleUpThreshold:   1,
		ScaleDownThreshold: 0,
		MinWorkers:         1,
		MaxWorkers:         10,
	}
}

type Autoscaler struct {
	k       *kernel.Kernel
	cfg     Config
	handler worker.Handler
	workers []*worker.Worker
	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func New(k *kernel.Kernel, cfg Config, handler worker.Handler) *Autoscaler {
	return &Autoscaler{
		k:       k,
		cfg:     cfg,
		handler: handler,
		workers: make([]*worker.Worker, 0),
	}
}

func (a *Autoscaler) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	for i := 0; i < a.cfg.MinWorkers; i++ {
		a.spawnWorker()
	}

	a.wg.Add(1)
	go a.loop(ctx)
}

func (a *Autoscaler) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
	a.wg.Wait()

	a.mu.Lock()
	defer a.mu.Unlock()
	for _, w := range a.workers {
		w.Stop()
	}
	a.workers = nil
}

func (a *Autoscaler) WorkerCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.workers)
}

func (a *Autoscaler) loop(ctx context.Context) {
	defer a.wg.Done()
	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.evaluate()
		}
	}
}

func (a *Autoscaler) evaluate() {
	workers := a.k.Scheduler.ListWorkers()
	queued := a.k.Scheduler.QueueDepth()

	totalCapacity := 0
	totalActive := 0
	for _, w := range workers {
		totalCapacity += w.Capacity
		totalActive += w.Active
	}

	log.Printf("[autoscaler] workers=%d active=%d/%d queued=%d",
		len(workers), totalActive, totalCapacity, queued)

	a.mu.Lock()
	currentCount := len(a.workers)
	a.mu.Unlock()

	if queued >= a.cfg.ScaleUpThreshold && currentCount < a.cfg.MaxWorkers {
		log.Printf("[autoscaler] scaling up (queued=%d >= threshold=%d)",
			queued, a.cfg.ScaleUpThreshold)
		a.spawnWorker()
		return
	}

	if totalActive <= a.cfg.ScaleDownThreshold && currentCount > a.cfg.MinWorkers {
		log.Printf("[autoscaler] scaling down (active=%d <= threshold=%d)",
			totalActive, a.cfg.ScaleDownThreshold)
		a.removeWorker()
	}
}

func (a *Autoscaler) spawnWorker() {
	w, err := worker.New(a.k, a.cfg.WorkerCapacity, a.handler)
	if err != nil {
		log.Printf("[autoscaler] failed to spawn worker: %v", err)
		return
	}
	w.Start()

	a.mu.Lock()
	a.workers = append(a.workers, w)
	a.mu.Unlock()

	log.Printf("[autoscaler] spawned worker %s (total=%d)", w.ID(), a.WorkerCount())
}

func (a *Autoscaler) removeWorker() {
	a.mu.Lock()
	if len(a.workers) == 0 {
		a.mu.Unlock()
		return
	}
	last := a.workers[len(a.workers)-1]
	a.workers = a.workers[:len(a.workers)-1]
	a.mu.Unlock()

	last.Stop()
	log.Printf("[autoscaler] removed worker %s (total=%d)", last.ID(), a.WorkerCount())
}

type Metrics struct {
	WorkerCount   int
	TotalCapacity int
	TotalActive   int
	QueueDepth    int
}

func (a *Autoscaler) Metrics() Metrics {
	workers := a.k.Scheduler.ListWorkers()
	totalCapacity := 0
	totalActive := 0
	for _, w := range workers {
		totalCapacity += w.Capacity
		totalActive += w.Active
	}
	return Metrics{
		WorkerCount:   a.WorkerCount(),
		TotalCapacity: totalCapacity,
		TotalActive:   totalActive,
		QueueDepth:    a.k.Scheduler.QueueDepth(),
	}
}

func (m Metrics) String() string {
	return fmt.Sprintf("workers=%d capacity=%d active=%d queued=%d",
		m.WorkerCount, m.TotalCapacity, m.TotalActive, m.QueueDepth)
}
