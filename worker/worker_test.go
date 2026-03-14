package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AnantSingh1510/agentd/kernel"
	"github.com/AnantSingh1510/agentd/kernel/types"
)

func setupKernel(t *testing.T) *kernel.Kernel {
	t.Helper()
	k, err := kernel.New(kernel.DefaultConfig())
	if err != nil {
		t.Fatalf("kernel init failed: %v", err)
	}
	return k
}

func TestWorkerRegistersWithScheduler(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	w, err := New(k, 2, func(ctx context.Context, payload []byte) ([]byte, error) {
		return payload, nil
	})
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	if w.ID() == "" {
		t.Fatal("worker ID should not be empty")
	}
}

func TestWorkerExecutesTask(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	var wg sync.WaitGroup
	wg.Add(1)

	w, _ := New(k, 2, func(ctx context.Context, payload []byte) ([]byte, error) {
		defer wg.Done()
		return []byte("result: " + string(payload)), nil
	})
	w.Start()
	defer w.Stop()

	received := make(chan string, 1)
	k.Subscribe(
		"task.completed."+w.ID(),
		func(event *types.Event) {
			received <- string(event.Payload)
		},
	)

	task, _ := k.SubmitTask([]byte("hello"))
	w.Dispatch(task)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
		result := <-received
		if result != "result: hello" {
			t.Fatalf("expected 'result: hello', got '%s'", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for task completion")
	}
}

func TestWorkerPublishesFailureEvent(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	var wg sync.WaitGroup
	wg.Add(1)

	w, _ := New(k, 2, func(ctx context.Context, payload []byte) ([]byte, error) {
		defer wg.Done()
		return nil, errors.New("something went wrong")
	})
	w.Start()
	defer w.Stop()

	received := make(chan string, 1)
	k.Subscribe(
		"task.failed."+w.ID(),
		func(event *types.Event) {
			received <- string(event.Payload)
		},
	)

	task, _ := k.SubmitTask([]byte("bad task"))
	w.Dispatch(task)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
		errMsg := <-received
		if errMsg != "something went wrong" {
			t.Fatalf("expected error message, got '%s'", errMsg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for failure event")
	}
}

func TestWorkerStopsCleanly(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	var wg sync.WaitGroup
	wg.Add(3)

	w, _ := New(k, 3, func(ctx context.Context, payload []byte) ([]byte, error) {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		return []byte("ok"), nil
	})
	w.Start()

	for i := 0; i < 3; i++ {
		task, _ := k.SubmitTask([]byte("work"))
		w.Dispatch(task)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
		w.Stop()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tasks to complete")
	}
}

func TestWorkerContextCancelledOnStop(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	var wg sync.WaitGroup
	wg.Add(1)

	w, _ := New(k, 1, func(ctx context.Context, payload []byte) ([]byte, error) {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Second):
			return []byte("late"), nil
		}
	})
	w.Start()

	task, _ := k.SubmitTask([]byte("slow task"))
	w.Dispatch(task)

	time.Sleep(100 * time.Millisecond)
	w.Stop()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not stop cleanly after context cancel")
	}
}