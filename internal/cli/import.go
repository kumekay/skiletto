package cli

import (
	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	var force, global bool
	cmd := &cobra.Command{
		Use:   "import [path]",
		Short: "Bootstrap skiletto.toml and skiletto.lock from a Vercel skills-lock.json",
		Long: "import reads a Vercel `npx skills` skills-lock.json (default: " +
			"skills-lock.json in the current directory), maps each entry to a " +
			"canonical git source, resolves its default-branch HEAD to a commit, " +
			"and writes a fully pinned skiletto.toml and skiletto.lock, installing " +
			"and linking each skill like sync.\n\n" +
			"Vercel's lock stores no git ref or commit, so import pins the current " +
			"HEAD. Entries already in the manifest are skipped; entries that cannot " +
			"be mapped or resolved are reported and cause a non-zero exit without " +
			"stopping the ones that do resolve. Import is one-way.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "skills-lock.json"
			if len(args) == 1 {
				path = args[0]
			}
			eng, err := engineFor(cmd, global)
			if err != nil {
				return err
			}
			return eng.Import(path, force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false,
		"overwrite installed skills that import cannot prove pristine (drifted lock orphans or unmanaged trees)")
	cmd.Flags().BoolVarP(&global, "global", "g", false,
		"write the machine-scope manifest and lock (config dir, skills under ~/.agents/skills) instead of the current project")
	cmd.Flags().Bool("no-hooks", false,
		"skip the pre-install hook configured under [hooks] in skiletto.toml")
	return cmd
}
