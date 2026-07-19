package engine

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/gitcli"
	"github.com/kumekay/skiletto/internal/manifest"
	"github.com/kumekay/skiletto/internal/source"
)

// failingResolveSource fails every Resolve with a fixed error.
type failingResolveSource struct {
	fakeSource
	err error
}

func (f *failingResolveSource) Resolve(string) (string, error) { return "", f.err }

// A ref-not-found failure on a spec parsed from a pasted /tree/ URL gets a
// hint suggesting the explicit repo//path@ref form: the single-segment ref
// guess cannot handle refs containing "/". The hint is applied in the
// engine, at the resolve that fails, so every add flavor gets it.
func TestAddTreeURLRefNotFoundHint(t *testing.T) {
	cause := fmt.Errorf("%w: %q at %s", gitcli.ErrRefNotFound, "feature", "https://github.com/o/r")
	f := newFixture(t, &fakeSource{})
	f.eng.NewSource = func(string) (source.Source, error) {
		return &failingResolveSource{err: cause}, nil
	}
	spec := manifest.SourceSpec{Source: "https://github.com/o/r", Path: "foo/skills/pdf", Ref: "feature", TreeURL: true}

	err := f.eng.Add(spec, false)
	if err == nil {
		t.Fatal("want a resolve error")
	}
	for _, want := range []string{"https://github.com/o/r//", "@<ref>"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("hint missing %q:\n%s", want, err)
		}
	}
	if !errors.Is(err, gitcli.ErrRefNotFound) {
		t.Error("hint must preserve the error chain")
	}

	// The discovery-driven flavors resolve too and must carry the hint.
	if err := f.eng.AddAll(spec, false); err == nil || !strings.Contains(err.Error(), "@<ref>") {
		t.Errorf("AddAll error missing hint: %v", err)
	}
	if err := f.eng.AddSkills(spec, []string{"pdf"}, false); err == nil || !strings.Contains(err.Error(), "@<ref>") {
		t.Errorf("AddSkills error missing hint: %v", err)
	}
}

// Without TreeURL, or for non-ref failures, resolve errors pass through
// untouched.
func TestAddResolveErrorsUnhinted(t *testing.T) {
	cause := fmt.Errorf("%w: %q at %s", gitcli.ErrRefNotFound, "nope", "https://github.com/o/r")
	f := newFixture(t, &fakeSource{})
	f.eng.NewSource = func(string) (source.Source, error) {
		return &failingResolveSource{err: cause}, nil
	}

	err := f.eng.Add(manifest.SourceSpec{Source: "https://github.com/o/r", Ref: "nope"}, false)
	if err == nil || strings.Contains(err.Error(), "@<ref>") {
		t.Errorf("plain spec must not get the /tree/ hint: %v", err)
	}
}
