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

// version is the build version, reported by `skiletto --version`. It
// defaults to "dev" and is overridden at release time via -ldflags
// "-X github.com/kumekay/skiletto/internal/cli.version=<tag>".
var version = "dev"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skiletto",
		Short: "Package manager for agent skills",
		Long: "skiletto installs agent skills from git repositories, " +
			"pinning them to exact commits for reproducible setups.",
		Version:      version,
		SilenceUsage: true,
	}
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newSyncCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newRemoveCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newImportCmd())
	return cmd
}

// engineFor builds an engine for the selected scope, writing through the
// command's streams. global selects the machine scope (manifest and lock
// in the platform config dir, skills under the home dir); otherwise the
// project scope rooted at the current directory is used.
func engineFor(cmd *cobra.Command, global bool) (*engine.Engine, error) {
	sc, err := resolveScope(global)
	if err != nil {
		return nil, err
	}
	eng, err := engine.New(sc)
	if err != nil {
		return nil, err
	}
	eng.Out = cmd.OutOrStdout()
	eng.Err = cmd.ErrOrStderr()
	return eng, nil
}

// resolveScope maps the --global flag to a scope, reading the home and
// config dirs from the environment (HOME / XDG_CONFIG_HOME on Linux) so
// the machine scope can be redirected in tests and by end users.
func resolveScope(global bool) (scope.Scope, error) {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return scope.Scope{}, err
		}
		config, err := os.UserConfigDir()
		if err != nil {
			return scope.Scope{}, err
		}
		return scope.Machine(home, config), nil
	}
	root, err := os.Getwd()
	if err != nil {
		return scope.Scope{}, err
	}
	return scope.Project(root), nil
}

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}
