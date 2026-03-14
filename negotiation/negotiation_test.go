package negotiation

import (
	"context"
	"strings"
	"testing"

	"github.com/AnantSingh1510/agentd/kernel"
)

func setupKernel(t *testing.T) *kernel.Kernel {
	t.Helper()
	k, err := kernel.New(kernel.DefaultConfig())
	if err != nil {
		t.Fatalf("kernel init failed: %v", err)
	}
	return k
}

// fakeHandler returns a fixed answer and reasoning — no LLM needed
func fakeHandler(answer, reasoning string) ModelHandler {
	return func(ctx context.Context, prompt []byte) ([]byte, string, error) {
		return []byte(answer), reasoning, nil
	}
}

func participants(k *kernel.Kernel) []*Participant {
	return []*Participant{
		{
			AgentID:   "agent-gpt",
			ModelName: "gpt-4",
			Role:      RoleProposer,
			Handler:   fakeHandler("Paris", "It is the capital of France."),
		},
		{
			AgentID:   "agent-claude",
			ModelName: "claude",
			Role:      RoleChallenger,
			Handler:   fakeHandler("Paris", "Widely known as the French capital."),
		},
		{
			AgentID:   "agent-arbitrator",
			ModelName: "gpt-4",
			Role:      RoleArbitrator,
			Handler:   fakeHandler("Paris", "Both models agree."),
		},
	}
}

func TestNegotiatorCreation(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	n, err := New(k, participants(k))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if n == nil {
		t.Fatal("negotiator should not be nil")
	}
}

func TestNegotiatorRequiresMinParticipants(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	_, err := New(k, []*Participant{
		{AgentID: "a1", ModelName: "gpt-4", Role: RoleArbitrator,
			Handler: fakeHandler("x", "y")},
	})
	if err != ErrNoParticipants {
		t.Fatalf("expected ErrNoParticipants, got %v", err)
	}
}

func TestNegotiatorRequiresArbitrator(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	_, err := New(k, []*Participant{
		{AgentID: "a1", ModelName: "gpt-4", Role: RoleProposer,
			Handler: fakeHandler("x", "y")},
		{AgentID: "a2", ModelName: "claude", Role: RoleChallenger,
			Handler: fakeHandler("x", "y")},
	})
	if err != ErrNoArbitrator {
		t.Fatalf("expected ErrNoArbitrator, got %v", err)
	}
}

func TestRoundProducesVerdict(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	n, _ := New(k, participants(k))
	round, err := n.Run(context.Background(), "task-1", []byte("What is the capital of France?"))
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if round.Verdict == nil {
		t.Fatal("expected a verdict")
	}
	if string(round.Verdict.FinalAnswer) != "Paris" {
		t.Fatalf("expected 'Paris', got '%s'", round.Verdict.FinalAnswer)
	}
}

func TestRoundCollectsProposals(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	n, _ := New(k, participants(k))
	round, _ := n.Run(context.Background(), "task-2", []byte("What is 2+2?"))

	// two non-arbitrator participants
	if len(round.Proposals) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(round.Proposals))
	}
}

func TestRoundCollectsChallenges(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	n, _ := New(k, participants(k))
	round, _ := n.Run(context.Background(), "task-3", []byte("What is 2+2?"))

	// each of 2 participants challenges the other's 1 proposal = 2 challenges
	if len(round.Challenges) != 2 {
		t.Fatalf("expected 2 challenges, got %d", len(round.Challenges))
	}
}

func TestDissentRecordedWhenModelsDisagree(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	ps := []*Participant{
		{
			AgentID:   "agent-a",
			ModelName: "gpt-4",
			Role:      RoleProposer,
			Handler:   fakeHandler("42", "The answer is 42."),
		},
		{
			AgentID:   "agent-b",
			ModelName: "claude",
			Role:      RoleChallenger,
			Handler:   fakeHandler("41", "I think it is 41."),
		},
		{
			AgentID:   "agent-arb",
			ModelName: "gpt-4",
			Role:      RoleArbitrator,
			Handler:   fakeHandler("42", "42 is correct."),
		},
	}

	n, _ := New(k, ps)
	round, _ := n.Run(context.Background(), "task-4", []byte("What is 6x7?"))

	if len(round.Verdict.Dissents) == 0 {
		t.Fatal("expected at least one dissent when models disagree")
	}
}

func TestGetRound(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	n, _ := New(k, participants(k))
	round, _ := n.Run(context.Background(), "task-5", []byte("ping"))

	fetched, err := n.GetRound(round.ID)
	if err != nil {
		t.Fatalf("GetRound failed: %v", err)
	}
	if fetched.ID != round.ID {
		t.Fatalf("expected %s, got %s", round.ID, fetched.ID)
	}
}

func TestGetRoundNotFound(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	n, _ := New(k, participants(k))
	_, err := n.GetRound("nonexistent")
	if err != ErrRoundNotFound {
		t.Fatalf("expected ErrRoundNotFound, got %v", err)
	}
}

func TestChallengePromptContainsOriginalQuestion(t *testing.T) {
	k := setupKernel(t)
	defer k.Shutdown()

	var capturedPrompt string
	ps := []*Participant{
		{
			AgentID:   "agent-a",
			ModelName: "gpt-4",
			Role:      RoleProposer,
			Handler:   fakeHandler("yes", "because yes"),
		},
		{
			AgentID:   "agent-b",
			ModelName: "claude",
			Role:      RoleChallenger,
			Handler: func(ctx context.Context, prompt []byte) ([]byte, string, error) {
				capturedPrompt = string(prompt)
				return []byte("disagree"), "different reasoning", nil
			},
		},
		{
			AgentID:   "agent-arb",
			ModelName: "gpt-4",
			Role:      RoleArbitrator,
			Handler:   fakeHandler("yes", "yes is right"),
		},
	}

	n, _ := New(k, ps)
	n.Run(context.Background(), "task-6", []byte("Is the sky blue?"))

	if !strings.Contains(capturedPrompt, "Is the sky blue?") {
		t.Fatalf("challenge prompt should contain original question, got: %s", capturedPrompt)
	}
}