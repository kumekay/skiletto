package cli

import (
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List managed skills with drift status, plus unmanaged skills",
		Long: "list shows each managed skill with its pinned commit (or 'editable') " +
			"and status (ok, drifted, missing, not-locked), followed by any unmanaged " +
			"skills found in the canonical skills dir but absent from the manifest. It " +
			"only observes: it never installs, removes, or restores anything.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := engineFor(cmd)
			if err != nil {
				return err
			}
			return eng.List()
		},
	}
	return cmd
}
