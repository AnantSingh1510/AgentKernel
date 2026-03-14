package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

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

		// default echo handler — swap for real LLM adapter later
		handler := func(ctx context.Context, payload []byte) ([]byte, error) {
			log.Printf("[worker] executing: %s", string(payload))
			return []byte("echo: " + string(payload)), nil
		}

		// boot autoscaler
		ascfg := autoscaler.DefaultConfig()
		as := autoscaler.New(k, ascfg, worker.Handler(handler))
		as.Start(context.Background())
		defer as.Stop()

		// stub negotiator with echo participants
		participants := []*negotiation.Participant{
			{
				AgentID:   "agent-alpha",
				ModelName: "echo-a",
				Role:      negotiation.RoleProposer,
				Handler: func(ctx context.Context, prompt []byte) ([]byte, string, error) {
					return prompt, "echo participant alpha", nil
				},
			},
			{
				AgentID:   "agent-beta",
				ModelName: "echo-b",
				Role:      negotiation.RoleChallenger,
				Handler: func(ctx context.Context, prompt []byte) ([]byte, string, error) {
					return prompt, "echo participant beta", nil
				},
			},
			{
				AgentID:   "agent-arb",
				ModelName: "echo-arb",
				Role:      negotiation.RoleArbitrator,
				Handler: func(ctx context.Context, prompt []byte) ([]byte, string, error) {
					return prompt, "arbitrated", nil
				},
			},
		}

		neg, err := negotiation.New(k, participants)
		if err != nil {
			return fmt.Errorf("negotiator init failed: %w", err)
		}

		srv := api.New(k, neg)

		// graceful shutdown on SIGINT/SIGTERM
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-quit
			log.Println("shutting down...")
			srv.Stop()
		}()

		log.Printf("agentd listnin on %s", listenAddr)
		return srv.Listen(listenAddr)
	},
}

func init() {
	serveCmd.Flags().StringVar(&listenAddr, "listen", ":50051", "address to listen on")
}