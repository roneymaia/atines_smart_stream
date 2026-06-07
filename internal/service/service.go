package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const (
	unitName = "atines-smart-stream.service"
	taskName = "AtinesSmartStream" // Windows Scheduled Task name
)

// Install registers the app to start automatically on boot.
func Install() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "linux":
		return installSystemd(exe)
	case "windows":
		return installWindowsTask(exe)
	default:
		return fmt.Errorf("instalação de serviço não suportada em %s", runtime.GOOS)
	}
}

// Uninstall removes the auto-start registration.
func Uninstall() error {
	switch runtime.GOOS {
	case "linux":
		_ = run("systemctl", "--user", "disable", "--now", unitName)
		return os.Remove(systemdUnitPath())
	case "windows":
		_ = run("schtasks", "/end", "/tn", taskName)
		return run("schtasks", "/delete", "/tn", taskName, "/f")
	default:
		return fmt.Errorf("não suportado em %s", runtime.GOOS)
	}
}

// installWindowsTask registers a Scheduled Task that launches the app at system
// boot (as SYSTEM). A plain exe like this one is NOT a true Windows service
// (it doesn't implement the SCM control protocol), so `sc create` would fail
// with error 1053 at start; a Scheduled Task runs a normal exe reliably.
// Requires an elevated (Administrator) prompt.
func installWindowsTask(exe string) error {
	// Single /tr value: the quoted exe path plus the headless flag.
	tr := `"` + exe + `" --no-browser`
	if err := run("schtasks", "/create",
		"/tn", taskName,
		"/tr", tr,
		"/sc", "onstart",
		"/ru", "SYSTEM",
		"/rl", "HIGHEST",
		"/f",
	); err != nil {
		return err
	}
	// Start it now too, so the user doesn't have to reboot. Don't fail the
	// install if the immediate start hiccups.
	_ = run("schtasks", "/run", "/tn", taskName)
	return nil
}

func installSystemd(exe string) error {
	unit := fmt.Sprintf(`[Unit]
Description=Atines Smart Stream
After=network-online.target

[Service]
ExecStart=%s --no-browser
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
`, exe)
	path := systemdUnitPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return err
	}
	if err := run("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	return run("systemctl", "--user", "enable", "--now", unitName)
}

func systemdUnitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", unitName)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
