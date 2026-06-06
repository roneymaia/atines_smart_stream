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

// parseRTSPTimeoutOpt picks the socket-timeout flag from the rtsp demuxer help.
// ffmpeg 4.x exposes "stimeout" (microseconds); 5.x+ removed it and uses
// "timeout" for the socket timeout instead.
func parseRTSPTimeoutOpt(rtspHelp string) string {
	if strings.Contains(rtspHelp, "stimeout") {
		return "-stimeout"
	}
	return "-timeout"
}

func detectRTSPTimeoutOpt(ctx context.Context, ffmpegPath string) string {
	out, err := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-h", "demuxer=rtsp").CombinedOutput()
	if err != nil {
		return "-timeout"
	}
	return parseRTSPTimeoutOpt(string(out))
}

// DetectCapabilities inspects the bundled ffmpeg for hardware H.264 encoders and
// the correct RTSP timeout flag. Encoder-detection errors degrade to
// software-only; the timeout flag always gets a sane default.
func DetectCapabilities(ctx context.Context, ffmpegPath string) Capabilities {
	caps := Capabilities{}
	if out, err := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-encoders").CombinedOutput(); err == nil {
		caps = parseEncoders(string(out))
	}
	caps.RTSPTimeoutOpt = detectRTSPTimeoutOpt(ctx, ffmpegPath)
	return caps
}
