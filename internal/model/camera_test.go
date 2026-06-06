package model

import "testing"

func TestApplyDefaults(t *testing.T) {
	c := Camera{Name: "Cam 1"}
	c.ApplyDefaults()
	if c.TranscodeMode != TranscodeAuto {
		t.Fatalf("transcode mode = %q, want %q", c.TranscodeMode, TranscodeAuto)
	}
	if c.AccelMode != AccelAuto {
		t.Fatalf("accel mode = %q, want %q", c.AccelMode, AccelAuto)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		cam  Camera
		ok   bool
	}{
		{"ok", Camera{Name: "A", RTSPURL: "rtsp://h/s", RTMPURL: "rtmp://h/a"}, true},
		{"no name", Camera{RTSPURL: "rtsp://h/s", RTMPURL: "rtmp://h/a"}, false},
		{"bad rtsp", Camera{Name: "A", RTSPURL: "http://h", RTMPURL: "rtmp://h/a"}, false},
		{"bad rtmp", Camera{Name: "A", RTSPURL: "rtsp://h/s", RTMPURL: "http://h"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cam.Validate()
			if (err == nil) != tc.ok {
				t.Fatalf("Validate() err=%v, want ok=%v", err, tc.ok)
			}
		})
	}
}
