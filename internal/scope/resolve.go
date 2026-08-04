package scope

import (
	"os"
	"path/filepath"
)

// Resolution describes how the machine scope was located: Source names the
// winner, and Shadowed lists manifest paths that exist but are not read
// because a higher-precedence config won.
type Resolution struct {
	Scope    Scope
	Source   string
	Shadowed []string
}

// Resolution sources, in words fit for a warning line or a doctor report.
const (
	SourceEnvOverride    = "SKILETTO_CONFIG_DIR"
	SourceXDG            = "XDG_CONFIG_HOME"
	SourceDotConfig      = "~/.config/skiletto"
	SourcePlatformConfig = "platform default"
)

// ResolveMachine locates the machine scope. Precedence, on every platform:
//
//  1. SKILETTO_CONFIG_DIR — the directory holding skiletto.toml directly,
//     no "skiletto" subdirectory appended;
//  2. XDG_CONFIG_HOME — config under $XDG_CONFIG_HOME/skiletto;
//  3. ~/.config/skiletto — when its skiletto.toml already exists (a
//     dotfiles-synced config), even on platforms whose default lives
//     elsewhere;
//  4. the platform config dir (%AppData% on Windows,
//     ~/Library/Application Support on macOS, ~/.config on Linux).
//
// A fresh install therefore lands in the platform default; the
// ~/.config/skiletto fallback only takes over once a manifest is actually
// there. Manifests that exist below the winning one are reported in
// Shadowed, newest convention first, so callers can warn about configs
// that are written to by nothing. home, getenv, and userConfigDir are
// injected so resolution stays testable and never assumes the real
// environment.
func ResolveMachine(home string, getenv func(string) string, userConfigDir func() (string, error)) (Resolution, error) {
	type candidate struct {
		source   string
		dir      string // config dir; manifest lives at dir/skiletto/skiletto.toml
		bare     bool   // dir holds skiletto.toml directly (SKILETTO_CONFIG_DIR)
		required bool   // an explicit override is used even without a manifest
	}
	var cands []candidate
	if dir := getenv("SKILETTO_CONFIG_DIR"); dir != "" {
		cands = append(cands, candidate{source: SourceEnvOverride, dir: dir, bare: true, required: true})
	}
	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" {
		cands = append(cands, candidate{source: SourceXDG, dir: xdg, required: true})
	}
	cands = append(cands, candidate{source: SourceDotConfig, dir: filepath.Join(home, ".config")})
	osDir, osErr := userConfigDir()
	if osErr == nil {
		cands = append(cands, candidate{source: SourcePlatformConfig, dir: osDir})
	}

	manifestOf := func(c candidate) string {
		if c.bare {
			return filepath.Join(c.dir, "skiletto.toml")
		}
		return filepath.Join(c.dir, "skiletto", "skiletto.toml")
	}
	var chosen *candidate
	for i := range cands {
		c := &cands[i]
		if c.required || fileExists(manifestOf(*c)) {
			chosen = c
			break
		}
	}
	if chosen == nil {
		// Nothing matched: a first write lands in the platform default,
		// which is the last candidate. When that one is unavailable its
		// error is the diagnosis.
		if osErr != nil {
			return Resolution{}, osErr
		}
		chosen = &cands[len(cands)-1]
	}

	res := Resolution{
		Scope:  machineScopeFor(home, chosen.bare, chosen.dir),
		Source: chosen.source,
	}
	chosenPath := filepath.Clean(manifestOf(*chosen))
	for i := range cands {
		c := cands[i]
		if c.source == chosen.source {
			continue // the winner itself (each source appears once)
		}
		p := filepath.Clean(manifestOf(c))
		if p == chosenPath {
			continue // Linux: the platform default IS the ~/.config fallback
		}
		if fileExists(p) {
			res.Shadowed = append(res.Shadowed, p)
		}
	}
	return res, nil
}

// machineScopeFor builds the machine Scope for a resolved config location:
// bare directories hold the manifest directly, every other candidate gets
// a "skiletto" subdirectory.
func machineScopeFor(home string, bare bool, dir string) Scope {
	if bare {
		s := Machine(home, "")
		s.ManifestPath = filepath.Join(dir, "skiletto.toml")
		s.LockPath = filepath.Join(dir, "skiletto.lock")
		return s
	}
	return Machine(home, dir)
}

// fileExists reports whether path names an existing regular file.
func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}
