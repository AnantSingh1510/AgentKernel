package memory

import (
	"context"
	"testing"
	"time"
)

const testAddr = "localhost:6379"

func setup(t *testing.T) (*Store, context.Context) {
	t.Helper()
	s, err := New(testAddr)
	if err != nil {
		t.Fatalf("failed to connect to Redis: %v", err)
	}
	ctx := context.Background()
	return s, ctx
}

func TestSetAndGet(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	err := s.Set(ctx, "agent-1", "goal", []byte("solve the problem"), 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, err := s.Get(ctx, "agent-1", "goal")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(val) != "solve the problem" {
		t.Fatalf("expected 'solve the problem', got '%s'", val)
	}
}

func TestNamespaceIsolation(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	s.Set(ctx, "agent-1", "secret", []byte("agent1-data"), 0)
	s.Set(ctx, "agent-2", "secret", []byte("agent2-data"), 0)

	val1, _ := s.Get(ctx, "agent-1", "secret")
	val2, _ := s.Get(ctx, "agent-2", "secret")

	if string(val1) == string(val2) {
		t.Fatal("agents should have isolated namespaces, but got same value")
	}
	if string(val1) != "agent1-data" {
		t.Fatalf("expected agent1-data, got %s", val1)
	}
	if string(val2) != "agent2-data" {
		t.Fatalf("expected agent2-data, got %s", val2)
	}
}

func TestGetNotFound(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	_, err := s.Get(ctx, "agent-x", "nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	s.Set(ctx, "agent-1", "temp", []byte("value"), 0)
	s.Delete(ctx, "agent-1", "temp")

	_, err := s.Get(ctx, "agent-1", "temp")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestTTLExpiry(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	s.Set(ctx, "agent-ttl", "expiring", []byte("bye"), 1*time.Second)
	time.Sleep(2 * time.Second)

	_, err := s.Get(ctx, "agent-ttl", "expiring")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after TTL expiry, got %v", err)
	}
}

func TestKeys(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	s.Flush(ctx, "agent-keys")
	s.Set(ctx, "agent-keys", "k1", []byte("v1"), 0)
	s.Set(ctx, "agent-keys", "k2", []byte("v2"), 0)

	keys, err := s.Keys(ctx, "agent-keys")
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestFlush(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	s.Set(ctx, "agent-flush", "a", []byte("1"), 0)
	s.Set(ctx, "agent-flush", "b", []byte("2"), 0)

	s.Flush(ctx, "agent-flush")

	keys, _ := s.Keys(ctx, "agent-flush")
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys after flush, got %d", len(keys))
	}
}

func TestEmptyAgentIDErrors(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	if err := s.Set(ctx, "", "k", []byte("v"), 0); err != ErrEmptyAgent {
		t.Fatalf("expected ErrEmptyAgent, got %v", err)
	}
}

func TestEmptyKeyErrors(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	if err := s.Set(ctx, "agent-1", "", []byte("v"), 0); err != ErrEmptyKey {
		t.Fatalf("expected ErrEmptyKey, got %v", err)
	}
}