package scheduler

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/AnantSingh1510/agentd/kernel/types"
)

var (
	ErrNoWorkers    = errors.New("no workers available")
	ErrTaskNotFound = errors.New("task not found")
	ErrWorkerExists = errors.New("worker already registered")
)

type Scheduler struct {
	mu      sync.Mutex
	workers map[string]*types.Worker
	tasks   map[string]*types.Task
	queue   []*types.Task
}

func New() *Scheduler {
	return &Scheduler{
		workers: make(map[string]*types.Worker),
		tasks:   make(map[string]*types.Task),
		queue:   make([]*types.Task, 0),
	}
}

func (s *Scheduler) RegisterWorker(id string, capacity int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workers[id]; exists {
		return ErrWorkerExists
	}
	s.workers[id] = &types.Worker{ID: id, Capacity: capacity}
	return nil
}

func (s *Scheduler) DeregisterWorker(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workers, id)
}

func (s *Scheduler) Submit(payload []byte) (*types.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task := &types.Task{
		ID:        uuid.NewString(),
		Payload:   payload,
		Status:    types.TaskStatusPending,
		CreatedAt: time.Now(),
	}

	s.tasks[task.ID] = task
	s.queue = append(s.queue, task)
	s.tryAssign(task)
	return task, nil
}

func (s *Scheduler) Complete(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	task.Status = types.TaskStatusDone
	if w, ok := s.workers[task.WorkerID]; ok {
		w.Active--
	}
	s.drainQueue()
	return nil
}

func (s *Scheduler) Fail(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	task.Status = types.TaskStatusFailed
	if w, ok := s.workers[task.WorkerID]; ok {
		w.Active--
	}
	s.drainQueue()
	return nil
}

func (s *Scheduler) GetTask(id string) (*types.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil, ErrTaskNotFound
	}
	return task, nil
}

func (s *Scheduler) tryAssign(task *types.Task) {
	worker := s.pickWorker()
	if worker == nil {
		return
	}
	task.Status = types.TaskStatusRunning
	task.WorkerID = worker.ID
	worker.Active++

	for i, t := range s.queue {
		if t.ID == task.ID {
			s.queue = append(s.queue[:i], s.queue[i+1:]...)
			break
		}
	}
}

func (s *Scheduler) drainQueue() {
	for _, task := range s.queue {
		if task.Status != types.TaskStatusPending {
			continue
		}
		s.tryAssign(task)
	}
}

func (s *Scheduler) pickWorker() *types.Worker {
	var best *types.Worker
	for _, w := range s.workers {
		if w.Active >= w.Capacity {
			continue
		}
		if best == nil || w.Active < best.Active {
			best = w
		}
	}
	return best
}
