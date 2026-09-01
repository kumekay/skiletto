// Package cli wires the skiletto command-line interface. Command handlers
// only parse flags and arguments and delegate to the engine.
package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	// Register compiled-in harness adapters.
	_ "github.com/kumekay/skiletto/internal/adapter/harness"
	"github.com/kumekay/skiletto/internal/engine"
	"github.com/kumekay/skiletto/internal/scope"
	"github.com/kumekay/skiletto/internal/ui"
)

// version is the build version, reported by `skiletto --version`. It
// defaults to "dev" and is overridden at release time via -ldflags
// "-X github.com/kumekay/skiletto/internal/cli.version=<tag>".
var version = "dev"

// userConfigDir is os.UserConfigDir behind a variable so tests can pin
// the platform config dir (e.g. exercise macOS behavior on any OS).
var userConfigDir = os.UserConfigDir

const projectBootstrapAnnotation = "skiletto.dev/project-bootstrap"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skiletto",
		Short: "Package manager for agent skills",
		Long: "skiletto installs agent skills from git repositories, " +
			"pinning them to exact commits for reproducible setups.",
		Version:      version,
		SilenceUsage: true,
	}
	cmd.PersistentFlags().BoolP("global", "g", false,
		"use the machine scope instead of the current project")
	cmd.PersistentFlags().Bool("no-input", false,
		"never prompt; where a prompt would appear, fail with an actionable error listing the flags to script the choice (implied when the CI env var is set)")
	// Registering the -v shorthand here keeps cobra's auto --version flag
	// long-only, so -v means verbose consistently on every command.
	cmd.PersistentFlags().BoolP("verbose", "v", false,
		"print extra diagnostics, including a line for each pre-install hook run")
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newSyncCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newRemoveCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newImportCmd())
	cmd.AddCommand(newHarnessCmd())
	cmd.AddCommand(newDoctorCmd())
	return cmd
}

// engineFor builds an engine for the selected scope, writing through the
// command's streams. The root --global flag selects the machine scope
// (manifest and lock in the platform config dir, skills under the home dir);
// otherwise the project scope is the nearest ancestor with a manifest. Add
// and import may bootstrap a project in the current directory. The machine
// scope is resolved either way: its harnesses apply in every scope.
func engineFor(cmd *cobra.Command) (*engine.Engine, error) {
	global, err := cmd.Flags().GetBool("global")
	if err != nil {
		return nil, err
	}
	res, err := resolveMachine()
	if err != nil {
		return nil, err
	}
	warnShadowed(cmd.ErrOrStderr(), res)
	machine := res.Scope
	sc := machine
	if !global {
		start, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		if sameDir(start, machine.Root) {
			return nil, fmt.Errorf("the current directory is your home directory, the machine scope root; pass --global (-g) to manage machine-wide skills")
		}
		root, found, err := scope.FindProjectRoot(start, machine.Root)
		if err != nil {
			return nil, err
		}
		if !found && cmd.Annotations[projectBootstrapAnnotation] != "true" {
			return nil, fmt.Errorf("no skiletto.toml found searching from %s; run this command inside a project, create one here with skiletto add or skiletto import, or pass --global (-g) to use the machine scope", start)
		}
		sc = scope.Project(root)
	}
	eng, err := engine.New(sc, machine)
	if err != nil {
		return nil, err
	}
	noInput, _ := cmd.Flags().GetBool("no-input")
	eng.PromptHarnesses = harnessPrompter(noInput)
	eng.Verbose, _ = cmd.Flags().GetBool("verbose")
	// Only commands that can install new content define --no-hooks; for the
	// rest the lookup errors and the default (hooks on) stands.
	if noHooks, err := cmd.Flags().GetBool("no-hooks"); err == nil {
		eng.NoHooks = noHooks
	}
	eng.Out = cmd.OutOrStdout()
	eng.Err = cmd.ErrOrStderr()
	if progressEnabled(ui.IsTerminalFile(os.Stderr), noInput, os.Getenv("CI")) && ui.EnableVT(os.Stderr) {
		p := ui.NewProgress(os.Stderr)
		eng.Progress = p
		// Route the engine's streams through the renderer so errors, notes,
		// and hook output never land mid-way through a transient status line.
		eng.Out = p.Writer(eng.Out)
		eng.Err = p.Writer(eng.Err)
	}
	return eng, nil
}

// progressEnabled reports whether per-skill progress may render: stderr
// must be a terminal, and --no-input or a non-empty CI env var force the
// plain (silent) output a script expects.
func progressEnabled(stderrTTY, noInput bool, ci string) bool {
	return stderrTTY && !noInput && ci == ""
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

// resolveMachine resolves the machine scope, honoring on every platform:
// SKILETTO_CONFIG_DIR, then XDG_CONFIG_HOME, then an existing
// ~/.config/skiletto/skiletto.toml (a dotfiles-synced config), and
// finally the platform default (%AppData% on Windows,
// ~/Library/Application Support on macOS, ~/.config on Linux). The
// fallback rescues setups where XDG_CONFIG_HOME is set in the shell but
// not exported to child processes. Writes go to the resolved path, so a
// fresh install lands in the platform default. See scope.ResolveMachine
// for the full precedence rules.
func resolveMachine() (scope.Resolution, error) {
	home := os.Getenv("HOME")
	if home == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return scope.Resolution{}, err
		}
	}
	return scope.ResolveMachine(home, os.Getenv, userConfigDir)
}

// warnShadowed reports machine manifests that exist below the winning
// one: they are never read or written, which usually means a stray
// config to merge or delete.
func warnShadowed(w io.Writer, res scope.Resolution) {
	for _, p := range res.Shadowed {
		_, _ = fmt.Fprintf(w, "warning: %s is shadowed by %s and will not be read; merge it or delete it\n",
			p, res.Scope.ManifestPath)
	}
}

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}
