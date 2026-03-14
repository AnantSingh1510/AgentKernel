package kernel

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/AnantSingh1510/agentd/kernel/broker"
	"github.com/AnantSingh1510/agentd/kernel/types"
)

func setupKernel(t *testing.T) *Kernel {
	t.Helper()
	k, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("failed to init kernel: %v", err)
	}
	return k
}

func TestKernelBoots(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	if k.Scheduler == nil {
		t.Fatal("scheduler is nil")
	}
	if k.Broker == nil {
		t.Fatal("broker is nil")
	}
	if k.Memory == nil {
		t.Fatal("memory is nil")
	}
}

func TestKernelScheduleAndComplete(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	k.Scheduler.RegisterWorker("w1", 2)

	task, err := k.SubmitTask([]byte("test payload"))
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}
	if task.Status != types.TaskStatusRunning {
		t.Fatalf("expected running, got %s", task.Status)
	}
	if err := k.Scheduler.Complete(task.ID); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	fetched, _ := k.Scheduler.GetTask(task.ID)
	if fetched.Status != types.TaskStatusDone {
		t.Fatalf("expected done, got %s", fetched.Status)
	}
}

func TestKernelPublishSubscribe(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	var wg sync.WaitGroup
	wg.Add(1)

	k.Subscribe("kernel.test", func(event *types.Event) {
		wg.Done()
	})

	k.Publish("kernel.test", []byte("ping"), "test")

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestKernelRememberAndRecall(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	ctx := context.Background()

	err := k.Remember(ctx, "agent-1", "task-context", []byte("some context"), 0)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	val, err := k.Recall(ctx, "agent-1", "task-context")
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if string(val) != "some context" {
		t.Fatalf("expected 'some context', got '%s'", val)
	}
}

func TestKernelShutdownClean(t *testing.T) {
	k := setupKernel(t)
	if err := k.Shutdown(); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

var _ broker.Handler = func(event *types.Event) {}
