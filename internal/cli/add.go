package cli

import (
	"github.com/spf13/cobra"

	"github.com/kumekay/skiletto/internal/manifest"
)

func newAddCmd() *cobra.Command {
	var editable, global bool
	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Add a skill: resolve, lock, install, and link it",
		Long: "add records a skill in skiletto.toml, resolves its ref to a commit, " +
			"pins it in skiletto.lock, materializes it, and links it into every harness.\n\n" +
			"The source is <repo>[//subdir][@ref]: a git URL, an owner/repo GitHub " +
			"shorthand, or a local path (with --editable, the working tree is " +
			"linked instead of copied).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := manifest.ParseSourceSpec(args[0])
			if err != nil {
				return err
			}
			eng, err := engineFor(cmd, global)
			if err != nil {
				return err
			}
			return eng.Add(spec, editable)
		},
	}
	cmd.Flags().BoolVar(&editable, "editable", false,
		"link the working tree of a local path source instead of copying a pinned commit")
	cmd.Flags().BoolVar(&global, "global", false,
		"install for the whole machine (config dir manifest, skills under ~/.agents/skills) instead of the current project")
	return cmd
}
