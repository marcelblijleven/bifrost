package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func appsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "apps", Short: "Manage applications"}
	cmd.AddCommand(appsListCmd(), appsGetCmd(), appsCreateCmd(), appsDeleteCmd())
	return cmd
}

func appsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all applications",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			apps, err := c.ListApplications(context.Background())
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				printJSON(apps)
				return nil
			}
			if len(apps) == 0 {
				fmt.Println("No applications.")
				return nil
			}
			rows := make([][]string, len(apps))
			for i, a := range apps {
				rows[i] = []string{a.ID[:8], a.Name, a.Provider, a.Owner + "/" + a.Repo, a.Branch}
			}
			printTable([]string{"ID", "NAME", "PROVIDER", "REPO", "BRANCH"}, rows)
			return nil
		},
	}
}

func appsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show application details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			app, err := c.GetApplication(context.Background(), args[0])
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				printJSON(app)
				return nil
			}
			fmt.Printf("ID:       %s\nName:     %s\nProvider: %s\nRepo:     %s/%s\nBranch:   %s\nCreated:  %s\n",
				app.ID, app.Name, app.Provider, app.Owner, app.Repo, app.Branch,
				app.CreatedAt.Format("2006-01-02 15:04"))
			return nil
		},
	}
}

func appsCreateCmd() *cobra.Command {
	var name, provider, owner, repo, branch, stepsJSON string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an application",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" || owner == "" || repo == "" {
				return fmt.Errorf("--name, --owner, and --repo are required")
			}
			body := map[string]any{
				"Name": name, "Provider": provider,
				"Owner": owner, "Repo": repo, "Branch": branch,
				"WebhookSecret": "",
			}
			if stepsJSON != "" {
				var steps []map[string]any
				if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
					return fmt.Errorf("invalid --steps JSON: %w", err)
				}
				body["PipelineSteps"] = steps
			}
			c, err := resolveClient()
			if err != nil {
				return err
			}
			app, err := c.CreateApplication(context.Background(), body)
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				printJSON(app)
				return nil
			}
			fmt.Printf(green+"✓"+reset+" Created %s  id: %s\n", app.Name, app.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Application name (required)")
	cmd.Flags().StringVar(&provider, "provider", "github", "Provider")
	cmd.Flags().StringVar(&owner, "owner", "", "Repository owner (required)")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository name (required)")
	cmd.Flags().StringVar(&branch, "branch", "main", "Target branch")
	cmd.Flags().StringVar(&stepsJSON, "steps", "", `Pipeline steps JSON, e.g. '[{"type":"semver"},{"type":"tag"}]'`)
	return cmd
}

func appsDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				fmt.Printf("Delete application %s? [y/N] ", args[0])
				var confirm string
				fmt.Scanln(&confirm)
				if confirm != "y" && confirm != "Y" {
					fmt.Println("Cancelled.")
					return nil
				}
			}
			c, err := resolveClient()
			if err != nil {
				return err
			}
			if err := c.DeleteApplication(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Println(green + "✓" + reset + " Deleted.")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}
