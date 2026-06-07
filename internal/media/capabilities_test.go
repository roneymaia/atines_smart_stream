package media

import (
	"strings"
	"testing"
)

const encodersSample = `
 Encoders:
 V..... libx264              libx264 H.264 / AVC
 V..... h264_nvenc           NVIDIA NVENC H.264 encoder
 V..... h264_vaapi           H.264/AVC (VAAPI)
`

func TestVerifyEncoders_DropsNonWorking(t *testing.T) {
	in := Capabilities{NVENC: true, QSV: true, AMF: false, VAAPI: true}
	// Only QSV actually works at runtime.
	works := func(enc string) bool { return enc == "h264_qsv" }
	got := verifyEncoders(in, works)
	if got.NVENC {
		t.Error("expected NVENC dropped (smoke test failed)")
	}
	if got.VAAPI {
		t.Error("expected VAAPI dropped (smoke test failed)")
	}
	if !got.QSV {
		t.Error("expected QSV kept (smoke test passed)")
	}
}

func TestVerifyEncoders_KeepsAllWorking(t *testing.T) {
	in := Capabilities{NVENC: true}
	got := verifyEncoders(in, func(string) bool { return true })
	if !got.NVENC {
		t.Error("expected NVENC kept")
	}
}

func TestSmokeArgs_VAAPIIncludesDevice(t *testing.T) {
	args := smokeArgs("h264_vaapi")
	joined := ""
	for _, a := range args {
		joined += a + " "
	}
	if !contains(joined, "-vaapi_device") || !contains(joined, "hwupload") {
		t.Fatalf("vaapi smoke args missing device/filter: %v", args)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

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
