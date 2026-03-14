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

var workersCmd = &cobra.Command{
	Use:   "workers",
	Short: "List all registered workers and their load",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := grpc.NewClient(serverAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return fmt.Errorf("could not connect to %s: %w", serverAddr, err)
		}
		defer conn.Close()

		client := proto.NewAgentDClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := client.ListWorkers(ctx, &proto.ListWorkersRequest{})
		if err != nil {
			return fmt.Errorf("list workers failed: %w", err)
		}

		if len(resp.Workers) == 0 {
			fmt.Println("no workers registered")
			return nil
		}

		fmt.Printf("%-30s  %8s  %8s  %s\n", "ID", "CAPACITY", "ACTIVE", "LOAD")
		fmt.Printf("%-30s  %8s  %8s  %s\n", "──────────────────────────────", "────────", "──────", "────")
		for _, w := range resp.Workers {
			load := "idle"
			if w.Active > 0 && w.Active < w.Capacity {
				load = "busy"
			} else if w.Active >= w.Capacity {
				load = "full"
			}
			fmt.Printf("%-30s  %8d  %8d  %s\n", w.Id, w.Capacity, w.Active, load)
		}
		return nil
	},
}