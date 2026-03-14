package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	ollamaadapter "github.com/AnantSingh1510/agentd/adapters/ollama"
	"github.com/AnantSingh1510/agentd/controlplane/api"
	"github.com/AnantSingh1510/agentd/controlplane/autoscaler"
	"github.com/AnantSingh1510/agentd/kernel"
	"github.com/AnantSingh1510/agentd/negotiation"
	"github.com/AnantSingh1510/agentd/worker"
)

var listenAddr string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the agentd server",
	RunE: func(cmd *cobra.Command, args []string) error {
		k, err := kernel.New(kernel.DefaultConfig())
		if err != nil {
			return fmt.Errorf("kernel init failed: %w", err)
		}
		defer k.Shutdown()

		llm := ollamaadapter.New("llama3.2")
		as := autoscaler.New(k, autoscaler.DefaultConfig(), worker.Handler(llm.WorkerHandler()))
		as.Start(context.Background())
		defer as.Stop()

		// alpha  — llama3.2
		// beta   — mistral
		// arb    — llama3.2 (arbitrator sees both, model choice matters less here)
		alpha := ollamaadapter.New("llama3.2")
		beta  := ollamaadapter.New("mistral")
		arb   := ollamaadapter.New("llama3.2")

		participants := []*negotiation.Participant{
			{
				AgentID:   "agent-alpha",
				ModelName: "llama3.2",
				Role:      negotiation.RoleProposer,
				Handler:   alpha.Handler(),
			},
			{
				AgentID:   "agent-beta",
				ModelName: "mistral",
				Role:      negotiation.RoleChallenger,
				Handler:   beta.Handler(),
			},
			{
				AgentID:   "agent-arb",
				ModelName: "llama3.2-arb",
				Role:      negotiation.RoleArbitrator,
				Handler:   arb.Handler(),
			},
		}

		neg, err := negotiation.New(k, participants)
		if err != nil {
			return fmt.Errorf("negotiator init failed: %w", err)
		}

		srv := api.New(k, neg)

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-quit
			log.Println("shutting down...")
			srv.Stop()
		}()

		log.Printf("agentd listening on %s", listenAddr)
		return srv.Listen(listenAddr)
	},
}

func init() {
	serveCmd.Flags().StringVar(&listenAddr, "listen", ":50051", "address to listen on")
}
