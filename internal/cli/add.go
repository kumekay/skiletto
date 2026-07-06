package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kumekay/skiletto/internal/engine"
	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/ui"
)

// promptSelector builds the Prompter the add command uses for an ambiguous
// source. It is a package variable so tests can inject a fake prompter
// without a real terminal.
var promptSelector = func(noInput bool) ui.Prompter {
	return ui.Select(ui.SelectOpts{
		StdinTTY:  ui.IsTerminalFile(os.Stdin),
		StdoutTTY: ui.IsTerminalFile(os.Stdout),
		NoInput:   noInput,
		CI:        os.Getenv("CI"),
	})
}

func newAddCmd() *cobra.Command {
	var editable, global, all bool
	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Add a skill: resolve, lock, install, and link it",
		Long: "add records a skill in skiletto.toml, resolves its ref to a commit, " +
			"pins it in skiletto.lock, materializes it, and links it into every enabled harness (see 'skiletto harness').\n\n" +
			"The source is <repo>[//subdir][@ref]: a git URL, an owner/repo GitHub " +
			"shorthand, or a local path (with --editable, the working tree is " +
			"linked instead of copied).\n\n" +
			"When the source holds several skills and no //path picks one, add shows " +
			"a multi-select picker in a terminal; --all installs every skill, and " +
			"without a TTY (or with --no-input) it prints the skills and the exact " +
			"commands to script the choice.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := manifest.ParseSourceSpec(args[0])
			if err != nil {
				return err
			}
			if err := absolutizePathSource(&spec); err != nil {
				return err
			}
			if all && spec.Path != "" {
				return fmt.Errorf("--all installs every skill in the source and cannot be combined with an explicit //path (%q)", spec.Path)
			}
			eng, err := engineFor(cmd, global)
			if err != nil {
				return err
			}
			if all {
				return eng.AddAll(spec, editable)
			}

			err = eng.Add(spec, editable)
			var multi *engine.MultipleSkillsError
			if !errors.As(err, &multi) {
				return err
			}

			noInput, _ := cmd.Flags().GetBool("no-input")
			selected, err := promptSelector(noInput).MultiSelect(
				fmt.Sprintf("Select skills to add from %s", multi.Source),
				pickerOptions(multi, editable),
			)
			if err != nil {
				return err
			}
			if len(selected) == 0 {
				return errors.New("no skills selected; nothing added")
			}
			return eng.AddSelected(spec, selected, editable)
		},
	}
	cmd.Flags().BoolVar(&editable, "editable", false,
		"link the working tree of a local path source instead of copying a pinned commit")
	cmd.Flags().BoolVarP(&global, "global", "g", false,
		"install for the whole machine (config dir manifest, skills under ~/.agents/skills) instead of the current project")
	cmd.Flags().BoolVar(&all, "all", false,
		"install every skill discovered in the source without prompting")
	return cmd
}

// absolutizePathSource rewrites a relative path source to an absolute one
// against the invocation cwd, so the manifest and lockfile always store a
// usable path (a relative source would otherwise be resolved against the
// canonical skills dir, producing a broken editable symlink). Home-relative
// (~/...) sources are left untouched: they stay portable and are expanded at
// use time. Non-path sources (git URLs, shorthands) are never touched.
func absolutizePathSource(spec *manifest.SourceSpec) error {
	if !spec.IsPath {
		return nil
	}
	if spec.Source == "~" || strings.HasPrefix(spec.Source, "~/") {
		return nil
	}
	if filepath.IsAbs(spec.Source) {
		return nil
	}
	abs, err := filepath.Abs(spec.Source)
	if err != nil {
		return err
	}
	spec.Source = abs
	return nil
}

// pickerOptions turns the discovered skills into picker options, each
// carrying the exact command that scripts selecting it alone.
func pickerOptions(multi *engine.MultipleSkillsError, editable bool) []ui.Option {
	opts := make([]ui.Option, len(multi.Skills))
	for i, sub := range multi.Skills {
		hint := "skiletto add "
		if editable {
			hint += "--editable "
		}
		hint += multi.Source + "//" + sub
		if multi.Ref != "" {
			hint += "@" + multi.Ref
		}
		opts[i] = ui.Option{Label: sub, Value: sub, Hint: hint}
	}
	return opts
}
