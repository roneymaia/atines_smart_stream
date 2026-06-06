package launcher

import (
	"os/exec"
	"runtime"
)

// browserCommand returns the OS command to open a URL in the default browser.
func browserCommand(goos, url string) (string, []string) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		return "open", []string{url}
	default:
		return "xdg-open", []string{url}
	}
}

// OpenBrowser best-effort launches the default browser at url.
func OpenBrowser(url string) {
	name, args := browserCommand(runtime.GOOS, url)
	_ = exec.Command(name, args...).Start()
}
