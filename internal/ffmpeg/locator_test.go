package ffmpeg

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocateInPrefersLocalDir(t *testing.T) {
	dir := t.TempDir()
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	local := filepath.Join(dir, name)
	if err := os.WriteFile(local, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	lookPath := func(string) (string, error) { return "/usr/bin/ffmpeg", nil }
	got, err := locateIn(dir, "ffmpeg", lookPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != local {
		t.Fatalf("got %q want %q", got, local)
	}
}

func TestLocateInFallsBackToPath(t *testing.T) {
	dir := t.TempDir()
	lookPath := func(string) (string, error) { return "/usr/bin/ffmpeg", nil }
	got, err := locateIn(dir, "ffmpeg", lookPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/usr/bin/ffmpeg" {
		t.Fatalf("got %q", got)
	}
}
