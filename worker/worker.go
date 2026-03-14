package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/AnantSingh1510/agentd/kernel"
	"github.com/AnantSingh1510/agentd/kernel/types"
)

// Handler is the function signature any executor must implement.
// It receives a task payload and returns a result or an error.
type Handler func(ctx context.Context, payload []byte) ([]byte, error)

// Worker polls the kernel for tasks, executes them via a Handler,
// and reports results back through the broker.
type Worker struct {
	id      string
	k       *kernel.Kernel
	handler Handler
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// New creates a Worker and registers it with the kernel scheduler.
// capacity controls how many tasks this worker runs concurrently.
func New(k *kernel.Kernel, capacity int, handler Handler) (*Worker, error) {
	id := "worker-" + uuid.NewString()[:8]

	if err := k.Scheduler.RegisterWorker(id, capacity); err != nil {
		return nil, fmt.Errorf("failed to register worker: %w", err)
	}

	return &Worker{
		id:      id,
		k:       k,
		handler: handler,
	}, nil
}

// Start begins processing tasks. It subscribes to task assignment
// events and spawns goroutines to handle each one.
func (w *Worker) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel

	subject := fmt.Sprintf("task.assigned.%s", w.id)

	w.k.Subscribe(subject, func(event *types.Event) {
		w.wg.Add(1)
		go w.execute(ctx, event)
	})

	log.Printf("[worker %s] started, listening on %s", w.id, subject)
}

// Stop signals the worker to stop and waits for in-flight tasks to finish.
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	w.k.Scheduler.DeregisterWorker(w.id)
	log.Printf("[worker %s] stopped", w.id)
}

// ID returns the worker's unique identifier.
func (w *Worker) ID() string {
	return w.id
}

// Dispatch is called by the control plane to send a task to this worker.
// It publishes a task.assigned event which Start() is listening for.
func (w *Worker) Dispatch(task *types.Task) error {
	subject := fmt.Sprintf("task.assigned.%s", w.id)
	return w.k.Publish(subject, task.Payload, "kernel")
}

// execute runs the handler for a single task and publishes the result.
func (w *Worker) execute(ctx context.Context, event *types.Event) {
	defer w.wg.Done()

	start := time.Now()
	log.Printf("[worker %s] executing task from event %s", w.id, event.ID)

	result, err := w.handler(ctx, event.Payload)

	elapsed := time.Since(start)

	if err != nil {
		log.Printf("[worker %s] task failed after %s: %v", w.id, elapsed, err)
		w.k.Publish(
			fmt.Sprintf("task.failed.%s", w.id),
			[]byte(err.Error()),
			w.id,
		)
		return
	}

	log.Printf("[worker %s] task completed in %s", w.id, elapsed)
	w.k.Publish(
		fmt.Sprintf("task.completed.%s", w.id),
		result,
		w.id,
	)
}