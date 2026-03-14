package commands

import (
	"os"

	"github.com/spf13/cobra"
)

var serverAddr string

var rootCmd = &cobra.Command{
	Use:   "agentd",
	Short: "agentd — distributed AI agent runtime",
	Long:  `agentd manages AI agents the way container platforms manage containers.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "localhost:50051", "agentd server address")
	rootCmd.AddCommand(submitCmd)
	rootCmd.AddCommand(workersCmd)
	rootCmd.AddCommand(negotiateCmd)
	rootCmd.AddCommand(serveCmd)
}