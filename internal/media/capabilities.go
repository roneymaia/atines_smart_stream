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

// smokeArgs builds an ffmpeg command that encodes a single black frame with the
// given encoder and discards it. Exit code 0 means the encoder works at runtime.
func smokeArgs(encoder string) []string {
	if encoder == "h264_vaapi" {
		return []string{
			"-hide_banner", "-loglevel", "error",
			"-vaapi_device", "/dev/dri/renderD128",
			"-f", "lavfi", "-i", "color=c=black:s=256x256:d=1", "-frames:v", "1",
			"-vf", "format=nv12,hwupload", "-c:v", "h264_vaapi", "-f", "null", "-",
		}
	}
	return []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=256x256:d=1", "-frames:v", "1",
		"-c:v", encoder, "-f", "null", "-",
	}
}

func encoderWorks(ctx context.Context, ffmpegPath, encoder string) bool {
	return exec.CommandContext(ctx, ffmpegPath, smokeArgs(encoder)...).Run() == nil
}

// verifyEncoders drops any compiled-in encoder that fails its smoke test, so the
// "auto" accel mode never selects a hardware encoder the host can't actually run.
func verifyEncoders(caps Capabilities, works func(encoder string) bool) Capabilities {
	if caps.NVENC && !works("h264_nvenc") {
		caps.NVENC = false
	}
	if caps.QSV && !works("h264_qsv") {
		caps.QSV = false
	}
	if caps.AMF && !works("h264_amf") {
		caps.AMF = false
	}
	if caps.VAAPI && !works("h264_vaapi") {
		caps.VAAPI = false
	}
	return caps
}

// DetectCapabilities inspects the bundled ffmpeg for hardware H.264 encoders and
// the correct RTSP timeout flag. Compiled-in encoders are smoke-tested so only
// the ones that actually run are reported. Encoder-detection errors degrade to
// software-only; the timeout flag always gets a sane default.
func DetectCapabilities(ctx context.Context, ffmpegPath string) Capabilities {
	caps := Capabilities{}
	if out, err := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-encoders").CombinedOutput(); err == nil {
		caps = parseEncoders(string(out))
	}
	caps = verifyEncoders(caps, func(enc string) bool { return encoderWorks(ctx, ffmpegPath, enc) })
	caps.RTSPTimeoutOpt = detectRTSPTimeoutOpt(ctx, ffmpegPath)
	return caps
}
