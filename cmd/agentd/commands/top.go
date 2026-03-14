package commands

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/AnantSingh1510/agentd/cmd/agentd/tui"
)

var topCmd = &cobra.Command{
	Use:   "top",
	Short: "Live dashboard — workers, metrics, divergence feed",
	RunE: func(cmd *cobra.Command, args []string) error {
		m := tui.NewModel(serverAddr)

		if err := m.Connect(); err != nil {
			return fmt.Errorf("could not connect to %s: %w", serverAddr, err)
		}

		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "dashboard error: %v\n", err)
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(topCmd)
}