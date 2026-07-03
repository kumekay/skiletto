package cli

import (
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	var force, global bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a skill from the manifest, lock, links, and disk",
		Long: "remove drops a skill from skiletto.toml and skiletto.lock, unlinks it " +
			"from every harness, and deletes its materialized copy. An editable skill " +
			"loses only its canonical link; the working tree is left untouched. A " +
			"drifted skill is refused unless --force, since removal discards local edits.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd, global)
			if err != nil {
				return err
			}
			return eng.Remove(args[0], force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false,
		"remove a drifted skill even though it has local modifications")
	cmd.Flags().BoolVar(&global, "global", false,
		"operate on the machine-scope manifest and lock instead of the current project")
	return cmd
}
