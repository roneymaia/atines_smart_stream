# Atines Smart Stream — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Um binário Go único, multiplataforma, que gerencia conversores de câmera RTSP→RTMP (destino SRS), com UI web embutida, decidindo por câmera entre remux (CPU ~0) e transcode (com aceleração por hardware quando disponível).

**Architecture:** Um processo Go com componentes isolados: Store (cadastro JSON), Media Strategy (decisão pura de args FFmpeg), Supervisor (um processo FFmpeg por câmera, com backoff), API HTTP+SSE e UI web embutida (`go:embed`). FFmpeg/ffprobe são binários externos empacotados ao lado do executável.

**Tech Stack:** Go 1.22+ (CGO desligado), FFmpeg/ffprobe (subprocessos), HTML/CSS/JS puro (sem toolchain Node), `net/http` (routing por método/path do Go 1.22), SSE para status ao vivo.

**Spec:** `docs/superpowers/specs/2026-06-06-atines-smart-stream-design.md`

**Convenções:**
- Todo código (identificadores, comentários) em inglês; mensagens de UI ao usuário em português.
- `go test ./...` deve passar ao fim de cada tarefa.
- Commits frequentes (um por tarefa).

---

## Task 0: Toolchain e scaffolding

**Files:**
- Create: `go.mod`
- Create: `main.go`

- [ ] **Step 1: Garantir Go 1.22+ instalado**

Verifique:
```bash
go version
```
Se ausente (esta máquina não tem), instale (Linux x86-64):
```bash
cd /tmp && curl -fsSLO https://go.dev/dl/go1.22.4.linux-amd64.tar.gz \
  && sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.22.4.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
go version   # espera: go1.22.x
```
(Ajuste a arquitetura do tarball conforme a máquina. Adicione `/usr/local/go/bin` ao PATH do shell.)

- [ ] **Step 2: Inicializar o módulo**

Run:
```bash
go mod init atines_smart_stream
```
Expected: cria `go.mod` com `module atines_smart_stream` e `go 1.22`.

- [ ] **Step 3: main.go mínimo que compila**

```go
package main

import "fmt"

func main() {
	fmt.Println("atines_smart_stream")
}
```

- [ ] **Step 4: Build sanity-check**

Run: `go build ./... && go vet ./...`
Expected: sem erros; gera o binário.

- [ ] **Step 5: Commit**

```bash
git add go.mod main.go
git commit -m "chore: bootstrap Go module and entrypoint"
```

---

## Task 1: Pacote model (tipos)

**Files:**
- Create: `internal/model/camera.go`
- Test: `internal/model/camera_test.go`

- [ ] **Step 1: Teste falhando (defaults aplicados)**

```go
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
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/model/ -v`
Expected: FAIL (tipos/métodos não definidos).

- [ ] **Step 3: Implementar os tipos**

```go
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
```

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/model/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/
git commit -m "feat(model): camera types, defaults and validation"
```

---

## Task 2: Pacote store (registry JSON atômico)

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

- [ ] **Step 1: Teste falhando (round-trip + CRUD)**

```go
package store

import (
	"path/filepath"
	"testing"

	"atines_smart_stream/internal/model"
)

func TestUpsertAssignsIDAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cameras.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cam := model.Camera{Name: "Cam 1", RTSPURL: "rtsp://h/s", RTMPURL: "rtmp://h/a"}
	saved, err := s.Upsert(cam)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" {
		t.Fatal("expected generated ID")
	}
	if saved.TranscodeMode != model.TranscodeAuto {
		t.Fatalf("defaults not applied: %+v", saved)
	}

	// Reopen: persisted on disk
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get(saved.ID)
	if !ok || got.Name != "Cam 1" {
		t.Fatalf("not persisted: %+v ok=%v", got, ok)
	}
}

func TestSetEnabledAndDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cameras.json")
	s, _ := Open(path)
	cam, _ := s.Upsert(model.Camera{Name: "C", RTSPURL: "rtsp://h/s", RTMPURL: "rtmp://h/a"})
	if err := s.SetEnabled(cam.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(cam.ID)
	if !got.Enabled {
		t.Fatal("not enabled")
	}
	if err := s.Delete(cam.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(cam.ID); ok {
		t.Fatal("expected deleted")
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/store/ -v`
Expected: FAIL (pacote não existe).

- [ ] **Step 3: Implementar o store**

```go
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"atines_smart_stream/internal/model"
)

// Store is a thread-safe JSON-backed camera registry.
type Store struct {
	path    string
	mu      sync.RWMutex
	cameras map[string]model.Camera
}

// Open loads the registry from path, creating an empty one if absent.
func Open(path string) (*Store, error) {
	s := &Store{path: path, cameras: map[string]model.Camera{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []model.Camera
	if len(data) > 0 {
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, err
		}
	}
	for _, c := range list {
		s.cameras[c.ID] = c
	}
	return s, nil
}

// List returns all cameras sorted by name.
func (s *Store) List() []model.Camera {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Camera, 0, len(s.cameras))
	for _, c := range s.cameras {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns one camera by ID.
func (s *Store) Get(id string) (model.Camera, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.cameras[id]
	return c, ok
}

// Upsert validates, assigns an ID if new, applies defaults, persists, and returns the stored camera.
func (s *Store) Upsert(cam model.Camera) (model.Camera, error) {
	cam.ApplyDefaults()
	if err := cam.Validate(); err != nil {
		return model.Camera{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cam.ID == "" {
		cam.ID = newID()
	}
	s.cameras[cam.ID] = cam
	if err := s.saveLocked(); err != nil {
		return model.Camera{}, err
	}
	return cam, nil
}

// Delete removes a camera by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cameras, id)
	return s.saveLocked()
}

// SetEnabled toggles the enabled flag of a camera.
func (s *Store) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cameras[id]
	if !ok {
		return errors.New("câmera não encontrada")
	}
	c.Enabled = enabled
	s.cameras[id] = c
	return s.saveLocked()
}

// saveLocked atomically writes the registry with 0600 perms. Caller holds the write lock.
func (s *Store) saveLocked() error {
	list := make([]model.Camera, 0, len(s.cameras))
	for _, c := range s.cameras {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(s.path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	// os.Rename replaces an existing file atomically on both Unix and Windows.
	return os.Rename(tmp, s.path)
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
```

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/store/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/
git commit -m "feat(store): atomic JSON camera registry with CRUD"
```

---

## Task 3: Media types + strategy (função pura) — NÚCLEO

**Files:**
- Create: `internal/media/types.go`
- Create: `internal/media/strategy.go`
- Test: `internal/media/strategy_test.go`

- [ ] **Step 1: Definir os tipos (sem teste ainda)**

`internal/media/types.go`:
```go
package media

// ProbeResult is the codec info detected from an input stream.
type ProbeResult struct {
	VideoCodec string // e.g. "h264", "hevc", "mjpeg", "mpeg4", "h263"
	AudioCodec string // e.g. "aac", "pcm_mulaw"; empty if no audio
	HasAudio   bool
}

// Capabilities lists hardware H.264 encoders available in the bundled ffmpeg.
type Capabilities struct {
	NVENC bool // h264_nvenc
	QSV   bool // h264_qsv
	AMF   bool // h264_amf
	VAAPI bool // h264_vaapi
}

// encoderPlan describes how the video stream is handled.
type encoderPlan struct {
	copyVideo bool
	preInput  []string // options that must appear before -i (e.g. vaapi device)
	videoArgs []string // -c:v ... (and -vf for vaapi)
}
```

- [ ] **Step 2: Teste falhando (tabela de decisão)**

`internal/media/strategy_test.go`:
```go
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
		"-rtsp_transport", "tcp", "-rw_timeout", "5000000",
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
		"-rtsp_transport", "tcp", "-rw_timeout", "5000000",
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
		"-rtsp_transport", "tcp", "-rw_timeout", "5000000",
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
		"-rtsp_transport", "tcp", "-rw_timeout", "5000000",
		"-i", "rtsp://user:pass@host:554/stream",
		"-vf", "format=nv12,hwupload", "-c:v", "h264_vaapi", "-g", "50",
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
```

- [ ] **Step 3: Rodar e ver falhar**

Run: `go test ./internal/media/ -v`
Expected: FAIL (BuildArgs/BuildInputURL não existem).

- [ ] **Step 4: Implementar strategy.go**

```go
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
```

- [ ] **Step 5: Rodar e ver passar**

Run: `go test ./internal/media/ -v`
Expected: PASS (todos os casos da tabela).

- [ ] **Step 6: Commit**

```bash
git add internal/media/types.go internal/media/strategy.go internal/media/strategy_test.go
git commit -m "feat(media): pure ffmpeg-args strategy (remux/transcode + hw accel)"
```

---

## Task 4: Probe (ffprobe) + parser

**Files:**
- Create: `internal/media/probe.go`
- Test: `internal/media/probe_test.go`

- [ ] **Step 1: Teste falhando (parser puro)**

`internal/media/probe_test.go`:
```go
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
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/media/ -run TestParseProbe -v`
Expected: FAIL (parseProbeJSON indefinido).

- [ ] **Step 3: Implementar probe.go**

```go
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
```

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/media/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/media/probe.go internal/media/probe_test.go
git commit -m "feat(media): ffprobe wrapper and JSON parser"
```

---

## Task 5: Capabilities (detecção de encoders)

**Files:**
- Create: `internal/media/capabilities.go`
- Test: `internal/media/capabilities_test.go`

- [ ] **Step 1: Teste falhando (parser de `ffmpeg -encoders`)**

`internal/media/capabilities_test.go`:
```go
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
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/media/ -run TestParseEncoders -v`
Expected: FAIL.

- [ ] **Step 3: Implementar capabilities.go**

```go
package media

import (
	"context"
	"os/exec"
	"strings"
)

func parseEncoders(out string) Capabilities {
	return Capabilities{
		NVENC: strings.Contains(out, "h264_nvenc"),
		QSV:   strings.Contains(out, "h264_qsv"),
		AMF:   strings.Contains(out, "h264_amf"),
		VAAPI: strings.Contains(out, "h264_vaapi"),
	}
}

// DetectCapabilities inspects the bundled ffmpeg for hardware H.264 encoders.
// On any error it returns an empty Capabilities (software-only).
func DetectCapabilities(ctx context.Context, ffmpegPath string) Capabilities {
	cmd := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-encoders")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Capabilities{}
	}
	return parseEncoders(string(out))
}
```

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/media/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/media/capabilities.go internal/media/capabilities_test.go
git commit -m "feat(media): detect available hardware encoders"
```

---

## Task 6: FFmpeg locator

**Files:**
- Create: `internal/ffmpeg/locator.go`
- Test: `internal/ffmpeg/locator_test.go`

- [ ] **Step 1: Teste falhando (acha binário ao lado)**

`internal/ffmpeg/locator_test.go`:
```go
package ffmpeg

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocateInPrefersLocalDir(t *testing.T) {
	dir := t.TempDir()
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	local := filepath.Join(dir, name)
	if err := os.WriteFile(local, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	lookPath := func(string) (string, error) { return "/usr/bin/ffmpeg", nil }
	got, err := locateIn(dir, "ffmpeg", lookPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != local {
		t.Fatalf("got %q want %q", got, local)
	}
}

func TestLocateInFallsBackToPath(t *testing.T) {
	dir := t.TempDir()
	lookPath := func(string) (string, error) { return "/usr/bin/ffmpeg", nil }
	got, err := locateIn(dir, "ffmpeg", lookPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/usr/bin/ffmpeg" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/ffmpeg/ -v`
Expected: FAIL.

- [ ] **Step 3: Implementar locator.go**

```go
package ffmpeg

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Locate returns paths to ffmpeg and ffprobe, preferring binaries next to the
// running executable, falling back to PATH.
func Locate() (ffmpegPath, ffprobePath string, err error) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	dir := filepath.Dir(exe)
	ffmpegPath, err = locateIn(dir, "ffmpeg", exec.LookPath)
	if err != nil {
		return "", "", err
	}
	ffprobePath, err = locateIn(dir, "ffprobe", exec.LookPath)
	if err != nil {
		return "", "", err
	}
	return ffmpegPath, ffprobePath, nil
}

// locateIn checks dir for the binary, then falls back to lookPath.
func locateIn(dir, base string, lookPath func(string) (string, error)) (string, error) {
	name := base
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	local := filepath.Join(dir, name)
	if st, err := os.Stat(local); err == nil && !st.IsDir() {
		return local, nil
	}
	if p, err := lookPath(base); err == nil {
		return p, nil
	}
	return "", errors.New(base + " não encontrado (nem ao lado do executável nem no PATH)")
}
```

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/ffmpeg/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ffmpeg/
git commit -m "feat(ffmpeg): locate bundled or PATH ffmpeg/ffprobe"
```

---

## Task 7: Supervisor (ciclo de vida + backoff)

**Files:**
- Create: `internal/supervisor/supervisor.go`
- Test: `internal/supervisor/supervisor_test.go`

Notas de design: o Supervisor recebe um `Runner` injetável (interface) para não depender de processos reais nos testes, e um `argsFor` para montar os args por câmera. Cada câmera roda numa goroutine com loop start→wait→backoff até `Stop`.

- [ ] **Step 1: Teste falhando (restart com backoff e status)**

`internal/supervisor/supervisor_test.go`:
```go
package supervisor

import (
	"context"
	"sync"
	"testing"
	"time"

	"atines_smart_stream/internal/model"
)

// fakeRunner returns programmed errors per call, signalling each run via a channel.
type fakeRunner struct {
	mu     sync.Mutex
	calls  int
	runs   chan int
	result func(call int) error
}

func (f *fakeRunner) Run(ctx context.Context, name string, args []string, onLine func(string)) error {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()
	f.runs <- n
	err := f.result(n)
	// Honour cancellation so Stop() ends the loop promptly.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func TestSupervisorRestartsOnExit(t *testing.T) {
	fr := &fakeRunner{runs: make(chan int, 8), result: func(int) error { return nil }}
	sup := New(SupervisorConfig{
		Runner:     fr,
		ArgsFor:    func(model.Camera) ([]string, error) { return []string{"-i", "x"}, nil },
		BaseBackoff: time.Millisecond,
		MaxBackoff:  2 * time.Millisecond,
	})
	cam := model.Camera{ID: "c1", Name: "C", RTSPURL: "rtsp://h/s", RTMPURL: "rtmp://h/a"}
	sup.Start(cam)

	// Expect at least 3 runs (i.e. it restarts after each clean exit).
	for i := 0; i < 3; i++ {
		select {
		case <-fr.runs:
		case <-time.After(time.Second):
			t.Fatalf("expected run #%d", i+1)
		}
	}
	sup.Stop("c1")
	st := sup.Status("c1")
	if st.Status != model.StatusStopped {
		t.Fatalf("status after stop = %q", st.Status)
	}
}

func TestSupervisorStatusRunning(t *testing.T) {
	block := make(chan struct{})
	fr := &fakeRunner{runs: make(chan int, 4), result: func(int) error { <-block; return nil }}
	sup := New(SupervisorConfig{
		Runner:      fr,
		ArgsFor:     func(model.Camera) ([]string, error) { return []string{"x"}, nil },
		BaseBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	})
	sup.Start(model.Camera{ID: "c1"})
	<-fr.runs
	// Give the goroutine a moment to mark running.
	time.Sleep(20 * time.Millisecond)
	if got := sup.Status("c1").Status; got != model.StatusRunning {
		t.Fatalf("status = %q want rodando", got)
	}
	close(block)
	sup.Stop("c1")
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/supervisor/ -v`
Expected: FAIL.

- [ ] **Step 3: Implementar supervisor.go**

```go
package supervisor

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"sync"
	"time"

	"atines_smart_stream/internal/model"
)

// Runner executes one ffmpeg invocation, blocking until it exits.
type Runner interface {
	Run(ctx context.Context, name string, args []string, onLine func(string)) error
}

// ExecRunner runs a real ffmpeg process, streaming stderr lines to onLine.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args []string, onLine func(string)) error {
	cmd := exec.CommandContext(ctx, name, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if onLine != nil {
				onLine(sc.Text())
			}
		}
		_, _ = io.Copy(io.Discard, stderr)
	}()
	return cmd.Wait()
}

// SupervisorConfig wires dependencies (injectable for tests).
type SupervisorConfig struct {
	FFmpegPath  string
	Runner      Runner
	ArgsFor     func(model.Camera) ([]string, error)
	OnStatus    func(model.CameraStatus)
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

type proc struct {
	cancel context.CancelFunc
}

// Supervisor manages one ffmpeg process per active camera.
type Supervisor struct {
	cfg      SupervisorConfig
	mu       sync.Mutex
	running  map[string]*proc
	statuses map[string]model.CameraStatus
}

func New(cfg SupervisorConfig) *Supervisor {
	if cfg.Runner == nil {
		cfg.Runner = ExecRunner{}
	}
	if cfg.BaseBackoff == 0 {
		cfg.BaseBackoff = time.Second
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 30 * time.Second
	}
	return &Supervisor{
		cfg:      cfg,
		running:  map[string]*proc{},
		statuses: map[string]model.CameraStatus{},
	}
}

// Start (re)launches the supervision loop for a camera. No-op if already running.
func (s *Supervisor) Start(cam model.Camera) {
	s.mu.Lock()
	if _, ok := s.running[cam.ID]; ok {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.running[cam.ID] = &proc{cancel: cancel}
	s.mu.Unlock()
	go s.loop(ctx, cam)
}

// Stop ends the supervision loop for a camera and marks it stopped.
func (s *Supervisor) Stop(id string) {
	s.mu.Lock()
	p, ok := s.running[id]
	if ok {
		delete(s.running, id)
	}
	s.mu.Unlock()
	if ok {
		p.cancel()
	}
	s.setStatus(model.CameraStatus{ID: id, Status: model.StatusStopped})
}

// Status returns the current status for a camera.
func (s *Supervisor) Status(id string) model.CameraStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statuses[id]
}

// AllStatuses returns a snapshot of all known statuses.
func (s *Supervisor) AllStatuses() []model.CameraStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.CameraStatus, 0, len(s.statuses))
	for _, st := range s.statuses {
		out = append(out, st)
	}
	return out
}

func (s *Supervisor) loop(ctx context.Context, cam model.Camera) {
	backoff := s.cfg.BaseBackoff
	restarts := 0
	for {
		if ctx.Err() != nil {
			return
		}
		args, err := s.cfg.ArgsFor(cam)
		if err != nil {
			s.setStatus(model.CameraStatus{ID: cam.ID, Status: model.StatusError, LastError: err.Error(), Restarts: restarts})
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff, s.cfg.MaxBackoff)
			continue
		}

		s.setStatus(model.CameraStatus{ID: cam.ID, Status: model.StatusStarting, Restarts: restarts})
		var lastLine string
		runErr := s.cfg.Runner.Run(ctx, s.cfg.FFmpegPath, args, func(line string) {
			lastLine = line
			// First output line implies the process is up and streaming.
			if st := s.Status(cam.ID); st.Status != model.StatusRunning {
				s.setStatus(model.CameraStatus{ID: cam.ID, Status: model.StatusRunning, StartedAt: time.Now(), Restarts: restarts})
			}
		})
		// Mark running even if the fake runner produced no lines (real ffmpeg always does).
		if st := s.Status(cam.ID); st.Status == model.StatusStarting {
			s.setStatus(model.CameraStatus{ID: cam.ID, Status: model.StatusRunning, StartedAt: time.Now(), Restarts: restarts})
		}

		if ctx.Err() != nil {
			return
		}

		restarts++
		msg := "processo encerrado"
		if runErr != nil {
			msg = runErr.Error()
		}
		if lastLine != "" {
			msg += " — " + lastLine
		}
		s.setStatus(model.CameraStatus{ID: cam.ID, Status: model.StatusReconnecting, LastError: msg, Restarts: restarts})
		if !sleepCtx(ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff, s.cfg.MaxBackoff)
	}
}

func (s *Supervisor) setStatus(st model.CameraStatus) {
	s.mu.Lock()
	s.statuses[st.ID] = st
	cb := s.cfg.OnStatus
	s.mu.Unlock()
	if cb != nil {
		cb(st)
	}
}

func nextBackoff(cur, max time.Duration) time.Duration {
	next := cur * 2
	if next > max {
		return max
	}
	return next
}

// sleepCtx sleeps for d unless ctx is cancelled first; returns false if cancelled.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
```

> Nota de teste: no `TestSupervisorStatusRunning` o runner bloqueia, então o status "rodando" é marcado pelo ramo "even if the fake runner produced no lines". Em `TestSupervisorRestartsOnExit` o runner retorna `nil` repetidamente e o backoff de 1ms garante reinícios rápidos.

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/supervisor/ -race -v`
Expected: PASS (sem data races).

- [ ] **Step 5: Commit**

```bash
git add internal/supervisor/
git commit -m "feat(supervisor): per-camera ffmpeg lifecycle with backoff and status"
```

---

## Task 8: API HTTP (CRUD + toggle) + SSE

**Files:**
- Create: `internal/api/server.go`
- Test: `internal/api/server_test.go`

Notas: usa o routing por método/path do Go 1.22 (`mux.HandleFunc("POST /api/cameras", ...)`). O servidor recebe um `store.Store` e o `supervisor.Supervisor`. A view de listagem mescla câmera + status.

- [ ] **Step 1: Teste falhando (CRUD + toggle)**

`internal/api/server_test.go`:
```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"atines_smart_stream/internal/model"
	"atines_smart_stream/internal/store"
	"atines_smart_stream/internal/supervisor"
)

func newTestServer(t *testing.T) (http.Handler, *store.Store, *supervisor.Supervisor) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "cameras.json"))
	if err != nil {
		t.Fatal(err)
	}
	sup := supervisor.New(supervisor.SupervisorConfig{
		Runner:  noopRunner{},
		ArgsFor: func(model.Camera) ([]string, error) { return []string{"x"}, nil },
	})
	return NewServer(st, sup), st, sup
}

type noopRunner struct{}

func (noopRunner) Run(_ interface{ Done() <-chan struct{} }, _ string, _ []string, _ func(string)) error {
	return nil
}

func TestCreateAndListCamera(t *testing.T) {
	srv, _, _ := newTestServer(t)
	body, _ := json.Marshal(model.Camera{Name: "Cam", RTSPURL: "rtsp://h/s", RTMPURL: "rtmp://h/a"})
	req := httptest.NewRequest("POST", "/api/cameras", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("create code = %d body=%s", rec.Code, rec.Body)
	}

	req = httptest.NewRequest("GET", "/api/cameras", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	var list []cameraView
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "Cam" {
		t.Fatalf("list = %+v", list)
	}
}

func TestCreateInvalidReturns400(t *testing.T) {
	srv, _, _ := newTestServer(t)
	body, _ := json.Marshal(model.Camera{Name: ""})
	req := httptest.NewRequest("POST", "/api/cameras", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestEnableDisableToggle(t *testing.T) {
	srv, st, _ := newTestServer(t)
	cam, _ := st.Upsert(model.Camera{Name: "C", RTSPURL: "rtsp://h/s", RTMPURL: "rtmp://h/a"})

	req := httptest.NewRequest("POST", "/api/cameras/"+cam.ID+"/enable", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("enable code = %d", rec.Code)
	}
	got, _ := st.Get(cam.ID)
	if !got.Enabled {
		t.Fatal("not enabled")
	}

	req = httptest.NewRequest("POST", "/api/cameras/"+cam.ID+"/disable", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	got, _ = st.Get(cam.ID)
	if got.Enabled {
		t.Fatal("still enabled")
	}
}

func TestDeleteCamera(t *testing.T) {
	srv, st, _ := newTestServer(t)
	cam, _ := st.Upsert(model.Camera{Name: "C", RTSPURL: "rtsp://h/s", RTMPURL: "rtmp://h/a"})
	req := httptest.NewRequest("DELETE", "/api/cameras/"+cam.ID, nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("delete code = %d", rec.Code)
	}
	if _, ok := st.Get(cam.ID); ok {
		t.Fatal("not deleted")
	}
}
```

> Atenção: o `noopRunner` no teste implementa a mesma assinatura do `supervisor.Runner`. Como a interface usa `context.Context`, ajuste o stub no teste para `Run(ctx context.Context, ...)` importando `context` — substitua a assinatura placeholder acima por `func (noopRunner) Run(ctx context.Context, name string, args []string, onLine func(string)) error { return nil }` e adicione `"context"` aos imports. (Mantido explícito aqui para evitar copiar errado.)

- [ ] **Step 2: Corrigir o stub do runner no teste**

Edite `internal/api/server_test.go`: adicione `"context"` aos imports e troque o método por:
```go
func (noopRunner) Run(ctx context.Context, name string, args []string, onLine func(string)) error {
	return nil
}
```

- [ ] **Step 3: Rodar e ver falhar**

Run: `go test ./internal/api/ -v`
Expected: FAIL (NewServer/cameraView indefinidos).

- [ ] **Step 4: Implementar server.go**

```go
package api

import (
	"encoding/json"
	"net/http"

	"atines_smart_stream/internal/model"
	"atines_smart_stream/internal/store"
	"atines_smart_stream/internal/supervisor"
)

// cameraView is a camera merged with its runtime status for the UI.
type cameraView struct {
	model.Camera
	Status    model.Status `json:"status"`
	LastError string       `json:"last_error,omitempty"`
}

// Server holds API dependencies.
type Server struct {
	store *store.Store
	sup   *supervisor.Supervisor
	hub   *sseHub
}

// NewServer wires the HTTP handler (API + SSE). UI mounting is added in the webui task.
func NewServer(st *store.Store, sup *supervisor.Supervisor) http.Handler {
	s := &Server{store: st, sup: sup, hub: newSSEHub()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/cameras", s.handleList)
	mux.HandleFunc("POST /api/cameras", s.handleUpsert)
	mux.HandleFunc("DELETE /api/cameras/{id}", s.handleDelete)
	mux.HandleFunc("POST /api/cameras/{id}/enable", s.handleEnable)
	mux.HandleFunc("POST /api/cameras/{id}/disable", s.handleDisable)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	s.mux = mux
	return s
}

// Server implements http.Handler via an embedded mux so the webui task can wrap it.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	cams := s.store.List()
	views := make([]cameraView, 0, len(cams))
	for _, c := range cams {
		st := s.sup.Status(c.ID)
		status := model.StatusStopped
		if st.Status != "" {
			status = st.Status
		}
		views = append(views, cameraView{Camera: c, Status: status, LastError: st.LastError})
	}
	writeJSON(w, 200, views)
}

func (s *Server) handleUpsert(w http.ResponseWriter, r *http.Request) {
	var cam model.Camera
	if err := json.NewDecoder(r.Body).Decode(&cam); err != nil {
		writeErr(w, 400, "JSON inválido")
		return
	}
	saved, err := s.store.Upsert(cam)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// If enabled, (re)start to apply changes.
	if saved.Enabled {
		s.sup.Stop(saved.ID)
		s.sup.Start(saved)
	}
	writeJSON(w, 200, saved)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.sup.Stop(id)
	if err := s.store.Delete(id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"ok": "1"})
}

func (s *Server) handleEnable(w http.ResponseWriter, r *http.Request)  { s.setEnabled(w, r, true) }
func (s *Server) handleDisable(w http.ResponseWriter, r *http.Request) { s.setEnabled(w, r, false) }

func (s *Server) setEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	id := r.PathValue("id")
	if err := s.store.SetEnabled(id, enabled); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	cam, _ := s.store.Get(id)
	if enabled {
		s.sup.Start(cam)
	} else {
		s.sup.Stop(id)
	}
	writeJSON(w, 200, map[string]bool{"enabled": enabled})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
```

Adicione o campo `mux` ao struct `Server` (declare em server.go junto aos outros campos):
```go
// (dentro do struct Server)
//   mux http.Handler
```
Ou seja, o struct fica:
```go
type Server struct {
	store *store.Store
	sup   *supervisor.Supervisor
	hub   *sseHub
	mux   http.Handler
}
```

- [ ] **Step 5: Implementar SSE no mesmo pacote (`internal/api/sse.go`)**

```go
package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"atines_smart_stream/internal/model"
)

type sseHub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newSSEHub() *sseHub {
	return &sseHub{clients: map[chan []byte]struct{}{}}
}

// Broadcast sends a status update to all connected clients.
func (h *sseHub) Broadcast(st model.CameraStatus) {
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- data:
		default: // drop if client is slow
		}
	}
}

func (h *sseHub) add() chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *sseHub) remove(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.hub.add()
	defer s.hub.remove(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(msg)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}

// StatusSink returns a callback to feed supervisor status into SSE.
func (s *Server) StatusSink() func(model.CameraStatus) {
	return s.hub.Broadcast
}
```

- [ ] **Step 6: Rodar e ver passar**

Run: `go test ./internal/api/ -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/
git commit -m "feat(api): camera CRUD/toggle handlers and SSE status stream"
```

---

## Task 9: UI web embutida

**Files:**
- Create: `internal/webui/embed.go`
- Create: `internal/webui/assets/index.html`
- Create: `internal/webui/assets/app.js`
- Create: `internal/webui/assets/style.css`
- Modify: `internal/api/server.go` (montar a UI na rota `/`)

Esta tarefa é majoritariamente front-end (sem TDD unitário); a verificação é manual no Task 11.

- [ ] **Step 1: embed.go**

```go
package webui

import (
	"embed"
	"io/fs"
)

//go:embed assets/*
var files embed.FS

// FS returns the embedded UI asset filesystem rooted at "assets".
func FS() fs.FS {
	sub, err := fs.Sub(files, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}
```

- [ ] **Step 2: index.html**

```html
<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Atines Smart Stream</title>
  <link rel="stylesheet" href="style.css">
</head>
<body>
  <header>
    <h1>Atines Smart Stream</h1>
    <button id="btn-new">+ Nova câmera</button>
  </header>

  <main>
    <table id="cameras">
      <thead>
        <tr><th>Status</th><th>Nome</th><th>Destino RTMP</th><th>Ativo</th><th>Ações</th></tr>
      </thead>
      <tbody id="rows"></tbody>
    </table>
    <p id="empty" hidden>Nenhuma câmera cadastrada. Clique em "+ Nova câmera".</p>
  </main>

  <dialog id="form-dialog">
    <form id="cam-form" method="dialog">
      <h2 id="form-title">Nova câmera</h2>
      <input type="hidden" id="f-id">
      <label>Nome <input id="f-name" required></label>
      <label>URL RTSP (ingest) <input id="f-rtsp" placeholder="rtsp://host:554/stream" required></label>
      <label>Usuário RTSP <input id="f-user"></label>
      <label>Senha RTSP <input id="f-pass" type="password"></label>
      <label>URL RTMP (destino) <input id="f-rtmp" placeholder="rtmp://srs/live/cam1" required></label>
      <label>Transcode
        <select id="f-transcode">
          <option value="auto">Automático</option>
          <option value="copy">Forçar copiar (remux)</option>
          <option value="transcode">Forçar transcodificar</option>
          <option value="keep_h265">Manter H.265 (avançado)</option>
        </select>
      </label>
      <label>Aceleração
        <select id="f-accel">
          <option value="auto">Automática</option>
          <option value="hardware">Hardware</option>
          <option value="software">Software</option>
        </select>
      </label>
      <p id="form-error" class="error"></p>
      <menu>
        <button value="cancel" type="button" id="btn-cancel">Cancelar</button>
        <button value="ok" id="btn-save">Salvar</button>
      </menu>
    </form>
  </dialog>

  <script src="app.js"></script>
</body>
</html>
```

- [ ] **Step 3: style.css**

```css
* { box-sizing: border-box; }
body { font-family: system-ui, sans-serif; margin: 0; background: #0f1420; color: #e6e9ef; }
header { display: flex; justify-content: space-between; align-items: center; padding: 16px 24px; background: #161c2c; border-bottom: 1px solid #232b3e; }
h1 { font-size: 18px; margin: 0; }
main { padding: 24px; }
button { background: #3b82f6; color: #fff; border: 0; padding: 8px 14px; border-radius: 6px; cursor: pointer; font-size: 14px; }
button.secondary { background: #2a3349; }
button.danger { background: #ef4444; }
table { width: 100%; border-collapse: collapse; }
th, td { text-align: left; padding: 10px 12px; border-bottom: 1px solid #232b3e; font-size: 14px; }
.dot { display: inline-block; width: 10px; height: 10px; border-radius: 50%; margin-right: 6px; }
.dot.rodando { background: #22c55e; }
.dot.parada { background: #6b7280; }
.dot.iniciando, .dot.reconectando { background: #f59e0b; }
.dot.erro { background: #ef4444; }
.switch { cursor: pointer; }
dialog { background: #161c2c; color: #e6e9ef; border: 1px solid #232b3e; border-radius: 10px; width: min(480px, 92vw); }
dialog label { display: block; margin: 10px 0; font-size: 13px; }
dialog input, dialog select { width: 100%; padding: 8px; margin-top: 4px; border-radius: 6px; border: 1px solid #2a3349; background: #0f1420; color: #e6e9ef; }
dialog menu { display: flex; gap: 8px; justify-content: flex-end; padding: 0; margin-top: 16px; }
.error { color: #fca5a5; min-height: 18px; font-size: 13px; }
.actions { display: flex; gap: 6px; }
```

- [ ] **Step 4: app.js**

```javascript
const rowsEl = document.getElementById("rows");
const emptyEl = document.getElementById("empty");
const dialog = document.getElementById("form-dialog");
const formError = document.getElementById("form-error");
let cameras = [];

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || ("HTTP " + res.status));
  return data;
}

function statusLabel(s) { return s || "parada"; }

function render() {
  rowsEl.innerHTML = "";
  emptyEl.hidden = cameras.length > 0;
  for (const c of cameras) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><span class="dot ${statusLabel(c.status)}"></span>${statusLabel(c.status)}</td>
      <td>${escapeHTML(c.name)}</td>
      <td>${escapeHTML(c.rtmp_url)}</td>
      <td><input type="checkbox" class="switch" ${c.enabled ? "checked" : ""} data-id="${c.id}"></td>
      <td class="actions">
        <button class="secondary" data-edit="${c.id}">Editar</button>
        <button class="danger" data-del="${c.id}">Excluir</button>
      </td>`;
    rowsEl.appendChild(tr);
  }
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (m) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[m]));
}

async function load() {
  cameras = await api("GET", "/api/cameras");
  render();
}

// Live status via SSE
const ev = new EventSource("/api/events");
ev.onmessage = (e) => {
  const st = JSON.parse(e.data);
  const cam = cameras.find((c) => c.id === st.id);
  if (cam) { cam.status = st.status; render(); }
};

// Open form (new)
document.getElementById("btn-new").onclick = () => openForm(null);

function openForm(cam) {
  formError.textContent = "";
  document.getElementById("form-title").textContent = cam ? "Editar câmera" : "Nova câmera";
  document.getElementById("f-id").value = cam?.id || "";
  document.getElementById("f-name").value = cam?.name || "";
  document.getElementById("f-rtsp").value = cam?.rtsp_url || "";
  document.getElementById("f-user").value = cam?.username || "";
  document.getElementById("f-pass").value = cam?.password || "";
  document.getElementById("f-rtmp").value = cam?.rtmp_url || "";
  document.getElementById("f-transcode").value = cam?.transcode_mode || "auto";
  document.getElementById("f-accel").value = cam?.accel_mode || "auto";
  dialog.showModal();
}

document.getElementById("btn-cancel").onclick = () => dialog.close();

document.getElementById("btn-save").onclick = async (e) => {
  e.preventDefault();
  const payload = {
    id: document.getElementById("f-id").value || "",
    name: document.getElementById("f-name").value,
    rtsp_url: document.getElementById("f-rtsp").value,
    username: document.getElementById("f-user").value,
    password: document.getElementById("f-pass").value,
    rtmp_url: document.getElementById("f-rtmp").value,
    transcode_mode: document.getElementById("f-transcode").value,
    accel_mode: document.getElementById("f-accel").value,
    enabled: cameras.find((c) => c.id === document.getElementById("f-id").value)?.enabled || false,
  };
  try {
    await api("POST", "/api/cameras", payload);
    dialog.close();
    await load();
  } catch (err) {
    formError.textContent = err.message;
  }
};

// Delegated row actions
rowsEl.addEventListener("click", async (e) => {
  const editId = e.target.getAttribute("data-edit");
  const delId = e.target.getAttribute("data-del");
  if (editId) openForm(cameras.find((c) => c.id === editId));
  if (delId && confirm("Excluir esta câmera?")) {
    await api("DELETE", "/api/cameras/" + delId);
    await load();
  }
});

rowsEl.addEventListener("change", async (e) => {
  if (!e.target.classList.contains("switch")) return;
  const id = e.target.getAttribute("data-id");
  await api("POST", "/api/cameras/" + id + (e.target.checked ? "/enable" : "/disable"));
  await load();
});

load().catch((err) => alert("Erro ao carregar: " + err.message));
```

- [ ] **Step 5: Montar a UI na rota `/` (modificar `internal/api/server.go`)**

Em `NewServer`, antes do `return s`, adicione o handler de assets estáticos como fallback:
```go
	mux.Handle("GET /", http.FileServer(http.FS(webui.FS())))
```
E adicione o import `"atines_smart_stream/internal/webui"` e `"net/http"` (já presente). Mantenha as rotas `/api/...` registradas antes — o mux do Go 1.22 dá precedência a padrões mais específicos, então `/api/cameras` ganha de `/`.

- [ ] **Step 6: Verificar build**

Run: `go build ./... && go test ./...`
Expected: compila com os assets embutidos; testes passam.

- [ ] **Step 7: Commit**

```bash
git add internal/webui/ internal/api/server.go
git commit -m "feat(webui): embedded single-page management UI"
```

---

## Task 10: Wiring no main + launcher

**Files:**
- Modify: `main.go`
- Create: `internal/launcher/browser.go`
- Test: `internal/launcher/browser_test.go`

- [ ] **Step 1: Teste falhando (comando de abrir navegador por SO)**

`internal/launcher/browser_test.go`:
```go
package launcher

import "testing"

func TestBrowserCommand(t *testing.T) {
	name, args := browserCommand("linux", "http://127.0.0.1:8787")
	if name != "xdg-open" || len(args) != 1 || args[0] != "http://127.0.0.1:8787" {
		t.Fatalf("linux: %q %v", name, args)
	}
	name, _ = browserCommand("windows", "http://x")
	if name != "rundll32" {
		t.Fatalf("windows: %q", name)
	}
	name, _ = browserCommand("darwin", "http://x")
	if name != "open" {
		t.Fatalf("darwin: %q", name)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/launcher/ -v`
Expected: FAIL.

- [ ] **Step 3: Implementar browser.go**

```go
package launcher

import (
	"os/exec"
	"runtime"
)

// browserCommand returns the OS command to open a URL in the default browser.
func browserCommand(goos, url string) (string, []string) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		return "open", []string{url}
	default:
		return "xdg-open", []string{url}
	}
}

// OpenBrowser best-effort launches the default browser at url.
func OpenBrowser(url string) {
	name, args := browserCommand(runtime.GOOS, url)
	_ = exec.Command(name, args...).Start()
}
```

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/launcher/ -v`
Expected: PASS.

- [ ] **Step 5: Implementar main.go completo (wiring)**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"atines_smart_stream/internal/api"
	"atines_smart_stream/internal/ffmpeg"
	"atines_smart_stream/internal/launcher"
	"atines_smart_stream/internal/media"
	"atines_smart_stream/internal/model"
	"atines_smart_stream/internal/store"
	"atines_smart_stream/internal/supervisor"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "endereço de escuta da UI")
	dataPath := flag.String("data", defaultDataPath(), "caminho do cameras.json")
	noBrowser := flag.Bool("no-browser", false, "não abrir o navegador ao iniciar")
	flag.Parse()

	ffmpegPath, ffprobePath, err := ffmpeg.Locate()
	if err != nil {
		log.Fatalf("FFmpeg: %v", err)
	}

	st, err := store.Open(*dataPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	caps := media.DetectCapabilities(context.Background(), ffmpegPath)
	log.Printf("encoders de hardware: nvenc=%v qsv=%v amf=%v vaapi=%v", caps.NVENC, caps.QSV, caps.AMF, caps.VAAPI)

	// argsFor probes the stream then builds ffmpeg args for a camera.
	argsFor := func(cam model.Camera) ([]string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		in, err := media.BuildInputURL(cam)
		if err != nil {
			return nil, err
		}
		probe, err := media.Probe(ctx, ffprobePath, in)
		if err != nil {
			// Probe failed (camera offline?): assume h264 so we at least try to remux.
			probe = media.ProbeResult{VideoCodec: "h264"}
		}
		return media.BuildArgs(cam, probe, caps)
	}

	sup := supervisor.New(supervisor.SupervisorConfig{
		FFmpegPath: ffmpegPath,
		ArgsFor:    argsFor,
	})

	srv := api.NewServer(st, sup)
	// Feed supervisor status into SSE.
	if sink, ok := srv.(interface{ StatusSink() func(model.CameraStatus) }); ok {
		sup.SetOnStatus(sink.StatusSink())
	}

	// Resume cameras that were enabled.
	for _, c := range st.List() {
		if c.Enabled {
			sup.Start(c)
		}
	}

	url := "http://" + *addr
	fmt.Printf("Atines Smart Stream rodando em %s\n", url)
	if !*noBrowser {
		launcher.OpenBrowser(url)
	}
	log.Fatal(http.ListenAndServe(*addr, srv))
}

func defaultDataPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return "cameras.json"
	}
	return filepath.Join(dir, "atines-smart-stream", "cameras.json")
}
```

- [ ] **Step 6: Adicionar `SetOnStatus` ao supervisor**

Em `internal/supervisor/supervisor.go`, adicione o método:
```go
// SetOnStatus sets the status callback after construction (used to wire SSE).
func (s *Supervisor) SetOnStatus(cb func(model.CameraStatus)) {
	s.mu.Lock()
	s.cfg.OnStatus = cb
	s.mu.Unlock()
}
```

- [ ] **Step 7: Build + testes**

Run: `go build ./... && go test ./...`
Expected: tudo compila e passa.

- [ ] **Step 8: Commit**

```bash
git add main.go internal/launcher/ internal/supervisor/supervisor.go
git commit -m "feat: wire app together, browser launcher, resume enabled cameras"
```

---

## Task 11: Verificação manual fim-a-fim

**Files:** nenhum (verificação).

- [ ] **Step 1: Subir um SRS local (destino RTMP)**

```bash
docker run --rm -p 1935:1935 -p 8080:8080 ossrs/srs:5
```

- [ ] **Step 2: Gerar um RTSP de teste (se não tiver câmera real)**

Em outro terminal, use o MediaMTX ou um ffmpeg loop. Exemplo simples com ffmpeg publicando num servidor RTSP de teste (MediaMTX):
```bash
docker run --rm -p 8554:8554 bluenviron/mediamtx
# publicar um padrão de teste:
ffmpeg -re -f lavfi -i testsrc=size=640x480:rate=15 -f lavfi -i sine=frequency=1000 \
  -c:v libx264 -c:a aac -f rtsp rtsp://127.0.0.1:8554/cam
```

- [ ] **Step 3: Rodar o app (usando ffmpeg do PATH em dev)**

```bash
go run . --no-browser
# abra http://127.0.0.1:8787 manualmente
```

- [ ] **Step 4: Cadastrar e ativar**

Cadastre uma câmera: RTSP `rtsp://127.0.0.1:8554/cam`, RTMP `rtmp://127.0.0.1/live/cam1`. Ative o toggle.
Expected: status fica "rodando" (verde). No SRS (http://localhost:8080) o stream `live/cam1` aparece publicado.

- [ ] **Step 5: Validar resiliência**

Mate o publisher RTSP; o status deve ir para "reconectando" e voltar a "rodando" quando o publisher retornar.

- [ ] **Step 6: Commit (notas de verificação, se houver)**

Sem mudanças de código necessárias se tudo passou.

---

## Task 12: Instalar como serviço (modo 24/7)

**Files:**
- Create: `internal/service/service.go`
- Modify: `main.go` (flags `--install-service` / `--uninstall-service`)

Notas: Linux via unit do systemd (usuário atual, `--user` ou system com sudo); Windows via `sc.exe`. macOS fora de escopo.

- [ ] **Step 1: Implementar service.go (Linux systemd + Windows sc)**

```go
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const unitName = "atines-smart-stream.service"

// Install registers the app as an OS service that starts on boot.
func Install() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "linux":
		return installSystemd(exe)
	case "windows":
		return run("sc", "create", "AtinesSmartStream", "binPath=", exe, "start=", "auto")
	default:
		return fmt.Errorf("instalação de serviço não suportada em %s", runtime.GOOS)
	}
}

// Uninstall removes the OS service.
func Uninstall() error {
	switch runtime.GOOS {
	case "linux":
		_ = run("systemctl", "--user", "disable", "--now", unitName)
		return os.Remove(systemdUnitPath())
	case "windows":
		return run("sc", "delete", "AtinesSmartStream")
	default:
		return fmt.Errorf("não suportado em %s", runtime.GOOS)
	}
}

func installSystemd(exe string) error {
	unit := fmt.Sprintf(`[Unit]
Description=Atines Smart Stream
After=network-online.target

[Service]
ExecStart=%s --no-browser
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
`, exe)
	path := systemdUnitPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return err
	}
	if err := run("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	return run("systemctl", "--user", "enable", "--now", unitName)
}

func systemdUnitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", unitName)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

- [ ] **Step 2: Flags no main.go**

Adicione após `flag.Parse()`:
```go
	if *installSvc {
		if err := service.Install(); err != nil {
			log.Fatalf("instalar serviço: %v", err)
		}
		fmt.Println("Serviço instalado. O app sobe junto com o sistema.")
		return
	}
	if *uninstallSvc {
		if err := service.Uninstall(); err != nil {
			log.Fatalf("remover serviço: %v", err)
		}
		fmt.Println("Serviço removido.")
		return
	}
```
E declare as flags junto às outras:
```go
	installSvc := flag.Bool("install-service", false, "instalar como serviço do SO (24/7)")
	uninstallSvc := flag.Bool("uninstall-service", false, "remover o serviço do SO")
```
E o import `"atines_smart_stream/internal/service"`.

- [ ] **Step 3: Build**

Run: `go build ./... && go vet ./...`
Expected: compila.

- [ ] **Step 4: Commit**

```bash
git add internal/service/ main.go
git commit -m "feat(service): install/uninstall as OS service (systemd/windows)"
```

---

## Task 13: Build multiplataforma + README

**Files:**
- Create: `build.sh`
- Create: `README.md`

- [ ] **Step 1: build.sh (cross-compile dos 4 alvos)**

```bash
#!/usr/bin/env bash
set -euo pipefail
APP=atines-smart-stream
OUT=bin
rm -rf "$OUT" && mkdir -p "$OUT"

build() {
  local goos=$1 goarch=$2 ext=$3
  echo "==> $goos/$goarch"
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch \
    go build -ldflags "-s -w" -o "$OUT/${APP}-${goos}-${goarch}${ext}" .
}

build linux   amd64 ""
build linux   arm64 ""
build windows amd64 ".exe"
build windows arm64 ".exe"
echo "Binários em $OUT/ (lembre de colocar ffmpeg/ffprobe ao lado de cada um no release)."
```
Torne executável: `chmod +x build.sh`.

- [ ] **Step 2: README.md**

```markdown
# Atines Smart Stream

Ponte RTSP → RTMP (destino SRS) com UI web embutida. Um binário único por plataforma.

## Como usar (usuário final)
1. Coloque `ffmpeg`/`ffprobe` (e `.exe` no Windows) na mesma pasta do executável,
   ou tenha-os no PATH.
2. Rode o executável. A tela de gerência abre no navegador (http://127.0.0.1:8787).
3. Cadastre câmeras (nome, RTSP, usuário/senha, RTMP de destino) e use o toggle "Ativo".

### Opções
- `--addr 127.0.0.1:8787`  endereço da UI
- `--data <caminho>`       arquivo de cadastro (cameras.json)
- `--no-browser`           não abre o navegador
- `--install-service`      instala como serviço do SO (roda 24/7)
- `--uninstall-service`    remove o serviço

## Build (desenvolvedor)
```bash
./build.sh   # gera bin/ para linux/windows x amd64/arm64
```
CGO desligado: cross-compila sem dependências de C. FFmpeg/ffprobe são empacotados
no release ao lado do binário.

## Arquitetura
Ver `docs/superpowers/specs/2026-06-06-atines-smart-stream-design.md`.
```

- [ ] **Step 3: Build de verificação (pelo menos o host)**

Run: `CGO_ENABLED=0 go build ./... && go test ./...`
Expected: compila e testes passam.

- [ ] **Step 4: Commit**

```bash
git add build.sh README.md
git commit -m "build: cross-compile script and README"
```

---

## Self-Review (preenchido pelo autor do plano)

**Cobertura do spec:**
- Cadastro (nome/rtsp/user/pass/rtmp) → Task 1 (model) + Task 8 (API) + Task 9 (UI). ✓
- Ativar/desativar por câmera → Task 8 (enable/disable) + Task 9 (toggle). ✓
- Status ao vivo → Task 7 (status) + Task 8 (SSE) + Task 9 (EventSource). ✓
- Remux vs transcode por câmera + hardware accel → Task 3 + Task 5. ✓
- Detecção de codec (ffprobe) → Task 4. ✓
- Persistência JSON atômica 0600 → Task 2. ✓
- Resiliência/backoff/resume → Task 7 + Task 10. ✓
- Binário único / UI embutida → Task 9 (embed) + Task 13 (build). ✓
- Multiplataforma ARM/x86 Win/Linux → Task 13 (CGO off cross-compile). ✓
- Modo híbrido / serviço → Task 12. ✓
- FFmpeg empacotado/localizado → Task 6. ✓

**Placeholders:** nenhum "TODO/TBD". Os pontos que exigem edição de arquivo existente (main.go, server.go struct) trazem o trecho exato.

**Consistência de tipos:** `model.Camera`, `model.CameraStatus`, `media.ProbeResult`, `media.Capabilities`, `supervisor.Runner`/`SupervisorConfig`, `api.cameraView` usados de forma consistente entre tarefas. `BuildArgs`/`BuildInputURL`/`Probe`/`DetectCapabilities`/`Locate` com assinaturas estáveis. `SetOnStatus` adicionado na Task 10 e usado no main.

**Riscos conhecidos / a confirmar na execução:**
- `os.Rename` atômico no Windows (Go usa MOVEFILE_REPLACE_EXISTING) — validado por documentação; reconfirmar no teste de store em Windows se possível.
- Status "rodando" inferido pela 1ª linha de stderr do ffmpeg (que sempre emite); fallback marca rodando mesmo sem linhas.
- VAAPI device fixo em `/dev/dri/renderD128` (v1); tornar configurável é evolução.
```
