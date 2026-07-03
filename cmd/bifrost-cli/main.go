package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	flagOutput string
	flagURL    string
	flagToken  string
)

// version is stamped at build time via -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:               "bifrost",
		Short:             "Bifrost release orchestration CLI",
		Version:           version,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}

	root.PersistentFlags().StringVarP(&flagOutput, "output", "o", "table", "Output format: table or json")
	root.PersistentFlags().StringVar(&flagURL, "url", "", "Bifrost server URL (overrides config and BIFROST_URL)")
	root.PersistentFlags().StringVar(&flagToken, "token", "", "API token (overrides config and BIFROST_TOKEN)")

	root.AddCommand(
		loginCmd(),
		whoamiCmd(),
		passwdCmd(),
		appsCmd(),
		runsCmd(),
		usersCmd(),
		groupsCmd(),
		statusCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func resolveClient() (*Client, error) {
	cfg := loadConfig()

	url := flagURL
	if url == "" {
		url = os.Getenv("BIFROST_URL")
	}
	if url == "" {
		url = cfg.URL
	}
	if url == "" {
		return nil, fmt.Errorf("no server URL configured — run `bifrost login` or set BIFROST_URL")
	}

	token := flagToken
	if token == "" {
		token = os.Getenv("BIFROST_TOKEN")
	}
	if token == "" {
		token = cfg.Token
	}

	return newClient(url, token), nil
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
