package media

// ProbeResult is the codec info detected from an input stream.
type ProbeResult struct {
	VideoCodec string // e.g. "h264", "hevc", "mjpeg", "mpeg4", "h263"
	AudioCodec string // e.g. "aac", "pcm_mulaw"; empty if no audio
	HasAudio   bool
}

// Capabilities lists hardware H.264 encoders available in the bundled ffmpeg
// plus the RTSP socket-timeout flag that this ffmpeg understands.
type Capabilities struct {
	NVENC bool // h264_nvenc
	QSV   bool // h264_qsv
	AMF   bool // h264_amf
	VAAPI bool // h264_vaapi
	// RTSPTimeoutOpt is the input flag for the RTSP socket I/O timeout:
	// "-stimeout" on ffmpeg 4.x, "-timeout" on ffmpeg 5.x+. Older ffmpeg
	// rejects "-rw_timeout" outright ("Option rw_timeout not found").
	RTSPTimeoutOpt string
}

// encoderPlan describes how the video stream is handled.
type encoderPlan struct {
	copyVideo bool
	preInput  []string // options that must appear before -i (e.g. vaapi device)
	videoArgs []string // -c:v ... (and -vf for vaapi)
}
