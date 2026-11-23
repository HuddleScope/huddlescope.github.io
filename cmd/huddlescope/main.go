package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/HuddleScope/huddlescope.github.io/internal/cli"
)

func main() {
	// Create a context that respects interrupt signals
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create root command with default dependencies
	rootCmd := cli.NewRootCommand(
		cli.WithOutput(os.Stdout),
		cli.WithErrorOutput(os.Stderr),
		cli.WithLoadConfig(cli.DefaultLoadConfig),
		cli.WithNewRepository(cli.DefaultNewRepository),
		cli.WithNow(cli.DefaultNow),
	)

	// Execute the root command
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		// Map sentinel errors to exit codes
		exitCode := 1
		if exitErr, ok := err.(*cli.ExitError); ok {
			exitCode = exitErr.Code
		}
		os.Exit(exitCode)
	}
}
