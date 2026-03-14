package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/AnantSingh1510/agentd/controlplane/api/proto"
	"github.com/AnantSingh1510/agentd/divergence"
)

var divergenceCmd = &cobra.Command{
	Use:   "divergence",
	Short: "Query the dissent dataset",
}

var divergenceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all recorded dissents",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := divergence.New("localhost:6379")
		if err != nil {
			return fmt.Errorf("store connection failed: %w", err)
		}
		defer store.Close()

		records, err := store.All(context.Background())
		if err != nil {
			return fmt.Errorf("query failed: %w", err)
		}

		if len(records) == 0 {
			fmt.Println("no dissents recorded yet")
			return nil
		}

		fmt.Printf("%-10s  %-14s  %-20s  %s\n", "ROUND", "MODEL", "PROMPT", "POSITION")
		fmt.Printf("%-10s  %-14s  %-20s  %s\n", "──────────", "──────────────", "────────────────────", "────────")
		for _, r := range records {
			prompt := r.Prompt
			if len(prompt) > 20 {
				prompt = prompt[:17] + "..."
			}
			position := r.Dissent.Position
			if len(position) > 60 {
				position = position[:57] + "..."
			}
			fmt.Printf("%-10s  %-14s  %-20s  %s\n",
				r.RoundID[:8],
				r.Dissent.ModelName,
				prompt,
				position,
			)
		}
		return nil
	},
}

var divergenceStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show dissent counts per model",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := divergence.New("localhost:6379")
		if err != nil {
			return fmt.Errorf("store connection failed: %w", err)
		}
		defer store.Close()

		stats, err := store.Stats(context.Background())
		if err != nil {
			return fmt.Errorf("stats failed: %w", err)
		}

		if len(stats) == 0 {
			fmt.Println("no dissents recorded yet")
			return nil
		}

		fmt.Printf("%-20s  %s\n", "MODEL", "DISSENTS")
		fmt.Printf("%-20s  %s\n", "────────────────────", "────────")
		for model, count := range stats {
			fmt.Printf("%-20s  %d\n", model, count)
		}
		return nil
	},
}

var divergenceNegotiateAndSaveCmd = &cobra.Command{
	Use:   "negotiate [prompt]",
	Short: "Run a negotiation round and persist dissents",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := grpc.NewClient(serverAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return fmt.Errorf("could not connect: %w", err)
		}
		defer conn.Close()

		client := proto.NewAgentDClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		fmt.Printf("starting negotiation...\n\n")

		resp, err := client.RunNegotiation(ctx, &proto.NegotiationRequest{
			TaskId: fmt.Sprintf("cli-%d", time.Now().UnixMilli()),
			Prompt: []byte(args[0]),
		})
		if err != nil {
			return fmt.Errorf("negotiation failed: %w", err)
		}

		fmt.Printf("round:     %s\n", resp.RoundId)
		fmt.Printf("answer:    %s\n\n", string(resp.FinalAnswer))

		if len(resp.Dissents) == 0 {
			fmt.Println("all models agreed — nothing to store")
			return nil
		}

		fmt.Printf("dissents (%d):\n", len(resp.Dissents))
		for _, d := range resp.Dissents {
			fmt.Printf("  [%s] %s\n", d.ModelName, d.Position[:min(len(d.Position), 120)])
		}

		return nil
	},
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	divergenceCmd.AddCommand(divergenceListCmd)
	divergenceCmd.AddCommand(divergenceStatsCmd)
	divergenceCmd.AddCommand(divergenceNegotiateAndSaveCmd)
	rootCmd.AddCommand(divergenceCmd)
}