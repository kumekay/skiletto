package cli

import (
	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Make installed skills match the lockfile exactly",
		Long: "sync installs exactly what skiletto.lock pins, resolves and locks " +
			"manifest entries that are not locked yet, and removes skills that are " +
			"locked but gone from the manifest. It never re-resolves already-locked " +
			"versions. Drifted skills (local modifications) are warned about and " +
			"skipped unless --force restores them.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := projectEngine(cmd)
			if err != nil {
				return err
			}
			return eng.Sync(force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false,
		"restore drifted skills to their locked version and allow pruning them")
	return cmd
}
