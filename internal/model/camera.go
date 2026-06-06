package model

import (
	"errors"
	"strings"
	"time"
)

// Status is the runtime state of a camera conversion.
type Status string

const (
	StatusStopped      Status = "parada"
	StatusStarting     Status = "iniciando"
	StatusRunning      Status = "rodando"
	StatusReconnecting Status = "reconectando"
	StatusError        Status = "erro"
)

// TranscodeMode controls remux-vs-transcode per camera.
type TranscodeMode string

const (
	TranscodeAuto     TranscodeMode = "auto"      // copy if h264, else transcode
	TranscodeCopy     TranscodeMode = "copy"      // force remux
	TranscodeForce    TranscodeMode = "transcode" // force transcode
	TranscodeKeepHEVC TranscodeMode = "keep_h265" // copy h264/hevc, transcode others
)

// AccelMode controls hardware acceleration selection.
type AccelMode string

const (
	AccelAuto     AccelMode = "auto"
	AccelHardware AccelMode = "hardware"
	AccelSoftware AccelMode = "software"
)

// Camera is a persisted conversion definition.
type Camera struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	RTSPURL       string        `json:"rtsp_url"`
	Username      string        `json:"username"`
	Password      string        `json:"password"`
	RTMPURL       string        `json:"rtmp_url"`
	Enabled       bool          `json:"enabled"`
	TranscodeMode TranscodeMode `json:"transcode_mode"`
	AccelMode     AccelMode     `json:"accel_mode"`
	DetectedVideo string        `json:"detected_video,omitempty"`
	DetectedAudio string        `json:"detected_audio,omitempty"`
}

// CameraStatus is the in-memory runtime state (never persisted).
type CameraStatus struct {
	ID        string    `json:"id"`
	Status    Status    `json:"status"`
	LastError string    `json:"last_error,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	Restarts  int       `json:"restarts"`
}

// ApplyDefaults fills empty mode fields with their defaults.
func (c *Camera) ApplyDefaults() {
	if c.TranscodeMode == "" {
		c.TranscodeMode = TranscodeAuto
	}
	if c.AccelMode == "" {
		c.AccelMode = AccelAuto
	}
}

// Validate checks required fields and URL schemes.
func (c *Camera) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("nome é obrigatório")
	}
	if !strings.HasPrefix(c.RTSPURL, "rtsp://") {
		return errors.New("URL de ingest deve começar com rtsp://")
	}
	if !strings.HasPrefix(c.RTMPURL, "rtmp://") {
		return errors.New("URL de destino deve começar com rtmp://")
	}
	return nil
}
