package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kumekay/skiletto/internal/cache"
	"github.com/kumekay/skiletto/internal/engine"
	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/scope"
)

// userCacheDir is os.UserCacheDir behind a variable so tests can pin it.
var userCacheDir = os.UserCacheDir

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Show where skiletto keeps its config, skills, and harness links",
		Long: "doctor reports the resolved machine config dir (which rule won: " +
			"SKILETTO_CONFIG_DIR, XDG_CONFIG_HOME, an existing ~/.config/skiletto, or " +
			"the platform default), the manifest and lockfile paths with their " +
			"exists/missing state, warnings about shadowed configs, the canonical " +
			"skills dir with the installed skills and their health, the user-wide " +
			"git repository cache (and how to clean it), the registered " +
			"harnesses with their link dirs, and — when run inside a project — the " +
			"project scope. It observes; it never writes.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := resolveMachine()
			if err != nil {
				return err
			}
			project, hasProject, err := projectScope(res.Scope.Root)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "skiletto version %s\n\n", version)
			if err := printMachineSection(out, res, project, hasProject); err != nil {
				return err
			}
			warnShadowed(cmd.ErrOrStderr(), res)
			return printProjectSection(out, res, project, hasProject)
		},
	}
}

// projectScope returns the nearest project scope containing the current
// directory. When none exists, it returns a scope rooted at the current
// directory for the diagnostic no-project message.
func projectScope(home string) (scope.Scope, bool, error) {
	start, err := os.Getwd()
	if err != nil {
		return scope.Scope{}, false, err
	}
	root, found, err := findProjectRoot(start, home)
	if err != nil {
		return scope.Scope{}, false, err
	}
	return scope.Project(root), found, nil
}

// printMachineSection reports the resolved machine config, its files, the
// installed skills with their health, and every registered harness with
// the scopes it is enabled in.
func printMachineSection(out io.Writer, res scope.Resolution, project scope.Scope, hasProject bool) error {
	machine := res.Scope
	_, _ = fmt.Fprintf(out, "Machine config\n")
	_, _ = fmt.Fprintf(out, "  config dir:      %s (%s)\n", filepath.Dir(machine.ManifestPath), res.Source)
	_, _ = fmt.Fprintf(out, "  manifest:        %s (%s)\n", machine.ManifestPath, fileState(machine.ManifestPath))
	_, _ = fmt.Fprintf(out, "  lockfile:        %s (%s)\n", machine.LockPath, fileState(machine.LockPath))
	_, _ = fmt.Fprintf(out, "  skills dir:      %s\n", machine.SkillsDir)
	if dir, err := cache.Dir(os.Getenv, userCacheDir); err == nil {
		clean := "rm -rf"
		if runtime.GOOS == "windows" {
			clean = "rmdir /s /q"
		}
		_, _ = fmt.Fprintf(out, "  cache dir:       %s\n", dir)
		_, _ = fmt.Fprintf(out, "                   clean: %s %s\n", clean, dir)
	}

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

	machineEnabled := harnessNames(machine.ManifestPath)
	projectEnabled := map[string]bool{}
	if hasProject {
		projectEnabled = harnessNames(project.ManifestPath)
	}
	_, _ = fmt.Fprintf(out, "\nHarnesses\n")
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, a := range eng.Adapters {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\n", a.Name(),
			harnessState(a.Name(), machineEnabled, projectEnabled), a.SkillsDir(machine))
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintln(out)
	return nil
}

// harnessState reports where a harness is enabled: the machine scope, the
// project doctor runs in, or both — so a project enablement can never hide
// behind a plain "disabled".
func harnessState(name string, machine, project map[string]bool) string {
	var where []string
	if machine[name] {
		where = append(where, "machine")
	}
	if project[name] {
		where = append(where, "project")
	}
	if len(where) == 0 {
		return "disabled"
	}
	return "enabled (" + strings.Join(where, ", ") + ")"
}

// harnessNames returns the harnesses key of the manifest at path; a
// missing or unreadable manifest counts as none.
func harnessNames(path string) map[string]bool {
	names := map[string]bool{}
	if m, err := manifest.Load(path); err == nil {
		for _, n := range m.Harnesses {
			names[n] = true
		}
	}
	return names
}

// printProjectSection reports the nearest project scope containing the
// current directory; otherwise it says no project was found.
func printProjectSection(out io.Writer, res scope.Resolution, project scope.Scope, hasProject bool) error {
	_, _ = fmt.Fprintf(out, "Project (%s)\n", project.Root)
	if !hasProject {
		_, _ = fmt.Fprintf(out, "  no skiletto.toml found from the current directory\n")
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
