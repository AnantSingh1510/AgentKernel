package negotiation

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/AnantSingh1510/agentd/kernel"
)

// Role identifies a participant's function in the negotiation round
type Role string

const (
	RoleProposer   Role = "proposer"
	RoleChallenger Role = "challenger"
	RoleArbitrator Role = "arbitrator"
)

// Proposal is a single model's answer to the task
type Proposal struct {
	AgentID   string
	ModelName string
	Payload   []byte
	Reasoning string
	CreatedAt time.Time
}

// Challenge is a critique of a proposal by another agent
type Challenge struct {
	AgentID    string
	ModelName  string
	TargetID   string // AgentID of the proposal being challenged
	Critique   string
	CreatedAt  time.Time
}

// Verdict is the arbitrator's final resolution
type Verdict struct {
	WinnerAgentID string
	FinalAnswer   []byte
	Reasoning     string
	Dissents      []Dissent
	CreatedAt     time.Time
}

// Dissent records a meaningful disagreement between models
type Dissent struct {
	AgentID   string
	ModelName string
	Position  string // brief summary of what this agent argued
}

// Round represents a single negotiation session over one task
type Round struct {
	ID        string
	TaskID    string
	Prompt    []byte
	Proposals []Proposal
	Challenges []Challenge
	Verdict   *Verdict
	CreatedAt time.Time
}

// ModelHandler is the function signature for any LLM backend.
// It receives a prompt and returns an answer and the model's reasoning.
type ModelHandler func(ctx context.Context, prompt []byte) (answer []byte, reasoning string, err error)

// Participant is a model-backed agent that can propose and challenge
type Participant struct {
	AgentID   string
	ModelName string
	Role      Role
	Handler   ModelHandler
}

var (
	ErrNoParticipants  = errors.New("at least two participants required")
	ErrNoArbitrator    = errors.New("an arbitrator participant is required")
	ErrRoundNotFound   = errors.New("round not found")
)

// Negotiator orchestrates multi-model negotiation rounds
type Negotiator struct {
	k            *kernel.Kernel
	participants []*Participant
	rounds       map[string]*Round
	mu           sync.Mutex
}

// New creates a Negotiator. Participants must include at least two
// proposers/challengers and exactly one arbitrator.
func New(k *kernel.Kernel, participants []*Participant) (*Negotiator, error) {
	if len(participants) < 2 {
		return nil, ErrNoParticipants
	}

	hasArbitrator := false
	for _, p := range participants {
		if p.Role == RoleArbitrator {
			hasArbitrator = true
			break
		}
	}
	if !hasArbitrator {
		return nil, ErrNoArbitrator
	}

	return &Negotiator{
		k:            k,
		participants: participants,
		rounds:       make(map[string]*Round),
	}, nil
}

// Run executes a full negotiation round for the given prompt.
// It blocks until a verdict is reached or the context is cancelled.
func (n *Negotiator) Run(ctx context.Context, taskID string, prompt []byte) (*Round, error) {
	round := &Round{
		ID:        uuid.NewString(),
		TaskID:    taskID,
		Prompt:    prompt,
		CreatedAt: time.Now(),
	}

	n.mu.Lock()
	n.rounds[round.ID] = round
	n.mu.Unlock()

	log.Printf("[negotiation %s] starting round for task %s", round.ID[:8], taskID)

	// Phase 1 — collect proposals from all non-arbitrator participants
	proposals, err := n.collectProposals(ctx, round)
	if err != nil {
		return round, fmt.Errorf("proposal phase failed: %w", err)
	}
	round.Proposals = proposals

	n.publishEvent("negotiation.proposals", round.ID, fmt.Sprintf(
		"%d proposals collected", len(proposals),
	))

	// Phase 2 — collect challenges
	challenges, err := n.collectChallenges(ctx, round)
	if err != nil {
		return round, fmt.Errorf("challenge phase failed: %w", err)
	}
	round.Challenges = challenges

	n.publishEvent("negotiation.challenges", round.ID, fmt.Sprintf(
		"%d challenges collected", len(challenges),
	))

	// Phase 3 — arbitrate
	verdict, err := n.arbitrate(ctx, round)
	if err != nil {
		return round, fmt.Errorf("arbitration failed: %w", err)
	}
	round.Verdict = verdict

	n.publishEvent("negotiation.verdict", round.ID, string(verdict.FinalAnswer))

	log.Printf("[negotiation %s] verdict reached, %d dissents recorded",
		round.ID[:8], len(verdict.Dissents))

	return round, nil
}

// GetRound returns a past round by ID
func (n *Negotiator) GetRound(id string) (*Round, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	r, ok := n.rounds[id]
	if !ok {
		return nil, ErrRoundNotFound
	}
	return r, nil
}

// collectProposals runs all proposer/challenger participants in parallel
func (n *Negotiator) collectProposals(ctx context.Context, round *Round) ([]Proposal, error) {
	var (
		mu        sync.Mutex
		proposals []Proposal
		wg        sync.WaitGroup
		firstErr  error
	)

	for _, p := range n.participants {
		if p.Role == RoleArbitrator {
			continue
		}

		wg.Add(1)
		go func(p *Participant) {
			defer wg.Done()

			answer, reasoning, err := p.Handler(ctx, round.Prompt)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("participant %s failed: %w", p.AgentID, err)
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			proposals = append(proposals, Proposal{
				AgentID:   p.AgentID,
				ModelName: p.ModelName,
				Payload:   answer,
				Reasoning: reasoning,
				CreatedAt: time.Now(),
			})
			mu.Unlock()
		}(p)
	}

	wg.Wait()
	return proposals, firstErr
}

// collectChallenges asks each non-arbitrator participant to critique
// every proposal that isn't their own
func (n *Negotiator) collectChallenges(ctx context.Context, round *Round) ([]Challenge, error) {
	var (
		mu         sync.Mutex
		challenges []Challenge
		wg         sync.WaitGroup
	)

	for _, p := range n.participants {
		if p.Role == RoleArbitrator {
			continue
		}

		for _, proposal := range round.Proposals {
			if proposal.AgentID == p.AgentID {
				continue // don't challenge your own proposal
			}

			wg.Add(1)
			go func(p *Participant, target Proposal) {
				defer wg.Done()

				challengePrompt := fmt.Sprintf(
					"Original question: %s\n\nAnother model answered: %s\nIts reasoning: %s\n\nDo you agree or disagree? State your critique concisely.",
					round.Prompt, target.Payload, target.Reasoning,
				)

				critique, _, err := p.Handler(ctx, []byte(challengePrompt))
				if err != nil {
					return
				}

				mu.Lock()
				challenges = append(challenges, Challenge{
					AgentID:   p.AgentID,
					ModelName: p.ModelName,
					TargetID:  target.AgentID,
					Critique:  string(critique),
					CreatedAt: time.Now(),
				})
				mu.Unlock()
			}(p, proposal)
		}
	}

	wg.Wait()
	return challenges, nil
}

// arbitrate asks the arbitrator participant to resolve the round
func (n *Negotiator) arbitrate(ctx context.Context, round *Round) (*Verdict, error) {
	var arbitrator *Participant
	for _, p := range n.participants {
		if p.Role == RoleArbitrator {
			arbitrator = p
			break
		}
	}

	// Build arbitration prompt
	summary := fmt.Sprintf("Question: %s\n\n", round.Prompt)
	for _, prop := range round.Proposals {
		summary += fmt.Sprintf("Model %s answered: %s\nReasoning: %s\n\n",
			prop.ModelName, prop.Payload, prop.Reasoning)
	}
	for _, ch := range round.Challenges {
		summary += fmt.Sprintf("Model %s challenged %s: %s\n\n",
			ch.ModelName, ch.TargetID, ch.Critique)
	}
	summary += "Based on the above, provide the best final answer."

	finalAnswer, reasoning, err := arbitrator.Handler(ctx, []byte(summary))
	if err != nil {
		return nil, fmt.Errorf("arbitrator failed: %w", err)
	}

	// Record dissents — proposals that differ from the winner
	var dissents []Dissent
	winnerID := arbitrator.AgentID
	for _, prop := range round.Proposals {
		if string(prop.Payload) != string(finalAnswer) {
			dissents = append(dissents, Dissent{
				AgentID:   prop.AgentID,
				ModelName: prop.ModelName,
				Position:  string(prop.Payload),
			})
		}
	}

	return &Verdict{
		WinnerAgentID: winnerID,
		FinalAnswer:   finalAnswer,
		Reasoning:     reasoning,
		Dissents:      dissents,
		CreatedAt:     time.Now(),
	}, nil
}

func (n *Negotiator) publishEvent(subject, roundID, detail string) {
	payload := []byte(fmt.Sprintf("round=%s %s", roundID[:8], detail))
	n.k.Publish(subject, payload, "negotiator")
}