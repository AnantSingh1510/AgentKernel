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

var submitCmd = &cobra.Command{
	Use:   "submit [payload]",
	Short: "Submit a task to the cluster",
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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := client.SubmitTask(ctx, &proto.SubmitTaskRequest{
			Payload: []byte(args[0]),
		})
		if err != nil {
			return fmt.Errorf("submit failed: %w", err)
		}

		fmt.Printf("task submitted\n")
		fmt.Printf("  id:     %s\n", resp.TaskId)
		fmt.Printf("  status: %s\n", resp.Status)
		if resp.WorkerId != "" {
			fmt.Printf("  worker: %s\n", resp.WorkerId)
		} else {
			fmt.Printf("  worker: queued — no available worker\n")
		}
		return nil
	},
}