package cli

import (
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "update [name]",
		Short: "Re-resolve refs and rewrite lock entries to the current commit",
		Long: "update is the only command that moves already-locked versions: it " +
			"re-resolves each entry's ref (or default branch) to the current commit, " +
			"re-materializes and re-links the content, and rewrites skiletto.lock. " +
			"With no argument it updates every skill; with a name, only that one. " +
			"Editable entries have nothing to re-resolve and are skipped. Drifted " +
			"skills are warned about and skipped unless --force overwrites them.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			var name string
			if len(args) == 1 {
				name = args[0]
			}
			return eng.Update(name, force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false,
		"overwrite drifted skills with the freshly re-resolved version")
	cmd.Flags().Bool("no-hooks", false,
		"skip the pre-install hook configured under [hooks] in skiletto.toml")
	return cmd
}
