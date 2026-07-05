package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kumekay/skiletto/internal/adapter"
	"github.com/kumekay/skiletto/internal/manifest"
)

// HarnessOption describes one registered harness adapter to the one-time
// configuration prompt. Detected reports whether the harness appears
// installed on this machine.
type HarnessOption struct {
	Name     string
	Detected bool
}

// resolveHarnesses returns the adapters enabled for this run: the union of
// the scope manifest's harnesses key and the machine manifest's (personal
// harnesses apply in every project). When neither scope has the key, an
// interactive run asks once and persists the answer to the scope manifest;
// a non-interactive run proceeds canonical-only with a note — never an
// error, because installing without links is always safe. Unknown names
// (from a newer or misspelled config) warn and are skipped.
//
// allowPrompt is false for commands where a first-time prompt would be a
// surprise (remove, update); they fall back the same way.
func (e *Engine) resolveHarnesses(m *manifest.Manifest, allowPrompt bool) ([]adapter.Adapter, error) {
	names := m.Harnesses
	machine := e.machineHarnesses(m)
	if machine != nil {
		names = unionNames(names, machine)
	}
	configured := m.Harnesses != nil || machine != nil

	if !configured {
		if allowPrompt && e.PromptHarnesses != nil {
			chosen, err := e.PromptHarnesses(e.harnessOptions())
			if err != nil {
				return nil, err
			}
			if chosen == nil {
				chosen = []string{}
			}
			m.Harnesses = chosen
			if err := m.Save(e.Scope.ManifestPath); err != nil {
				return nil, err
			}
			names = chosen
		} else {
			_, _ = fmt.Fprintf(e.Out, "note: no harnesses configured; skills are installed to %s only (run 'skiletto harness enable claude' to link them)\n", e.Scope.SkillsDir)
			return nil, nil
		}
	}

	return e.adaptersFor(names, true), nil
}

// machineHarnesses returns the machine manifest's harnesses key, or nil
// when there is no machine scope or the key is absent. When the engine's
// scope is the machine scope itself, the already-loaded manifest is
// authoritative — reading the file again would just race it.
func (e *Engine) machineHarnesses(m *manifest.Manifest) []string {
	if e.Machine == nil {
		return nil
	}
	if e.Machine.ManifestPath == e.Scope.ManifestPath {
		return m.Harnesses
	}
	mm, err := manifest.Load(e.Machine.ManifestPath)
	if err != nil {
		_, _ = fmt.Fprintf(e.Err, "warning: %v\n", err)
		return nil
	}
	return mm.Harnesses
}

// adaptersFor maps harness names to registered adapters, optionally
// warning about names no adapter claims.
func (e *Engine) adaptersFor(names []string, warnUnknown bool) []adapter.Adapter {
	byName := make(map[string]adapter.Adapter, len(e.Adapters))
	for _, a := range e.Adapters {
		byName[a.Name()] = a
	}
	var enabled []adapter.Adapter
	for _, n := range names {
		a, ok := byName[n]
		if !ok {
			if warnUnknown {
				_, _ = fmt.Fprintf(e.Err, "warning: unknown harness %q in configuration; ignoring\n", n)
			}
			continue
		}
		enabled = append(enabled, a)
	}
	return enabled
}

// harnessOptions describes every registered adapter for the prompt,
// detection included.
func (e *Engine) harnessOptions() []HarnessOption {
	opts := make([]HarnessOption, 0, len(e.Adapters))
	for _, a := range e.Adapters {
		detected := false
		if e.Machine != nil {
			detected = a.Detected(*e.Machine)
		}
		opts = append(opts, HarnessOption{Name: a.Name(), Detected: detected})
	}
	return opts
}

// unionNames merges two name lists, deduplicated and sorted.
func unionNames(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range append(append([]string{}, a...), b...) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// HarnessEnable adds harnesses to the scope's manifest key and links every
// installed skill into them. Unknown names fail before anything is written.
func (e *Engine) HarnessEnable(names []string, force bool) error {
	if err := e.validateHarnessNames(names); err != nil {
		return err
	}
	m, lf, err := e.load()
	if err != nil {
		return err
	}
	m.Harnesses = unionNames(m.Harnesses, names)
	if err := m.Save(e.Scope.ManifestPath); err != nil {
		return err
	}
	adapters := e.adaptersFor(names, false)
	failures := 0
	for _, locked := range lf.Skills {
		target := e.Scope.SkillDir(locked.Name)
		for _, a := range adapters {
			if err := a.Link(e.Scope, locked.Name, target, force); err != nil {
				failures++
				_, _ = fmt.Fprintf(e.Err, "error: %s: adapter %s: %v\n", locked.Name, a.Name(), err)
			}
		}
	}
	_, _ = fmt.Fprintf(e.Out, "enabled %s\n", strings.Join(names, ", "))
	if failures > 0 {
		return fmt.Errorf("%d skill(s) failed to link; see errors above", failures)
	}
	return nil
}

// HarnessDisable removes harnesses from the scope's manifest key and
// unlinks every installed skill from them. Disabling something the scope
// never enabled is an error; when the harness stays enabled machine-wide
// (union semantics) a warning says so.
func (e *Engine) HarnessDisable(names []string, force bool) error {
	if err := e.validateHarnessNames(names); err != nil {
		return err
	}
	m, lf, err := e.load()
	if err != nil {
		return err
	}
	enabled := map[string]bool{}
	for _, n := range m.Harnesses {
		enabled[n] = true
	}
	for _, n := range names {
		if !enabled[n] {
			return fmt.Errorf("harness %q is not enabled in this scope", n)
		}
		delete(enabled, n)
	}
	remaining := make([]string, 0, len(enabled))
	for n := range enabled {
		remaining = append(remaining, n)
	}
	sort.Strings(remaining)
	m.Harnesses = remaining
	if err := m.Save(e.Scope.ManifestPath); err != nil {
		return err
	}

	machine := map[string]bool{}
	if e.Machine != nil && e.Machine.ManifestPath != e.Scope.ManifestPath {
		for _, n := range e.machineHarnesses(m) {
			machine[n] = true
		}
	}
	adapters := e.adaptersFor(names, false)
	failures := 0
	for _, a := range adapters {
		if machine[a.Name()] {
			_, _ = fmt.Fprintf(e.Err, "warning: harness %q is still enabled machine-wide (%s); its links stay\n", a.Name(), e.Machine.ManifestPath)
			continue
		}
		for _, locked := range lf.Skills {
			if err := a.Unlink(e.Scope, locked.Name, force); err != nil {
				failures++
				_, _ = fmt.Fprintf(e.Err, "error: %s: adapter %s: %v\n", locked.Name, a.Name(), err)
			}
		}
	}
	_, _ = fmt.Fprintf(e.Out, "disabled %s\n", strings.Join(names, ", "))
	if failures > 0 {
		return fmt.Errorf("%d skill(s) failed to unlink; see errors above", failures)
	}
	return nil
}

// HarnessList prints every registered harness with where it is enabled,
// its link dir for the current scope, and whether it looks installed.
func (e *Engine) HarnessList() error {
	m, _, err := e.load()
	if err != nil {
		return err
	}
	project := map[string]bool{}
	for _, n := range m.Harnesses {
		project[n] = true
	}
	machine := map[string]bool{}
	machineIsScope := e.Machine != nil && e.Machine.ManifestPath == e.Scope.ManifestPath
	if e.Machine != nil && !machineIsScope {
		for _, n := range e.machineHarnesses(m) {
			machine[n] = true
		}
	}
	for _, a := range e.Adapters {
		var where []string
		if project[a.Name()] {
			label := "project"
			if machineIsScope {
				label = "machine"
			}
			where = append(where, label)
		}
		if machine[a.Name()] {
			where = append(where, "machine")
		}
		state := "disabled"
		if len(where) > 0 {
			state = "enabled (" + strings.Join(where, ", ") + ")"
		}
		detected := ""
		if e.Machine != nil && a.Detected(*e.Machine) {
			detected = "\tdetected"
		}
		_, _ = fmt.Fprintf(e.Out, "%s\t%s\t%s%s\n", a.Name(), state, a.SkillsDir(e.Scope), detected)
	}
	return nil
}

// validateHarnessNames rejects names no registered adapter claims, listing
// the registered ones.
func (e *Engine) validateHarnessNames(names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("no harness names given")
	}
	byName := map[string]bool{}
	registered := make([]string, 0, len(e.Adapters))
	for _, a := range e.Adapters {
		byName[a.Name()] = true
		registered = append(registered, a.Name())
	}
	sort.Strings(registered)
	for _, n := range names {
		if !byName[n] {
			return fmt.Errorf("unknown harness %q; registered harnesses: %s", n, strings.Join(registered, ", "))
		}
	}
	return nil
}
