// Package manifest reads and writes skiletto.toml, the hand-editable file
// recording which skills the user wants installed.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// Entry describes one requested skill.
type Entry struct {
	Source   string `toml:"source"`
	Path     string `toml:"path,omitempty"`
	Ref      string `toml:"ref,omitempty"`
	Editable bool   `toml:"editable,omitempty"`
}

// Manifest is the parsed skiletto.toml.
type Manifest struct {
	Skills map[string]Entry `toml:"skills"`
}

// Load reads the manifest at path. A missing file yields an empty manifest.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Manifest{Skills: map[string]Entry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m.Skills == nil {
		m.Skills = map[string]Entry{}
	}
	return &m, nil
}

// Save writes the manifest to path as a [skills] table with one inline
// entry per skill, sorted by name.
func (m *Manifest) Save(path string) error {
	var b strings.Builder
	b.WriteString("[skills]\n")
	names := make([]string, 0, len(m.Skills))
	for name := range m.Skills {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		e := m.Skills[name]
		fields := []string{fmt.Sprintf("source = %s", tomlString(e.Source))}
		if e.Path != "" {
			fields = append(fields, fmt.Sprintf("path = %s", tomlString(e.Path)))
		}
		if e.Ref != "" {
			fields = append(fields, fmt.Sprintf("ref = %s", tomlString(e.Ref)))
		}
		if e.Editable {
			fields = append(fields, "editable = true")
		}
		fmt.Fprintf(&b, "%s = { %s }\n", tomlKey(name), strings.Join(fields, ", "))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// tomlString renders s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// tomlKey renders a table key, quoting it unless it is a bare key.
func tomlKey(s string) string {
	for _, r := range s {
		bare := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if !bare {
			return tomlString(s)
		}
	}
	if s == "" {
		return `""`
	}
	return s
}
