package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show dashboard summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			stats, err := c.GetDashboard(context.Background())
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				printJSON(stats)
				return nil
			}
			fmt.Printf("%sBifrost Status%s\n\n", bold, reset)
			fmt.Printf("  Total runs:    %d\n", stats.TotalRuns)
			fmt.Printf("  Succeeded:     %s%d%s\n", green, stats.SucceededRuns, reset)
			fmt.Printf("  Failed:        %s%d%s\n", red, stats.FailedRuns, reset)
			fmt.Printf("  Avg duration:  %.0fs\n", stats.AvgDurationSeconds)

			if len(stats.PendingActions) > 0 {
				fmt.Printf("\n%s%d pending action(s):%s\n", yellow, len(stats.PendingActions), reset)
				for _, a := range stats.PendingActions {
					fmt.Printf("  • [%s] %s — %s\n", a.Type, a.ApplicationName, a.Message)
				}
			}
			return nil
		},
	}
}
