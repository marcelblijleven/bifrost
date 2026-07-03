package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func loginCmd() *cobra.Command {
	var url, email, password string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate and save credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := bufio.NewReader(os.Stdin)

			if url == "" {
				fmt.Print("Server URL: ")
				line, _ := r.ReadString('\n')
				url = strings.TrimSpace(line)
			}
			if email == "" {
				fmt.Print("Email: ")
				line, _ := r.ReadString('\n')
				email = strings.TrimSpace(line)
			}
			if password == "" {
				password = readPassword(r, "Password: ")
			}

			c := newClient(url, "")
			token, err := c.Login(context.Background(), email, password)
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}
			if err := saveConfig(Config{URL: url, Token: token}); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf(green+"✓"+reset+" Logged in. Config saved to %s\n", configPath())
			return nil
		},
	}

	cmd.Flags().StringVar(&url, "url", "", "Server URL")
	cmd.Flags().StringVar(&email, "email", "", "Email")
	cmd.Flags().StringVar(&password, "password", "", "Password (use interactive prompt to avoid shell history)")
	return cmd
}

func passwdCmd() *cobra.Command {
	var current, newPassword string

	cmd := &cobra.Command{
		Use:   "passwd",
		Short: "Change your own password",
		RunE: func(cmd *cobra.Command, args []string) error {
			r := bufio.NewReader(os.Stdin)
			if current == "" {
				current = readPassword(r, "Current password: ")
			}
			if newPassword == "" {
				newPassword = readPassword(r, "New password: ")
			}
			c, err := resolveClient()
			if err != nil {
				return err
			}
			if err := c.ChangePassword(context.Background(), current, newPassword); err != nil {
				return fmt.Errorf("change password failed: %w", err)
			}
			fmt.Println(green + "✓" + reset + " Password changed.")
			return nil
		},
	}

	cmd.Flags().StringVar(&current, "current-password", "", "Current password (use interactive prompt to avoid shell history)")
	cmd.Flags().StringVar(&newPassword, "new-password", "", "New password (use interactive prompt to avoid shell history)")
	return cmd
}

// readPassword prompts on stdout and reads a password from stdin, masking
// input when connected to a terminal and falling back to a plain line read
// otherwise (e.g. when piped in tests or scripts).
func readPassword(r *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	b, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		line, _ := r.ReadString('\n')
		return strings.TrimSpace(line)
	}
	return string(b)
}

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the currently authenticated user",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			me, err := c.Me(context.Background())
			if err != nil {
				return err
			}
			if flagOutput == "json" {
				printJSON(me)
				return nil
			}
			fmt.Printf("user_id:  %s\nemail:    %s\n", me["user_id"], me["email"])
			return nil
		},
	}
}
