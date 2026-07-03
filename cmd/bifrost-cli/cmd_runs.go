package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var terminalStatuses = map[string]bool{
	"success": true, "failed": true, "cancelled": true, "superseded": true,
}

func runsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "runs", Short: "Manage pipeline runs"}
	cmd.AddCommand(
		runsListCmd(),
		runsGetCmd(),
		runsWatchCmd(),
		runsCancelCmd(),
		runsRetryCmd(),
		runsApproveCmd(),
		runsRejectCmd(),
	)
	return cmd
}

func runsListCmd() *cobra.Command {
	var status, branch string
	var limit int

	cmd := &cobra.Command{
		Use:   "list <app-id>",
		Short: "List pipeline runs for an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			runs, err := c.ListRuns(context.Background(), args[0], status, branch, limit, 0)
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				printJSON(runs)
				return nil
			}
			if len(runs) == 0 {
				fmt.Println("No runs.")
				return nil
			}
			rows := make([][]string, len(runs))
			for i, r := range runs {
				tag := r.Tag
				if tag == "" {
					tag = "—"
				}
				msg := r.CommitMessage
				if len(msg) > 50 {
					msg = msg[:50] + "…"
				}
				rows[i] = []string{
					r.ID[:8],
					shortSHA(r.CommitSHA),
					tag,
					colorStatus(r.Status),
					fmtDuration(r.StartedAt, r.CompletedAt),
					msg,
				}
			}
			printTable([]string{"ID", "SHA", "TAG", "STATUS", "DURATION", "MESSAGE"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&branch, "branch", "", "Filter by branch")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max results")
	return cmd
}

func runsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <run-id>",
		Short: "Show run details and steps",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			run, err := c.GetRun(context.Background(), args[0])
			if err != nil {
				return err
			}
			steps, _ := c.ListSteps(context.Background(), args[0])
			if flagOutput == "json" {
				printJSON(map[string]any{"run": run, "steps": steps})
				return nil
			}
			renderRunDetail(run, steps)
			return nil
		},
	}
}

func runsWatchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch <run-id>",
		Short: "Watch a run's progress in real time (exits 1 on failure)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			return watchRunID(c, args[0])
		},
	}
}

func runsCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <run-id>",
		Short: "Cancel a pending or running run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			if err := c.CancelRun(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Println(green + "✓" + reset + " Run cancelled.")
			return nil
		},
	}
}

func runsRetryCmd() *cobra.Command {
	var stepIndex int
	cmd := &cobra.Command{
		Use:   "retry <run-id>",
		Short: "Retry a run from a specific step",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			if err := c.RetryStep(context.Background(), args[0], stepIndex); err != nil {
				return err
			}
			fmt.Printf(green+"✓"+reset+" Retrying run %s from step %d\n", args[0][:8], stepIndex)
			return nil
		},
	}
	cmd.Flags().IntVar(&stepIndex, "step", 0, "Step index to retry from (required)")
	cmd.MarkFlagRequired("step")
	return cmd
}

func runsApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve <run-id>",
		Short: "Approve the pending approval gate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			approvals, err := c.ListApprovals(context.Background(), args[0])
			if err != nil {
				return err
			}
			for _, a := range approvals {
				if a.Status == "pending" {
					if err := c.Approve(context.Background(), args[0], a.StepIndex); err != nil {
						return err
					}
					fmt.Printf(green+"✓"+reset+" Approved step %d (%s)\n", a.StepIndex, a.StepName)
					return nil
				}
			}
			return fmt.Errorf("no pending approval found for run %s", args[0])
		},
	}
}

func runsRejectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reject <run-id>",
		Short: "Reject the pending approval gate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			approvals, err := c.ListApprovals(context.Background(), args[0])
			if err != nil {
				return err
			}
			for _, a := range approvals {
				if a.Status == "pending" {
					if err := c.Reject(context.Background(), args[0], a.StepIndex); err != nil {
						return err
					}
					fmt.Printf(green+"✓"+reset+" Rejected step %d (%s)\n", a.StepIndex, a.StepName)
					return nil
				}
			}
			return fmt.Errorf("no pending approval found for run %s", args[0])
		},
	}
}

func renderRunDetail(run Run, steps []StepResult) {
	tag := run.Tag
	if tag == "" {
		tag = "—"
	}
	fmt.Printf("%sRun %s%s  •  %s  •  branch: %s  •  tag: %s\n",
		bold, run.ID[:8], reset, colorStatus(run.Status), run.Branch, tag)
	if run.CommitMessage != "" {
		fmt.Printf("%s%s%s\n", gray, run.CommitMessage, reset)
	}
	fmt.Printf("%s%s%s  triggered by: %s\n\n", gray, shortSHA(run.CommitSHA), reset, run.TriggeredBy)

	for _, s := range steps {
		dur := fmtDuration(s.StartedAt, s.CompletedAt)
		fmt.Printf("  %s  %-24s  %s%s%s",
			stepIcon(s.Status), s.StepName, gray, dur, reset)
		if s.Output != "" {
			out := s.Output
			if len(out) > 72 {
				out = out[:69] + "…"
			}
			fmt.Printf("  %s%s%s", gray, out, reset)
		}
		if s.ErrorMessage != "" {
			fmt.Printf("  %s%s%s", red, s.ErrorMessage, reset)
		}
		fmt.Println()
	}
	fmt.Printf("\nDuration: %s\n", fmtDuration(run.StartedAt, run.CompletedAt))
}

func watchRunID(c *Client, runID string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	redraw := func() (Run, error) {
		run, err := c.GetRun(ctx, runID)
		if err != nil {
			return run, err
		}
		steps, _ := c.ListSteps(ctx, runID)
		clearScreen()
		renderRunDetail(run, steps)
		return run, nil
	}

	// Try SSE first; fall back to polling on error.
	events, sseErr := c.sse(ctx, "/runs/"+runID+"/events")
	if sseErr != nil {
		return pollRunID(ctx, c, runID)
	}

	run, err := redraw()
	if err != nil {
		return err
	}
	if terminalStatuses[run.Status] {
		return runExitError(run.Status)
	}

	for range events {
		run, err = redraw()
		if err != nil {
			return err
		}
		if terminalStatuses[run.Status] {
			return runExitError(run.Status)
		}
	}

	// Channel closed — do one final check.
	run, err = c.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	return runExitError(run.Status)
}

func pollRunID(ctx context.Context, c *Client, runID string) error {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			run, err := c.GetRun(ctx, runID)
			if err != nil {
				return err
			}
			steps, _ := c.ListSteps(ctx, runID)
			clearScreen()
			renderRunDetail(run, steps)
			if terminalStatuses[run.Status] {
				return runExitError(run.Status)
			}
		}
	}
}

func runExitError(status string) error {
	if status == "success" {
		return nil
	}
	if terminalStatuses[status] {
		return fmt.Errorf("run ended with status: %s", status)
	}
	return nil
}
