package launcher

import "testing"

func TestBrowserCommand(t *testing.T) {
	name, args := browserCommand("linux", "http://127.0.0.1:8787")
	if name != "xdg-open" || len(args) != 1 || args[0] != "http://127.0.0.1:8787" {
		t.Fatalf("linux: %q %v", name, args)
	}
	if name, _ := browserCommand("windows", "http://x"); name != "rundll32" {
		t.Fatalf("windows: %q", name)
	}
	if name, _ := browserCommand("darwin", "http://x"); name != "open" {
		t.Fatalf("darwin: %q", name)
	}
}
