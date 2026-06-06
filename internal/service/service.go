package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const unitName = "atines-smart-stream.service"

// Install registers the app as an OS service that starts on boot.
func Install() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "linux":
		return installSystemd(exe)
	case "windows":
		return run("sc", "create", "AtinesSmartStream", "binPath=", exe+" --no-browser", "start=", "auto")
	default:
		return fmt.Errorf("instalação de serviço não suportada em %s", runtime.GOOS)
	}
}

// Uninstall removes the OS service.
func Uninstall() error {
	switch runtime.GOOS {
	case "linux":
		_ = run("systemctl", "--user", "disable", "--now", unitName)
		return os.Remove(systemdUnitPath())
	case "windows":
		return run("sc", "delete", "AtinesSmartStream")
	default:
		return fmt.Errorf("não suportado em %s", runtime.GOOS)
	}
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
