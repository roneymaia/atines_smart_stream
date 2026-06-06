package media

import "testing"

const sampleProbeJSON = `{
  "streams": [
    {"codec_type": "video", "codec_name": "h264"},
    {"codec_type": "audio", "codec_name": "aac"}
  ]
}`

const sampleProbeNoAudio = `{"streams":[{"codec_type":"video","codec_name":"mjpeg"}]}`

func TestParseProbeJSON(t *testing.T) {
	r, err := parseProbeJSON([]byte(sampleProbeJSON))
	if err != nil {
		t.Fatal(err)
	}
	if r.VideoCodec != "h264" || r.AudioCodec != "aac" || !r.HasAudio {
		t.Fatalf("got %+v", r)
	}
}

func TestParseProbeJSON_NoAudio(t *testing.T) {
	r, err := parseProbeJSON([]byte(sampleProbeNoAudio))
	if err != nil {
		t.Fatal(err)
	}
	if r.VideoCodec != "mjpeg" || r.HasAudio {
		t.Fatalf("got %+v", r)
	}
}
