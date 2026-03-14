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

type Handler func(ctx context.Context, payload []byte) ([]byte, error)

type Worker struct {
	id      string
	k       *kernel.Kernel
	handler Handler
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

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

func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	w.k.Scheduler.DeregisterWorker(w.id)
	log.Printf("[worker %s] stopped", w.id)
}

func (w *Worker) ID() string {
	return w.id
}

func (w *Worker) Dispatch(task *types.Task) error {
	subject := fmt.Sprintf("task.assigned.%s", w.id)
	return w.k.Publish(subject, task.Payload, "kernel")
}

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