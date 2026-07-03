// Package cli wires the skiletto command-line interface. Command handlers
// only parse flags and arguments and delegate to the engine.
package cli

import (
	"os"

	"github.com/spf13/cobra"

	// Register compiled-in harness adapters.
	_ "github.com/kumekay/skiletto/internal/adapter/claude"
	"github.com/kumekay/skiletto/internal/engine"
	"github.com/kumekay/skiletto/internal/scope"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skiletto",
		Short: "Package manager for agent skills",
		Long: "skiletto installs agent skills from git repositories, " +
			"pinning them to exact commits for reproducible setups.",
		SilenceUsage: true,
	}
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newSyncCmd())
	return cmd
}

// projectEngine builds an engine for the project scope rooted at the
// current directory, writing through the command's streams.
func projectEngine(cmd *cobra.Command) (*engine.Engine, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	eng, err := engine.New(scope.Project(root))
	if err != nil {
		return nil, err
	}
	eng.Out = cmd.OutOrStdout()
	eng.Err = cmd.ErrOrStderr()
	return eng, nil
}

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}
