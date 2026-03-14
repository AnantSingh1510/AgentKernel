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
	"github.com/AnantSingh1510/agentd/kernel/types"
)

type Role string

const (
	RoleProposer   Role = "proposer"
	RoleChallenger Role = "challenger"
	RoleArbitrator Role = "arbitrator"
)

type Proposal struct {
	AgentID   string
	ModelName string
	Payload   []byte
	Reasoning string
	CreatedAt time.Time
}

type Challenge struct {
	AgentID   string
	ModelName string
	TargetID  string
	Critique  string
	CreatedAt time.Time
}

type Verdict struct {
	WinnerAgentID string
	FinalAnswer   []byte
	Reasoning     string
	Dissents      []Dissent
	CreatedAt     time.Time
}

type Dissent struct {
	AgentID   string
	ModelName string
	Position  string
}

type Round struct {
	ID         string
	TaskID     string
	Prompt     []byte
	Proposals  []Proposal
	Challenges []Challenge
	Verdict    *Verdict
	CreatedAt  time.Time
}

type ModelHandler func(ctx context.Context, prompt []byte) ([]byte, string, error)

type Participant struct {
	AgentID   string
	ModelName string
	Role      Role
	Handler   ModelHandler
}

var (
	ErrNoParticipants = errors.New("at least two participants required")
	ErrNoArbitrator   = errors.New("an arbitrator participant is required")
	ErrRoundNotFound  = errors.New("round not found")
)

type Negotiator struct {
	k            *kernel.Kernel
	participants []*Participant
	rounds       map[string]*Round
	mu           sync.Mutex
}

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

	proposals, err := n.collectProposals(ctx, round)
	if err != nil {
		return round, fmt.Errorf("proposal phase failed: %w", err)
	}
	round.Proposals = proposals
	n.publishEvent("negotiation.proposals", round.ID, fmt.Sprintf("%d proposals collected", len(proposals)))

	challenges, err := n.collectChallenges(ctx, round)
	if err != nil {
		return round, fmt.Errorf("challenge phase failed: %w", err)
	}
	round.Challenges = challenges
	n.publishEvent("negotiation.challenges", round.ID, fmt.Sprintf("%d challenges collected", len(challenges)))

	verdict, err := n.arbitrate(ctx, round)
	if err != nil {
		return round, fmt.Errorf("arbitration failed: %w", err)
	}
	round.Verdict = verdict
	n.publishEvent("negotiation.verdict", round.ID, string(verdict.FinalAnswer))

	log.Printf("[negotiation %s] verdict reached, %d dissents recorded", round.ID[:8], len(verdict.Dissents))
	return round, nil
}

func (n *Negotiator) GetRound(id string) (*Round, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	r, ok := n.rounds[id]
	if !ok {
		return nil, ErrRoundNotFound
	}
	return r, nil
}

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

		// register a task with the scheduler so active count reflects real work
		task, _ := n.k.SubmitTask([]byte(fmt.Sprintf("proposal:%s:%s", round.ID[:8], p.AgentID)))

		wg.Add(1)
		go func(p *Participant, task *types.Task) {
			defer wg.Done()
			defer func() {
				if task != nil {
					n.k.Scheduler.Complete(task.ID)
				}
			}()

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
		}(p, task)
	}

	wg.Wait()
	return proposals, firstErr
}

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
				continue
			}

			task, _ := n.k.SubmitTask([]byte(fmt.Sprintf("challenge:%s:%s", round.ID[:8], p.AgentID)))

			wg.Add(1)
			go func(p *Participant, target Proposal, task *types.Task) {
				defer wg.Done()
				defer func() {
					if task != nil {
						n.k.Scheduler.Complete(task.ID)
					}
				}()

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
			}(p, proposal, task)
		}
	}

	wg.Wait()
	return challenges, nil
}

func (n *Negotiator) arbitrate(ctx context.Context, round *Round) (*Verdict, error) {
	var arbitrator *Participant
	for _, p := range n.participants {
		if p.Role == RoleArbitrator {
			arbitrator = p
			break
		}
	}

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

	// arbitration also counts as active work
	task, _ := n.k.SubmitTask([]byte(fmt.Sprintf("arbitrate:%s", round.ID[:8])))
	defer func() {
		if task != nil {
			n.k.Scheduler.Complete(task.ID)
		}
	}()

	finalAnswer, reasoning, err := arbitrator.Handler(ctx, []byte(summary))
	if err != nil {
		return nil, fmt.Errorf("arbitrator failed: %w", err)
	}

	var dissents []Dissent
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
		WinnerAgentID: arbitrator.AgentID,
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
