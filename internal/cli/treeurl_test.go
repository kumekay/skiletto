package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kumekay/skiletto/internal/gitcli"
	"github.com/kumekay/skiletto/internal/manifest"
)

// A ref-not-found failure on a spec parsed from a /tree/ URL gets a hint:
// the single-segment ref guess cannot handle refs containing "/", so the
// explicit repo//path@ref form is suggested.
func TestTreeURLHintOnRefNotFound(t *testing.T) {
	spec := manifest.SourceSpec{
		Source:  "https://github.com/o/r",
		Path:    "foo/skills/pdf",
		Ref:     "feature",
		TreeURL: true,
	}
	cause := fmt.Errorf("resolve: %w", gitcli.ErrRefNotFound)
	err := treeURLHint(spec, cause)
	if err == nil {
		t.Fatal("want a non-nil error")
	}
	msg := err.Error()
	for _, want := range []string{"resolve:", "https://github.com/o/r//", "@<ref>"} {
		if !strings.Contains(msg, want) {
			t.Errorf("hint missing %q:\n%s", want, msg)
		}
	}
}

func TestTreeURLHintLeavesOtherErrorsAlone(t *testing.T) {
	cause := fmt.Errorf("resolve: %w", gitcli.ErrRefNotFound)
	if got := treeURLHint(manifest.SourceSpec{Source: "https://github.com/o/r"}, cause); got != cause {
		t.Errorf("non-tree spec: error changed to %v", got)
	}
	other := fmt.Errorf("network down")
	if got := treeURLHint(manifest.SourceSpec{TreeURL: true}, other); got != other {
		t.Errorf("non-ref error: error changed to %v", got)
	}
	if got := treeURLHint(manifest.SourceSpec{TreeURL: true}, nil); got != nil {
		t.Errorf("nil error: got %v", got)
	}
}
