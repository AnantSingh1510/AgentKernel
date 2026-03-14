package divergence

import (
	"context"
	"testing"
	"time"

	"github.com/AnantSingh1510/agentd/negotiation"
)

const testAddr = "localhost:6379"

func setup(t *testing.T) (*Store, context.Context) {
	t.Helper()
	s, err := New(testAddr)
	if err != nil {
		t.Fatalf("store init failed: %v", err)
	}
	ctx := context.Background()
	s.Flush(ctx)
	return s, ctx
}

func fakeRound(roundID, taskID, prompt, verdict string, dissents []negotiation.Dissent) *negotiation.Round {
	return &negotiation.Round{
		ID:        roundID,
		TaskID:    taskID,
		Prompt:    []byte(prompt),
		CreatedAt: time.Now(),
		Verdict: &negotiation.Verdict{
			FinalAnswer: []byte(verdict),
			Dissents:    dissents,
			CreatedAt:   time.Now(),
		},
	}
}

func TestSaveAndAll(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	round := fakeRound("round-0001-abcd-efgh", "task-1", "What is 2+2?", "4", []negotiation.Dissent{
		{AgentID: "agent-a", ModelName: "llama3.2", Position: "4"},
		{AgentID: "agent-b", ModelName: "mistral", Position: "five"},
	})

	if err := s.SaveRound(ctx, round); err != nil {
		t.Fatalf("SaveRound failed: %v", err)
	}

	records, err := s.All(ctx)
	if err != nil {
		t.Fatalf("All failed: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
}

func TestByModel(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	round := fakeRound("round-0002-abcd-efgh", "task-2", "capital of France?", "Paris", []negotiation.Dissent{
		{AgentID: "agent-a", ModelName: "llama3.2", Position: "Paris"},
		{AgentID: "agent-b", ModelName: "mistral", Position: "Lyon"},
	})

	s.SaveRound(ctx, round)

	records, err := s.ByModel(ctx, "mistral")
	if err != nil {
		t.Fatalf("ByModel failed: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 mistral record, got %d", len(records))
	}
	if records[0].Dissent.Position != "Lyon" {
		t.Fatalf("expected 'Lyon', got '%s'", records[0].Dissent.Position)
	}
}

func TestByRound(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	r1 := fakeRound("round-0003-abcd-efgh", "task-3", "prompt a", "answer a", []negotiation.Dissent{
		{AgentID: "agent-a", ModelName: "llama3.2", Position: "x"},
	})
	r2 := fakeRound("round-0004-abcd-efgh", "task-4", "prompt b", "answer b", []negotiation.Dissent{
		{AgentID: "agent-b", ModelName: "mistral", Position: "y"},
	})

	s.SaveRound(ctx, r1)
	s.SaveRound(ctx, r2)

	records, err := s.ByRound(ctx, r1.ID)
	if err != nil {
		t.Fatalf("ByRound failed: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record for round 1, got %d", len(records))
	}
	if records[0].TaskID != "task-3" {
		t.Fatalf("expected task-3, got %s", records[0].TaskID)
	}
}

func TestStats(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	round := fakeRound("round-0005-abcd-efgh", "task-5", "best language?", "Go", []negotiation.Dissent{
		{AgentID: "agent-a", ModelName: "llama3.2", Position: "Python"},
		{AgentID: "agent-b", ModelName: "mistral", Position: "Rust"},
		{AgentID: "agent-c", ModelName: "llama3.2", Position: "TypeScript"},
	})

	s.SaveRound(ctx, round)

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats["llama3.2"] != 2 {
		t.Fatalf("expected llama3.2 count 2, got %d", stats["llama3.2"])
	}
	if stats["mistral"] != 1 {
		t.Fatalf("expected mistral count 1, got %d", stats["mistral"])
	}
}

func TestNoDissentsSavesNothing(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	round := fakeRound("round-0006-abcd-efgh", "task-6", "agree?", "yes", []negotiation.Dissent{})
	s.SaveRound(ctx, round)

	records, _ := s.All(ctx)
	if len(records) != 0 {
		t.Fatalf("expected 0 records for no-dissent round, got %d", len(records))
	}
}

func TestFlush(t *testing.T) {
	s, ctx := setup(t)
	defer s.Close()

	round := fakeRound("round-0007-abcd-efgh", "task-7", "flush me", "ok", []negotiation.Dissent{
		{AgentID: "agent-a", ModelName: "llama3.2", Position: "x"},
	})
	s.SaveRound(ctx, round)
	s.Flush(ctx)

	records, _ := s.All(ctx)
	if len(records) != 0 {
		t.Fatalf("expected 0 records after flush, got %d", len(records))
	}
}