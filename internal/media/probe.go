package media

import (
	"context"
	"encoding/json"
	"os/exec"
)

type probeOutput struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
	} `json:"streams"`
}

func parseProbeJSON(data []byte) (ProbeResult, error) {
	var po probeOutput
	if err := json.Unmarshal(data, &po); err != nil {
		return ProbeResult{}, err
	}
	var r ProbeResult
	for _, s := range po.Streams {
		switch s.CodecType {
		case "video":
			if r.VideoCodec == "" {
				r.VideoCodec = s.CodecName
			}
		case "audio":
			if !r.HasAudio {
				r.AudioCodec = s.CodecName
				r.HasAudio = true
			}
		}
	}
	return r, nil
}

// Probe runs ffprobe against an input URL and returns codec info.
func Probe(ctx context.Context, ffprobePath, inputURL string) (ProbeResult, error) {
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "error",
		"-rtsp_transport", "tcp",
		"-print_format", "json",
		"-show_streams",
		inputURL,
	)
	out, err := cmd.Output()
	if err != nil {
		return ProbeResult{}, err
	}
	return parseProbeJSON(out)
}
