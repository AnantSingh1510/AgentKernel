package types

import "time"

type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusDone    TaskStatus = "done"
	TaskStatusFailed  TaskStatus = "failed"
)

type Task struct {
	ID        string
	Payload   []byte
	Status    TaskStatus
	CreatedAt time.Time
	WorkerID  string
}

type Worker struct {
	ID       string
	Capacity int
	Active   int
}

type Event struct {
	ID      string
	Subject string
	Payload []byte
	From    string
}