// Package adapter defines the harness Adapter extension point and a
// compiled-in registry. Adapters know where a harness expects skills for a
// scope and how to link individual skill directories there. The shared
// link helper owns the link mechanism (symlinks today; the seam for a
// future Windows junction/copy fallback).
package adapter

import (
	"fmt"
	"sort"

	"github.com/kumekay/skiletto/internal/scope"
)

// Adapter integrates one harness.
type Adapter interface {
	// Name identifies the adapter (e.g. "claude").
	Name() string
	// SkillsDir is where the harness looks for skills in the given scope.
	SkillsDir(s scope.Scope) string
	// Link makes the skill at target visible to the harness under name.
	// force additionally replaces a copy-linked install that has diverged
	// from its canonical tree (a local modification).
	Link(s scope.Scope, name, target string, force bool) error
	// Unlink removes the harness link for name. Missing links are no-ops.
	// force additionally removes a diverged copy-linked install.
	Unlink(s scope.Scope, name string, force bool) error
}

var registry = map[string]Adapter{}

// Register adds an adapter to the compiled-in registry.
func Register(a Adapter) {
	registry[a.Name()] = a
}

// All returns the registered adapters sorted by name.
func All() []Adapter {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	adapters := make([]Adapter, 0, len(names))
	for _, name := range names {
		adapters = append(adapters, registry[name])
	}
	return adapters
}

// NotASymlinkError reports a link location occupied by something other
// than a symlink (e.g. a real skill directory installed by another tool),
// which skiletto never replaces. Hint carries platform guidance (Windows
// copy links) and is empty on unix, keeping the message unchanged there.
type NotASymlinkError struct {
	Path string
	Hint string
}

func (e *NotASymlinkError) Error() string {
	return fmt.Sprintf("%s exists and is not a symlink; refusing to replace it%s", e.Path, e.Hint)
}

// NotOurLinkError reports a harness link location occupied by something
// skiletto cannot prove it created — a foreign directory, or on Windows a
// copy-linked install that diverged — which it refuses to remove.
type NotOurLinkError struct {
	Path string
	Hint string
}

func (e *NotOurLinkError) Error() string {
	return fmt.Sprintf("%s is not a skiletto link; refusing to remove it%s", e.Path, e.Hint)
}
