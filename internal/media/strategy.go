package media

import (
	"net/url"

	"atines_smart_stream/internal/model"
)

// BuildInputURL injects credentials into the RTSP URL.
func BuildInputURL(cam model.Camera) (string, error) {
	u, err := url.Parse(cam.RTSPURL)
	if err != nil {
		return "", err
	}
	if cam.Username != "" {
		u.User = url.UserPassword(cam.Username, cam.Password)
	}
	return u.String(), nil
}

// BuildArgs returns the full ffmpeg argument list (excluding the binary path).
func BuildArgs(cam model.Camera, probe ProbeResult, caps Capabilities) ([]string, error) {
	in, err := BuildInputURL(cam)
	if err != nil {
		return nil, err
	}
	plan := videoPlan(cam, probe, caps)

	args := make([]string, 0, 24)
	args = append(args, plan.preInput...)
	args = append(args, "-rtsp_transport", "tcp", "-rw_timeout", "5000000", "-i", in)
	if plan.copyVideo {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, plan.videoArgs...)
	}
	args = append(args, audioArgs(probe)...)
	args = append(args, "-f", "flv", cam.RTMPURL)
	return args, nil
}

func videoPlan(cam model.Camera, probe ProbeResult, caps Capabilities) encoderPlan {
	if !needsVideoTranscode(cam.TranscodeMode, probe.VideoCodec) {
		return encoderPlan{copyVideo: true}
	}
	return selectEncoder(cam.AccelMode, caps)
}

func needsVideoTranscode(mode model.TranscodeMode, videoCodec string) bool {
	switch mode {
	case model.TranscodeCopy:
		return false
	case model.TranscodeForce:
		return true
	case model.TranscodeKeepHEVC:
		return videoCodec != "h264" && videoCodec != "hevc"
	default: // auto
		return videoCodec != "h264"
	}
}

func selectEncoder(accel model.AccelMode, caps Capabilities) encoderPlan {
	if accel == model.AccelAuto || accel == model.AccelHardware {
		switch {
		case caps.NVENC:
			return encoderPlan{videoArgs: []string{"-c:v", "h264_nvenc", "-preset", "p4", "-tune", "ll", "-g", "50"}}
		case caps.QSV:
			return encoderPlan{videoArgs: []string{"-c:v", "h264_qsv", "-g", "50"}}
		case caps.AMF:
			return encoderPlan{videoArgs: []string{"-c:v", "h264_amf", "-usage", "lowlatency", "-g", "50"}}
		case caps.VAAPI:
			return encoderPlan{
				preInput:  []string{"-vaapi_device", "/dev/dri/renderD128"},
				videoArgs: []string{"-vf", "format=nv12,hwupload", "-c:v", "h264_vaapi", "-g", "50"},
			}
		}
		// Hardware requested but unavailable: fall back to software.
	}
	return encoderPlan{videoArgs: []string{"-c:v", "libx264", "-preset", "veryfast", "-tune", "zerolatency", "-g", "50"}}
}

func audioArgs(probe ProbeResult) []string {
	if !probe.HasAudio {
		return []string{"-an"}
	}
	if probe.AudioCodec == "aac" {
		return []string{"-c:a", "copy"}
	}
	return []string{"-c:a", "aac", "-b:a", "128k"}
}
