// Package lockfile reads and writes skiletto.lock, the generated file
// pinning every installed skill to an exact commit and content hash.
package lockfile

import (
	"fmt"
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

// Skill is one locked skill entry.
type Skill struct {
	Name     string `toml:"name"`
	Source   string `toml:"source"`
	Path     string `toml:"path,omitempty"`
	Ref      string `toml:"ref,omitempty"`
	Commit   string `toml:"commit,omitempty"`
	Hash     string `toml:"hash,omitempty"`
	Editable bool   `toml:"editable,omitempty"`
}

// Lockfile is the parsed skiletto.lock.
type Lockfile struct {
	Version int     `toml:"version"`
	Skills  []Skill `toml:"skill,omitempty"`
}

// Load reads the lockfile at path. A missing file yields an empty version-1
// lockfile.
func Load(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Lockfile{Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	var lf Lockfile
	if err := toml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if lf.Version != 1 {
		return nil, fmt.Errorf("%s: unsupported lockfile version %d", path, lf.Version)
	}
	return &lf, nil
}

// Save writes the lockfile to path.
func (lf *Lockfile) Save(path string) error {
	data, err := toml.Marshal(lf)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Find returns the entry named name, or nil.
func (lf *Lockfile) Find(name string) *Skill {
	for i := range lf.Skills {
		if lf.Skills[i].Name == name {
			return &lf.Skills[i]
		}
	}
	return nil
}

// Upsert replaces the entry with the same name, or appends it.
func (lf *Lockfile) Upsert(s Skill) {
	if existing := lf.Find(s.Name); existing != nil {
		*existing = s
		return
	}
	lf.Skills = append(lf.Skills, s)
}

// Remove deletes the entry named name, if present.
func (lf *Lockfile) Remove(name string) {
	for i := range lf.Skills {
		if lf.Skills[i].Name == name {
			lf.Skills = append(lf.Skills[:i], lf.Skills[i+1:]...)
			return
		}
	}
}
