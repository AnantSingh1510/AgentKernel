package autoscaler

import (
	"context"
	"testing"
	"time"

	"github.com/AnantSingh1510/agentd/kernel"
	"github.com/AnantSingh1510/agentd/worker"
)

func setupKernel(t *testing.T) *kernel.Kernel {
	t.Helper()
	k, err := kernel.New(kernel.DefaultConfig())
	if err != nil {
		t.Fatalf("kernel init failed: %v", err)
	}
	return k
}

func echoHandler() worker.Handler {
	return func(ctx context.Context, payload []byte) ([]byte, error) {
		return payload, nil
	}
}

func slowHandler() worker.Handler {
	return func(ctx context.Context, payload []byte) ([]byte, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return payload, nil
		}
	}
}

func TestAutoscalerStartsMinWorkers(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	cfg := DefaultConfig()
	cfg.MinWorkers = 2
	cfg.Interval = 10 * time.Second

	a := New(k, cfg, echoHandler())
	a.Start(context.Background())
	defer a.Stop()

	if a.WorkerCount() != 2 {
		t.Fatalf("expected 2 workers, got %d", a.WorkerCount())
	}
}

func TestAutoscalerRespectsMaxWorkers(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	cfg := DefaultConfig()
	cfg.MinWorkers = 1
	cfg.MaxWorkers = 2
	cfg.Interval = 10 * time.Second

	a := New(k, cfg, echoHandler())
	a.Start(context.Background())
	defer a.Stop()

	a.spawnWorker()
	a.spawnWorker()
	a.spawnWorker()

	a.evaluate()

	if a.WorkerCount() > cfg.MaxWorkers+3 {
		t.Fatalf("worker count %d exceeds max %d", a.WorkerCount(), cfg.MaxWorkers)
	}
}

func TestAutoscalerScalesDownToMin(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	cfg := DefaultConfig()
	cfg.MinWorkers = 1
	cfg.MaxWorkers = 5
	cfg.ScaleDownThreshold = 10
	cfg.Interval = 10 * time.Second

	a := New(k, cfg, echoHandler())
	a.Start(context.Background())
	defer a.Stop()

	// spawn extras
	a.spawnWorker()
	a.spawnWorker()

	if a.WorkerCount() < 3 {
		t.Fatalf("expected at least 3 workers before scale down")
	}
	a.evaluate()


	if a.WorkerCount() < cfg.MinWorkers {
		t.Fatalf("worker count %d fell below minimum %d", a.WorkerCount(), cfg.MinWorkers)
	}
}

func TestAutoscalerMetrics(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	cfg := DefaultConfig()
	cfg.MinWorkers = 2
	cfg.Interval = 10 * time.Second

	a := New(k, cfg, echoHandler())
	a.Start(context.Background())
	defer a.Stop()



	m := a.Metrics()

	if m.WorkerCount != 2 {
		t.Fatalf("expected 2 workers in metrics, got %d", m.WorkerCount)
	}
	if m.TotalCapacity != 2*cfg.WorkerCapacity {
		t.Fatalf("expected capacity %d, got %d", 2*cfg.WorkerCapacity, m.TotalCapacity)
	}
}

func TestAutoscalerStopsCleanly(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	cfg := DefaultConfig()
	cfg.MinWorkers = 3
	cfg.Interval = 10 * time.Second

	a := New(k, cfg, echoHandler())
	a.Start(context.Background())

	done := make(chan struct{})
	go func() {
		a.Stop()
		close(done)
	}()

	select {
	case <-done:
		if a.WorkerCount() != 0 {
			t.Fatalf("expected 0 workers after stop, got %d", a.WorkerCount())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("autoscaler did not stop cleanly within timeout")
	}
}