package media

import (
	"reflect"
	"testing"

	"atines_smart_stream/internal/model"
)

func cam(mode model.TranscodeMode, accel model.AccelMode) model.Camera {
	return model.Camera{
		RTSPURL:       "rtsp://host:554/stream",
		Username:      "user",
		Password:      "pass",
		RTMPURL:       "rtmp://srs/live/cam1",
		TranscodeMode: mode,
		AccelMode:     accel,
	}
}

func TestBuildInputURL(t *testing.T) {
	got, err := BuildInputURL(cam(model.TranscodeAuto, model.AccelAuto))
	if err != nil {
		t.Fatal(err)
	}
	want := "rtsp://user:pass@host:554/stream"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildArgs_H264Remux(t *testing.T) {
	got, err := BuildArgs(cam(model.TranscodeAuto, model.AccelAuto),
		ProbeResult{VideoCodec: "h264", AudioCodec: "aac", HasAudio: true}, Capabilities{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-rtsp_transport", "tcp", "-timeout", "5000000",
		"-i", "rtsp://user:pass@host:554/stream",
		"-c:v", "copy",
		"-c:a", "copy",
		"-f", "flv", "rtmp://srs/live/cam1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("\n got=%v\nwant=%v", got, want)
	}
}

func TestBuildArgs_MJPEGTranscodeSoftware(t *testing.T) {
	got, _ := BuildArgs(cam(model.TranscodeAuto, model.AccelAuto),
		ProbeResult{VideoCodec: "mjpeg", AudioCodec: "pcm_mulaw", HasAudio: true}, Capabilities{})
	want := []string{
		"-rtsp_transport", "tcp", "-timeout", "5000000",
		"-i", "rtsp://user:pass@host:554/stream",
		"-c:v", "libx264", "-preset", "veryfast", "-tune", "zerolatency", "-g", "50",
		"-c:a", "aac", "-b:a", "128k",
		"-f", "flv", "rtmp://srs/live/cam1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("\n got=%v\nwant=%v", got, want)
	}
}

func TestBuildArgs_HEVCAutoTranscodesToNVENC(t *testing.T) {
	got, _ := BuildArgs(cam(model.TranscodeAuto, model.AccelAuto),
		ProbeResult{VideoCodec: "hevc", HasAudio: false}, Capabilities{NVENC: true})
	want := []string{
		"-rtsp_transport", "tcp", "-timeout", "5000000",
		"-i", "rtsp://user:pass@host:554/stream",
		"-c:v", "h264_nvenc", "-preset", "p4", "-tune", "ll", "-g", "50",
		"-an",
		"-f", "flv", "rtmp://srs/live/cam1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("\n got=%v\nwant=%v", got, want)
	}
}

func TestBuildArgs_KeepHEVCCopiesHevc(t *testing.T) {
	got, _ := BuildArgs(cam(model.TranscodeKeepHEVC, model.AccelAuto),
		ProbeResult{VideoCodec: "hevc", AudioCodec: "aac", HasAudio: true}, Capabilities{})
	// hevc kept as copy
	if got[6] != "-c:v" || got[7] != "copy" {
		t.Fatalf("expected hevc copy, got %v", got)
	}
}

func TestBuildArgs_VAAPIAddsDeviceAndFilter(t *testing.T) {
	got, _ := BuildArgs(cam(model.TranscodeForce, model.AccelHardware),
		ProbeResult{VideoCodec: "h264", HasAudio: false}, Capabilities{VAAPI: true})
	want := []string{
		"-vaapi_device", "/dev/dri/renderD128",
		"-rtsp_transport", "tcp", "-timeout", "5000000",
		"-i", "rtsp://user:pass@host:554/stream",
		"-vf", "format=nv12,hwupload", "-c:v", "h264_vaapi", "-g", "50",
		"-an",
		"-f", "flv", "rtmp://srs/live/cam1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("\n got=%v\nwant=%v", got, want)
	}
}

func TestBuildArgs_UsesDetectedRTSPTimeoutFlag(t *testing.T) {
	// ffmpeg 4.x: socket timeout flag is -stimeout.
	got, _ := BuildArgs(cam(model.TranscodeCopy, model.AccelAuto),
		ProbeResult{VideoCodec: "h264", HasAudio: false},
		Capabilities{RTSPTimeoutOpt: "-stimeout"})
	want := []string{
		"-rtsp_transport", "tcp", "-stimeout", "5000000",
		"-i", "rtsp://user:pass@host:554/stream",
		"-c:v", "copy",
		"-an",
		"-f", "flv", "rtmp://srs/live/cam1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("\n got=%v\nwant=%v", got, want)
	}
}

func TestBuildArgs_SoftwareModeIgnoresHardware(t *testing.T) {
	got, _ := BuildArgs(cam(model.TranscodeForce, model.AccelSoftware),
		ProbeResult{VideoCodec: "h264", HasAudio: false}, Capabilities{NVENC: true})
	if got[6] != "-c:v" || got[7] != "libx264" {
		t.Fatalf("expected libx264, got %v", got)
	}
}
