package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kumekay/skiletto/internal/engine"
	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/scope"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Show where skiletto keeps its config, skills, and harness links",
		Long: "doctor reports the resolved machine config dir (which rule won: " +
			"SKILETTO_CONFIG_DIR, XDG_CONFIG_HOME, an existing ~/.config/skiletto, or " +
			"the platform default), the manifest and lockfile paths with their " +
			"exists/missing state, warnings about shadowed configs, the canonical " +
			"skills dir with the installed skills and their health, the registered " +
			"harnesses with their link dirs, and — when run inside a project — the " +
			"project scope. It observes; it never writes.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := resolveMachine()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "skiletto version %s\n\n", version)
			if err := printMachineSection(out, res); err != nil {
				return err
			}
			printShadowed(out, res)
			return printProjectSection(out, res)
		},
	}
}

// printMachineSection reports the resolved machine config, its files, the
// installed skills with their health, and every registered harness.
func printMachineSection(out io.Writer, res scope.Resolution) error {
	machine := res.Scope
	_, _ = fmt.Fprintf(out, "Machine config\n")
	_, _ = fmt.Fprintf(out, "  config dir:      %s (%s)\n", filepath.Dir(machine.ManifestPath), res.Source)
	_, _ = fmt.Fprintf(out, "  manifest:        %s (%s)\n", machine.ManifestPath, fileState(machine.ManifestPath))
	_, _ = fmt.Fprintf(out, "  lockfile:        %s (%s)\n", machine.LockPath, fileState(machine.LockPath))
	_, _ = fmt.Fprintf(out, "  skills dir:      %s\n", machine.SkillsDir)

	eng, err := engine.New(machine, machine)
	if err != nil {
		return err
	}
	eng.Out = out
	eng.Err = io.Discard
	statuses, err := eng.Status()
	if err != nil {
		return err
	}
	managed := 0
	for _, s := range statuses {
		if s.Status != "unmanaged" {
			managed++
		}
	}
	_, _ = fmt.Fprintf(out, "  installed:       %d skill(s)\n", managed)
	printSkillHealth(out, statuses)

	enabled := map[string]bool{}
	if m, err := manifest.Load(machine.ManifestPath); err == nil {
		for _, n := range m.Harnesses {
			enabled[n] = true
		}
	}
	_, _ = fmt.Fprintf(out, "\nHarnesses\n")
	for _, a := range eng.Adapters {
		state := "disabled"
		if enabled[a.Name()] {
			state = "enabled"
		}
		_, _ = fmt.Fprintf(out, "  %-13s %-8s %s\n", a.Name(), state, a.SkillsDir(machine))
	}
	_, _ = fmt.Fprintln(out)
	return nil
}

// printShadowed lists machine manifests that exist but lose to the
// resolved one, so stale copies cannot silently diverge.
func printShadowed(out io.Writer, res scope.Resolution) {
	for _, p := range res.Shadowed {
		_, _ = fmt.Fprintf(out, "warning: %s is shadowed by %s and will not be read; merge it or delete it\n",
			p, res.Scope.ManifestPath)
	}
	if len(res.Shadowed) > 0 {
		_, _ = fmt.Fprintln(out)
	}
}

// printProjectSection reports the project scope of the current directory
// when it holds a skiletto.toml; otherwise it says so.
func printProjectSection(out io.Writer, res scope.Resolution) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Project (%s)\n", root)
	project := scope.Project(root)
	if _, err := os.Stat(project.ManifestPath); err != nil {
		_, _ = fmt.Fprintf(out, "  no skiletto.toml in the current directory\n")
		return nil
	}
	_, _ = fmt.Fprintf(out, "  manifest:        %s (%s)\n", project.ManifestPath, fileState(project.ManifestPath))
	_, _ = fmt.Fprintf(out, "  lockfile:        %s (%s)\n", project.LockPath, fileState(project.LockPath))
	_, _ = fmt.Fprintf(out, "  skills dir:      %s\n", project.SkillsDir)

	eng, err := engine.New(project, res.Scope)
	if err != nil {
		return err
	}
	eng.Out = out
	eng.Err = io.Discard
	statuses, err := eng.Status()
	if err != nil {
		return err
	}
	printSkillHealth(out, statuses)
	return nil
}

// printSkillHealth renders one line per skill with the same columns as
// list, so drift and the other problems stand out in the report. Empty
// input prints nothing.
func printSkillHealth(out io.Writer, statuses []engine.SkillStatus) {
	if len(statuses) == 0 {
		return
	}
	_, _ = fmt.Fprintf(out, "  skill health:\n")
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "    NAME	VERSION	STATUS	SOURCE")
	for _, s := range statuses {
		_, _ = fmt.Fprintf(tw, "    %s	%s	%s	%s\n", s.Name, versionCell(s), s.Status, dash(s.Source))
	}
	_ = tw.Flush()
}

// versionCell is the VERSION column: "editable", a short commit, or "-".
func versionCell(s engine.SkillStatus) string {
	if s.Editable {
		return "editable"
	}
	return dash(s.Commit)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// fileState reports whether path exists.
func fileState(path string) string {
	if _, err := os.Stat(path); err == nil {
		return "exists"
	}
	return "missing"
}
