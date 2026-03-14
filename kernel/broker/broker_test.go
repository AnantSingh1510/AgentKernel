package broker

import (
	"sync"
	"testing"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/AnantSingh1510/agentd/kernel/types"
)

const testURL = nats.DefaultURL

func TestPublishAndSubscribe(t *testing.T) {
	b, err := New(testURL)
	if err != nil {
		t.Fatalf("failed to connect to NATS: %v", err)
	}
	defer b.Drain()

	var wg sync.WaitGroup
	wg.Add(1)
	received := make(chan string, 1)

	err = b.Subscribe("test.subject", func(event *types.Event) {
		received <- string(event.Payload)
		wg.Done()
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	err = b.Publish("test.subject", []byte("hello kernel"), "agent-1")
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
		payload := <-received
		if payload != "hello kernel" {
			t.Fatalf("expected 'hello kernel', got '%s'", payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestSubscribeWildcard(t *testing.T) {
	b, err := New(testURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer b.Drain()

	count := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)

	b.Subscribe("agent.*", func(event *types.Event) {
		mu.Lock()
		count++
		mu.Unlock()
		wg.Done()
	})

	b.Publish("agent.started", []byte("a"), "w1")
	b.Publish("agent.stopped", []byte("b"), "w1")

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		if count != 2 {
			t.Fatalf("expected 2 messages, got %d", count)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for wildcard messages")
	}
}

func TestPublishEmptySubjectErrors(t *testing.T) {
	b, err := New(testURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer b.Drain()

	if err := b.Publish("", []byte("x"), "a1"); err != ErrEmptySubject {
		t.Fatalf("expected ErrEmptySubject, got %v", err)
	}
}

func TestSubscribeNilHandlerErrors(t *testing.T) {
	b, err := New(testURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer b.Drain()

	if err := b.Subscribe("some.subject", nil); err != ErrNilHandler {
		t.Fatalf("expected ErrNilHandler, got %v", err)
	}
}
