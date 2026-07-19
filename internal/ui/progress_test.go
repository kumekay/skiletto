package ui

import (
	"bytes"
	"fmt"
	"testing"
)

func TestProgressStepsReplaceEachOtherAndDonePersists(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf)
	p.Step("pdf", "resolving")
	p.Step("pdf", "fetching")
	p.Done("pdf", "installed")
	want := "pdf: resolving…\r\x1b[Kpdf: fetching…\r\x1b[Kpdf: installed\n"
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestProgressDoneWithoutStepWritesPlainLine(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf)
	p.Done("pdf", "installed")
	if got, want := buf.String(), "pdf: installed\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestProgressClearErasesPendingLine(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf)
	p.Step("pdf", "resolving")
	p.Clear()
	if got, want := buf.String(), "pdf: resolving…\r\x1b[K"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	// A second Clear with nothing pending writes nothing.
	p.Clear()
	if got, want := buf.String(), "pdf: resolving…\r\x1b[K"; got != want {
		t.Errorf("output after idempotent clear = %q, want %q", got, want)
	}
}

func TestProgressWriterClearsPendingLineBeforeWriting(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf)
	w := p.Writer(&buf)
	p.Step("pdf", "fetching")
	_, _ = fmt.Fprintln(w, "error: boom")
	want := "pdf: fetching…\r\x1b[Kerror: boom\n"
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestProgressWriterWithoutPendingWritesThrough(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf)
	w := p.Writer(&buf)
	_, _ = fmt.Fprintln(w, "plain")
	if got, want := buf.String(), "plain\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
