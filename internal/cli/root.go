// Package cli wires the skiletto command-line interface. Command handlers
// only parse flags and arguments and delegate to the engine.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	// Register compiled-in harness adapters.
	_ "github.com/kumekay/skiletto/internal/adapter/harness"
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
	cmd.PersistentFlags().Bool("no-input", false,
		"never prompt; where a prompt would appear, fail with an actionable error listing the flags to script the choice (implied when the CI env var is set)")
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newSyncCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newRemoveCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newImportCmd())
	cmd.AddCommand(newHarnessCmd())
	return cmd
}

// engineFor builds an engine for the selected scope, writing through the
// command's streams. global selects the machine scope (manifest and lock
// in the platform config dir, skills under the home dir); otherwise the
// project scope rooted at the current directory is used. The machine scope
// is resolved either way: its harnesses apply in every scope.
func engineFor(cmd *cobra.Command, global bool) (*engine.Engine, error) {
	machine, err := machineScope()
	if err != nil {
		return nil, err
	}
	sc := machine
	if !global {
		root, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		if sameDir(root, machine.Root) {
			return nil, fmt.Errorf("the current directory is your home directory, the machine scope root; pass --global (-g) to manage machine-wide skills")
		}
		sc = scope.Project(root)
	}
	eng, err := engine.New(sc, machine)
	if err != nil {
		return nil, err
	}
	noInput, _ := cmd.Flags().GetBool("no-input")
	eng.PromptHarnesses = harnessPrompter(noInput)
	// Only commands that can install new content define --no-hooks; for the
	// rest the lookup errors and the default (hooks on) stands.
	if noHooks, err := cmd.Flags().GetBool("no-hooks"); err == nil {
		eng.NoHooks = noHooks
	}
	eng.Out = cmd.OutOrStdout()
	eng.Err = cmd.ErrOrStderr()
	return eng, nil
}

// sameDir reports whether two paths name the same directory, comparing the
// actual filesystem entries so symlinked homes and cosmetic path
// differences cannot dodge the check.
func sameDir(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}

// machineScope resolves the machine scope, reading the home and config
// dirs from the environment (HOME / XDG_CONFIG_HOME) so it can be
// redirected in tests and by end users. The env is honored on every
// platform; where it is unset the OS defaults apply (%USERPROFILE% and
// %AppData% on Windows, ~ and ~/.config on Linux).
func machineScope() (scope.Scope, error) {
	home := os.Getenv("HOME")
	if home == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return scope.Scope{}, err
		}
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		var err error
		if config, err = os.UserConfigDir(); err != nil {
			return scope.Scope{}, err
		}
	}
	return scope.Machine(home, config), nil
}

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}
