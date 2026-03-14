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

func debaHandler(client *ollamaadapter.Client, stance string) negotiation.ModelHandler {
	return func(ctx context.Context, prompt []byte) ([]byte, string, error) {
		injected := fmt.Sprintf(
			"You are a debater. You must argue %s the following position. "+
				"Be direct, take a clear stance, and do not hedge. "+
				"Give your best argument in 3-5 sentences.\n\nPosition: %s",
			stance, string(prompt),
		)
		answer, reasoning, err := client.Handler()(ctx, []byte(injected))
		return answer, reasoning, err
	}
}

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

		alpha := ollamaadapter.New("llama3.2")
		beta  := ollamaadapter.New("mistral")
		arb   := ollamaadapter.New("llama3.2")

		participants := []*negotiation.Participant{
			{
				AgentID:   "agent-alpha",
				ModelName: "llama3.2",
				Role:      negotiation.RoleProposer,
				Handler:   debaHandler(alpha, "FOR"),
			},
			{
				AgentID:   "agent-beta",
				ModelName: "mistral",
				Role:      negotiation.RoleChallenger,
				Handler:   debaHandler(beta, "AGAINST"),
			},
			{
				AgentID:   "agent-arb",
				ModelName: "llama3.2-arb",
				Role:      negotiation.RoleArbitrator,
				Handler: func(ctx context.Context, prompt []byte) ([]byte, string, error) {
					injected := fmt.Sprintf(
						"You are an impartial judge in a debate. "+
							"Read the arguments FOR and AGAINST carefully. "+
							"Give a balanced verdict: which argument is stronger and why. "+
							"Be specific about which points were most convincing.\n\n%s",
						string(prompt),
					)
					return arb.Handler()(ctx, []byte(injected))
				},
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
