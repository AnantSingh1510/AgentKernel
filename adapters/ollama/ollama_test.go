package ollama

import (
	"context"
	"testing"
)

// These tests require a running Ollama instance with llama3.2 pulled.

func TestGenerate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ollama integration test in short mode")
	}

	c := New("llama3.2")
	resp, err := c.Generate(context.Background(), "Reply with only the word PONG.")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp == "" {
		t.Fatal("expected non-empty response")
	}
	t.Logf("response: %s", resp)
}

func TestHandlerInterface(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ollama integration test in short mode")
	}

	c := New("llama3.2")
	handler := c.Handler()

	answer, reasoning, err := handler(context.Background(), []byte("Reply with only the word PONG."))
	if err != nil {
		t.Fatalf("handler failed: %v", err)
	}
	if len(answer) == 0 {
		t.Fatal("expected non-empty answer")
	}
	if reasoning == "" {
		t.Fatal("expected non-empty reasoning label")
	}
	t.Logf("answer: %s | reasoning: %s", answer, reasoning)
}