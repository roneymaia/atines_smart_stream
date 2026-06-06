package supervisor

import (
	"bufio"
	"context"
	"os/exec"
	"sync"
	"time"

	"atines_smart_stream/internal/model"
)

// Runner executes one ffmpeg invocation, blocking until it exits. onLine is
// called for each stderr line while the process runs.
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
	// Drain stderr fully BEFORE Wait (required when using StderrPipe).
	scannerDone := make(chan struct{})
	go func() {
		defer close(scannerDone)
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if onLine != nil {
				onLine(sc.Text())
			}
		}
	}()
	<-scannerDone
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
	done   chan struct{}
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

// SetOnStatus sets the status callback after construction (used to wire SSE).
func (s *Supervisor) SetOnStatus(cb func(model.CameraStatus)) {
	s.mu.Lock()
	s.cfg.OnStatus = cb
	s.mu.Unlock()
}

// Start (re)launches the supervision loop for a camera. No-op if already running.
func (s *Supervisor) Start(cam model.Camera) {
	s.mu.Lock()
	if _, ok := s.running[cam.ID]; ok {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &proc{cancel: cancel, done: make(chan struct{})}
	s.running[cam.ID] = p
	s.mu.Unlock()
	go func() {
		s.loop(ctx, cam)
		close(p.done)
	}()
}

// Stop ends the supervision loop for a camera, blocking until it has fully
// exited (so the final status is deterministic).
func (s *Supervisor) Stop(id string) {
	s.mu.Lock()
	p, ok := s.running[id]
	if ok {
		delete(s.running, id)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	p.cancel()
	<-p.done
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
	// On exit (always due to Stop/cancellation), mark the camera stopped last.
	defer s.setStatus(model.CameraStatus{ID: cam.ID, Status: model.StatusStopped})

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
			// First output line means ffmpeg has connected and is streaming.
			if st := s.Status(cam.ID); st.Status != model.StatusRunning {
				s.setStatus(model.CameraStatus{ID: cam.ID, Status: model.StatusRunning, StartedAt: time.Now(), Restarts: restarts})
			}
		})

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
