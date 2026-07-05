package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/kumekay/skiletto/internal/engine"
	"github.com/kumekay/skiletto/internal/ui"
)

// harnessPrompter returns the one-time harness picker for a scope with no
// harnesses key, or nil when the environment is non-interactive — the
// engine then installs to the canonical dir only, with a note. It is a
// package variable so tests can inject a fake picker.
var harnessPrompter = func(noInput bool) func([]engine.HarnessOption) ([]string, error) {
	opts := ui.SelectOpts{
		StdinTTY:  ui.IsTerminalFile(os.Stdin),
		StdoutTTY: ui.IsTerminalFile(os.Stdout),
		NoInput:   noInput,
		CI:        os.Getenv("CI"),
	}
	if !opts.Interactive() {
		return nil
	}
	return func(harnesses []engine.HarnessOption) ([]string, error) {
		options := make([]ui.Option, len(harnesses))
		for i, h := range harnesses {
			label := h.Name
			if h.Detected {
				label += " (detected)"
			}
			options[i] = ui.Option{
				Label:    label,
				Value:    h.Name,
				Hint:     "skiletto harness enable " + h.Name,
				Selected: h.Detected,
			}
		}
		return ui.Select(opts).MultiSelect(
			"Select harnesses to link skills into (saved to the manifest)", options)
	}
}

func newHarnessCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "harness",
		Short: "Configure which harnesses skills are linked into",
		Long: "Skills always materialize in the canonical .agents/skills directory. " +
			"Harnesses (e.g. Claude Code) that read their own directory instead get " +
			"per-skill links, controlled by the harnesses key in skiletto.toml. The " +
			"effective set is the union of the project manifest's key and the machine " +
			"manifest's (~/.config/skiletto/skiletto.toml), so personal harnesses apply " +
			"in every project without touching shared files.",
	}
	cmd.AddCommand(newHarnessListCmd())
	cmd.AddCommand(newHarnessEnableCmd())
	cmd.AddCommand(newHarnessDisableCmd())
	return cmd
}

func newHarnessListCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show registered harnesses, where they are enabled, and their link dirs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd, global)
			if err != nil {
				return err
			}
			return eng.HarnessList()
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false,
		"report for the machine scope instead of the current project")
	return cmd
}

func newHarnessEnableCmd() *cobra.Command {
	var global, force bool
	cmd := &cobra.Command{
		Use:   "enable <name>...",
		Short: "Enable harnesses in this scope and link all installed skills into them",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd, global)
			if err != nil {
				return err
			}
			return eng.HarnessEnable(args, force)
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false,
		"enable machine-wide (applies in every scope) instead of for the current project")
	cmd.Flags().BoolVar(&force, "force", false,
		"replace a copy-linked install that has diverged from the canonical tree")
	return cmd
}

func newHarnessDisableCmd() *cobra.Command {
	var global, force bool
	cmd := &cobra.Command{
		Use:   "disable <name>...",
		Short: "Disable harnesses in this scope and remove their skill links",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd, global)
			if err != nil {
				return err
			}
			return eng.HarnessDisable(args, force)
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false,
		"disable machine-wide instead of for the current project")
	cmd.Flags().BoolVar(&force, "force", false,
		"also remove a copy-linked install that has diverged from the canonical tree")
	return cmd
}
