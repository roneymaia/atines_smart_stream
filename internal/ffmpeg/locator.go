package ffmpeg

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Locate returns paths to ffmpeg and ffprobe, preferring binaries next to the
// running executable, falling back to PATH.
func Locate() (ffmpegPath, ffprobePath string, err error) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	dir := filepath.Dir(exe)
	ffmpegPath, err = locateIn(dir, "ffmpeg", exec.LookPath)
	if err != nil {
		return "", "", err
	}
	ffprobePath, err = locateIn(dir, "ffprobe", exec.LookPath)
	if err != nil {
		return "", "", err
	}
	return ffmpegPath, ffprobePath, nil
}

// locateIn checks dir for the binary, then falls back to lookPath.
func locateIn(dir, base string, lookPath func(string) (string, error)) (string, error) {
	name := base
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	local := filepath.Join(dir, name)
	if st, err := os.Stat(local); err == nil && !st.IsDir() {
		return local, nil
	}
	if p, err := lookPath(base); err == nil {
		return p, nil
	}
	return "", errors.New(base + " não encontrado (nem ao lado do executável nem no PATH)")
}
