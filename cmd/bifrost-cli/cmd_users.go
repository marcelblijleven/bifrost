package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

func usersCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "users", Short: "Manage users"}
	cmd.AddCommand(usersListCmd(), usersCreateCmd(), usersDeleteCmd(), usersResetPasswordCmd(), usersSetAdminCmd())
	return cmd
}

func usersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all users",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			users, err := c.ListUsers(context.Background())
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				printJSON(users)
				return nil
			}
			if len(users) == 0 {
				fmt.Println("No users.")
				return nil
			}
			rows := make([][]string, len(users))
			for i, u := range users {
				admin := ""
				if u.IsAdmin {
					admin = "yes"
				}
				rows[i] = []string{u.ID[:8], u.Email, admin, u.CreatedAt.Format("2006-01-02")}
			}
			printTable([]string{"ID", "EMAIL", "ADMIN", "CREATED"}, rows)
			return nil
		},
	}
}

func usersCreateCmd() *cobra.Command {
	var email, password string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a user",
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" || password == "" {
				return fmt.Errorf("--email and --password are required")
			}
			c, err := resolveClient()
			if err != nil {
				return err
			}
			u, err := c.CreateUser(context.Background(), email, password)
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				printJSON(u)
				return nil
			}
			fmt.Printf(green+"✓"+reset+" Created user %s  id: %s\n", u.Email, u.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "User email (required)")
	cmd.Flags().StringVar(&password, "password", "", "User password (required)")
	return cmd
}

func usersDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				fmt.Printf("Delete user %s? [y/N] ", args[0])
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
			if err := c.DeleteUser(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Println(green + "✓" + reset + " Deleted.")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func usersSetAdminCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-admin <id> <true|false>",
		Short: "Grant or revoke admin rights",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			isAdmin, err := strconv.ParseBool(args[1])
			if err != nil {
				return fmt.Errorf("second argument must be true or false")
			}
			c, err := resolveClient()
			if err != nil {
				return err
			}
			if err := c.SetUserAdmin(context.Background(), args[0], isAdmin); err != nil {
				return err
			}
			if isAdmin {
				fmt.Println(green + "✓" + reset + " Admin rights granted.")
			} else {
				fmt.Println(green + "✓" + reset + " Admin rights revoked.")
			}
			return nil
		},
	}
}

func usersResetPasswordCmd() *cobra.Command {
	var newPassword string
	cmd := &cobra.Command{
		Use:   "reset-password <id>",
		Short: "Set a user's password without knowing their current one",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if newPassword == "" {
				newPassword = readPassword(bufio.NewReader(os.Stdin), "New password: ")
			}
			c, err := resolveClient()
			if err != nil {
				return err
			}
			if err := c.ResetUserPassword(context.Background(), args[0], newPassword); err != nil {
				return err
			}
			fmt.Println(green + "✓" + reset + " Password reset.")
			return nil
		},
	}
	cmd.Flags().StringVar(&newPassword, "new-password", "", "New password (use interactive prompt to avoid shell history)")
	return cmd
}
