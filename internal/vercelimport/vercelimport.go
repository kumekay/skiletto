// Package vercelimport reads Vercel's skills-lock.json and maps its entries
// to canonical git sources. It is deliberately isolated: skiletto's only
// interop with `npx skills` is this one-way bootstrap. Vercel's lock records
// just a normalized source and a content hash, with no git ref and no commit
// SHA, so mapping produces sources whose default-branch HEAD skiletto then
// resolves and pins itself.
package vercelimport

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Lock is the subset of skills-lock.json that import reads.
type Lock struct {
	Version int              `json:"version"`
	Skills  map[string]Entry `json:"skills"`
}

// Entry is one skill recorded by `npx skills`. Only the fields import needs
// are modeled; content hashes and timestamps are ignored.
type Entry struct {
	Source     string `json:"source"`
	SourceType string `json:"sourceType"`
	SkillPath  string `json:"skillPath"`
	SourceURL  string `json:"sourceUrl"`
	Ref        string `json:"ref"`
}

// Mapped is a lock entry successfully mapped to a canonical git source.
type Mapped struct {
	Name   string
	Source string // canonical source string (git URL or path)
	Path   string // subdirectory of the skill within the source
	Ref    string
}

// Failure is a lock entry that could not be mapped to a git source.
type Failure struct {
	Name   string
	Reason string
}

// Read loads skills-lock.json from path. A missing file yields an actionable
// error rather than an empty import.
func Read(path string) (*Lock, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%s not found; run import in a directory that has a Vercel skills-lock.json, or pass its path: skiletto import <path>", path)
	}
	if err != nil {
		return nil, err
	}
	var lk Lock
	if err := json.Unmarshal(data, &lk); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// Only the current version 3 is accepted: Vercel wipes any lock with an
	// older version and starts fresh, so older files are extinct on disk and
	// their skillPath semantics are unverifiable.
	if lk.Version != 3 {
		what := fmt.Sprintf("unsupported skills-lock.json version %d", lk.Version)
		if lk.Version == 0 {
			what = "missing or unsupported skills-lock.json version"
		}
		return nil, fmt.Errorf("%s: %s (skiletto import understands version 3)", path, what)
	}
	return &lk, nil
}

// Map converts every lock entry to a canonical git source, sorted by skill
// name for deterministic output. Entries whose sourceType skiletto cannot
// turn into a reproducible git source are returned as failures with a
// reason instead of aborting the whole import.
func (lk *Lock) Map() (mapped []Mapped, failures []Failure) {
	names := make([]string, 0, len(lk.Skills))
	for name := range lk.Skills {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		e := lk.Skills[name]
		// The lock records skillPath as the SKILL.md file, not its directory;
		// strip the trailing SKILL.md component to recover the skill
		// subdirectory.
		path := stripSkillMd(e.SkillPath)
		switch e.SourceType {
		case "github":
			url, err := githubURL(e.Source)
			if err != nil {
				failures = append(failures, Failure{Name: name, Reason: err.Error()})
				continue
			}
			mapped = append(mapped, Mapped{Name: name, Source: url, Path: path, Ref: e.Ref})
		case "git":
			src := e.Source
			if src == "" {
				src = e.SourceURL
			}
			if src == "" {
				failures = append(failures, Failure{Name: name, Reason: "git entry has no source URL"})
				continue
			}
			mapped = append(mapped, Mapped{Name: name, Source: src, Path: path, Ref: e.Ref})
		case "local":
			failures = append(failures, Failure{
				Name:   name,
				Reason: fmt.Sprintf("local skill cannot be imported from a git source; add it directly with 'skiletto add --editable %s' for live edits or 'skiletto add %s' for a pinned copy", e.Source, e.Source),
			})
		case "":
			failures = append(failures, Failure{Name: name, Reason: "entry has no sourceType"})
		default:
			failures = append(failures, Failure{
				Name:   name,
				Reason: fmt.Sprintf("sourceType %q cannot be mapped to a git source (import supports github and git)", e.SourceType),
			})
		}
	}
	return mapped, failures
}

// stripSkillMd turns a skillPath (which points at the SKILL.md file) into
// the skill's subdirectory. A repo-root skill's "SKILL.md" strips to ".",
// which pins the source root itself: the lock named the root skill
// unambiguously, and an empty path would instead re-discover every skill
// in the repo.
func stripSkillMd(p string) string {
	if p == "SKILL.md" {
		return "."
	}
	return strings.TrimSuffix(p, "/SKILL.md")
}

// githubURL turns a Vercel "owner/repo" github source into a full clone URL.
func githubURL(source string) (string, error) {
	s := strings.TrimSuffix(strings.Trim(source, "/"), ".git")
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("github source %q is not in owner/repo form", source)
	}
	return "https://github.com/" + s, nil
}
