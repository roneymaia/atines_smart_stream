package media

import "testing"

const encodersSample = `
 Encoders:
 V..... libx264              libx264 H.264 / AVC
 V..... h264_nvenc           NVIDIA NVENC H.264 encoder
 V..... h264_vaapi           H.264/AVC (VAAPI)
`

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
