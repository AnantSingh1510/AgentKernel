package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/AnantSingh1510/agentd/controlplane/api/proto"
)

var negotiateCmd = &cobra.Command{
	Use:   "negotiate [prompt]",
	Short: "Run a multi-model negotiation round",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := grpc.NewClient(serverAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return fmt.Errorf("could not connect to %s: %w", serverAddr, err)
		}
		defer conn.Close()

		client := proto.NewAgentDClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
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
			fmt.Println("all models agreed")
		} else {
			fmt.Printf("dissents (%d):\n", len(resp.Dissents))
			for _, d := range resp.Dissents {
				pos := d.Position
				if len(pos) > 120 {
					pos = pos[:117] + "..."
				}
				fmt.Printf("  [%s] %s\n", d.ModelName, pos)
			}
		}
		return nil
	},
}
