package ui

import (
	"fmt"
	"io"
)

// Progress renders per-skill status lines on a terminal during long
// installs. Transient steps ("pdf: resolving…") are replaced in place;
// Done prints a persistent line. It renders only when the CLI decided the
// terminal allows it — the engine sees it behind a nil-able interface.
type Progress struct {
	w       io.Writer
	pending bool
}

// NewProgress returns a Progress rendering to w (stderr in production).
func NewProgress(w io.Writer) *Progress {
	return &Progress{w: w}
}

// Step shows a transient "name: stage…" line, replacing any pending one.
func (p *Progress) Step(name, stage string) {
	p.clear()
	_, _ = fmt.Fprintf(p.w, "%s: %s…", name, stage)
	p.pending = true
}

// Done replaces any pending line with a persistent "name: result" line.
func (p *Progress) Done(name, result string) {
	p.clear()
	_, _ = fmt.Fprintf(p.w, "%s: %s\n", name, result)
}

// Clear erases the pending transient line, if any.
func (p *Progress) Clear() {
	p.clear()
}

func (p *Progress) clear() {
	if p.pending {
		_, _ = fmt.Fprint(p.w, "\r\x1b[K")
		p.pending = false
	}
}

// Writer wraps w so that any write first erases the pending transient
// line: interleaved output (errors, hook output) never lands mid-line.
func (p *Progress) Writer(w io.Writer) io.Writer {
	return progressWriter{p: p, w: w}
}

type progressWriter struct {
	p *Progress
	w io.Writer
}

func (pw progressWriter) Write(b []byte) (int, error) {
	pw.p.clear()
	return pw.w.Write(b)
}
