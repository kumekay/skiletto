package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnableVTOnRegularFile(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "not-a-console"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	got := EnableVT(f)
	// Unix terminals interpret VT escapes natively, so EnableVT always
	// reports true. On Windows a regular file has no console mode to
	// enable, so it must report false.
	want := runtime.GOOS != "windows"
	if got != want {
		t.Errorf("EnableVT(regular file) = %v, want %v on %s", got, want, runtime.GOOS)
	}
}
