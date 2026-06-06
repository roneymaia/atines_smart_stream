package media

import "testing"

const encodersSample = `
 Encoders:
 V..... libx264              libx264 H.264 / AVC
 V..... h264_nvenc           NVIDIA NVENC H.264 encoder
 V..... h264_vaapi           H.264/AVC (VAAPI)
`

func TestParseRTSPTimeoutOpt(t *testing.T) {
	// ffmpeg 4.x help mentions stimeout.
	help4x := "  -stimeout  <int>  set timeout (in microseconds) of socket TCP I/O operations\n  -timeout <int> ... listen ..."
	if got := parseRTSPTimeoutOpt(help4x); got != "-stimeout" {
		t.Fatalf("4.x: got %q want -stimeout", got)
	}
	// ffmpeg 5.x+ help has no stimeout, only timeout.
	help5x := "  -timeout  <int>  set timeout (in microseconds) of socket TCP I/O operations"
	if got := parseRTSPTimeoutOpt(help5x); got != "-timeout" {
		t.Fatalf("5.x: got %q want -timeout", got)
	}
}

func TestParseEncoders(t *testing.T) {
	caps := parseEncoders(encodersSample)
	if !caps.NVENC {
		t.Error("expected NVENC")
	}
	if !caps.VAAPI {
		t.Error("expected VAAPI")
	}
	if caps.QSV {
		t.Error("did not expect QSV")
	}
	if caps.AMF {
		t.Error("did not expect AMF")
	}
}
