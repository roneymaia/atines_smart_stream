package media

import (
	"context"
	"os/exec"
	"strings"
)

func parseEncoders(out string) Capabilities {
	return Capabilities{
		NVENC: strings.Contains(out, "h264_nvenc"),
		QSV:   strings.Contains(out, "h264_qsv"),
		AMF:   strings.Contains(out, "h264_amf"),
		VAAPI: strings.Contains(out, "h264_vaapi"),
	}
}

// DetectCapabilities inspects the bundled ffmpeg for hardware H.264 encoders.
// On any error it returns an empty Capabilities (software-only).
func DetectCapabilities(ctx context.Context, ffmpegPath string) Capabilities {
	cmd := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-encoders")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Capabilities{}
	}
	return parseEncoders(string(out))
}
