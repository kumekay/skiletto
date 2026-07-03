// Package adapter defines the harness Adapter extension point and a
// compiled-in registry. Adapters know where a harness expects skills for a
// scope and how to link individual skill directories there. The shared
// link helper owns the link mechanism (symlinks today; the seam for a
// future Windows junction/copy fallback).
package adapter

import (
	"fmt"
	"os"
	"path/filepath"
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
	Link(s scope.Scope, name, target string) error
	// Unlink removes the harness link for name. Missing links are no-ops.
	Unlink(s scope.Scope, name string) error
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

// Symlink creates (or replaces) a symlink at link pointing to target,
// creating parent directories as needed. It refuses to replace anything
// that is not a symlink.
func Symlink(link, target string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	if fi, err := os.Lstat(link); err == nil {
		if fi.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s exists and is not a symlink; refusing to replace it", link)
		}
		if err := os.Remove(link); err != nil {
			return err
		}
	}
	return os.Symlink(target, link)
}

// RemoveLink removes the symlink at link. A missing link is a no-op;
// anything that is not a symlink is left alone with an error.
func RemoveLink(link string) error {
	fi, err := os.Lstat(link)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s is not a symlink; refusing to remove it", link)
	}
	return os.Remove(link)
}
