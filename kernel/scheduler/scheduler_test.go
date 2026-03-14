package scheduler

import (
	"testing"
	"github.com/AnantSingh1510/agentd/kernel/types"
)

func TestRegisterWorker(t *testing.T) {
	s := New()
	if err := s.RegisterWorker("w1", 2); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := s.RegisterWorker("w1", 2); err != ErrWorkerExists {
		t.Fatalf("expected ErrWorkerExists, got %v", err)
	}
}

func TestSubmitAssignsToWorker(t *testing.T) {
	s := New()
	s.RegisterWorker("w1", 2)

	task, err := s.Submit([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Status != types.TaskStatusRunning {
		t.Fatalf("expected running, got %s", task.Status)
	}
	if task.WorkerID != "w1" {
		t.Fatalf("expected w1, got %s", task.WorkerID)
	}
}

func TestSubmitQueuesWhenNoCapacity(t *testing.T) {
	s := New()
	s.RegisterWorker("w1", 1)

	s.Submit([]byte("task1"))
	task2, _ := s.Submit([]byte("task2"))

	if task2.Status != types.TaskStatusPending {
		t.Fatalf("expected pending, got %s", task2.Status)
	}
}

func TestCompleteFreesCapacityAndDrainsQueue(t *testing.T) {
	s := New()
	s.RegisterWorker("w1", 1)

	task1, _ := s.Submit([]byte("task1"))
	task2, _ := s.Submit([]byte("task2"))

	if task2.Status != types.TaskStatusPending {
		t.Fatalf("expected task2 pending, got %s", task2.Status)
	}
	if err := s.Complete(task1.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task2.Status != types.TaskStatusRunning {
		t.Fatalf("expected task2 running after drain, got %s", task2.Status)
	}
}

func TestFailFreesCapacity(t *testing.T) {
	s := New()
	s.RegisterWorker("w1", 1)

	task1, _ := s.Submit([]byte("task1"))
	task2, _ := s.Submit([]byte("task2"))

	s.Fail(task1.ID)

	if task2.Status != types.TaskStatusRunning {
		t.Fatalf("expected task2 running after fail, got %s", task2.Status)
	}
}

func TestPicksLeastLoadedWorker(t *testing.T) {
	s := New()
	s.RegisterWorker("w1", 2)
	s.RegisterWorker("w2", 2)

	s.Submit([]byte("t1"))
	s.Submit([]byte("t2"))

	used := map[string]int{}
	for _, task := range s.tasks {
		used[task.WorkerID]++
	}
	if used["w1"] == 0 || used["w2"] == 0 {
		t.Fatalf("expected both workers used, got %v", used)
	}
}

func TestGetTask(t *testing.T) {
	s := New()
	s.RegisterWorker("w1", 1)

	task, _ := s.Submit([]byte("payload"))
	fetched, err := s.GetTask(task.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fetched.ID != task.ID {
		t.Fatalf("expected %s, got %s", task.ID, fetched.ID)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	s := New()
	_, err := s.GetTask("nonexistent")
	if err != ErrTaskNotFound {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
}
