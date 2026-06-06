package supervisor

import (
	"context"
	"sync"
	"testing"
	"time"

	"atines_smart_stream/internal/model"
)

// fakeRunner simulates ffmpeg: it notifies each run, emits one stderr line
// (so the supervisor marks the camera "running"), then returns a programmed
// result. The runs notification is non-blocking so a fast restart loop never
// deadlocks on a full channel.
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
	select {
	case f.runs <- n:
	default:
	}
	if onLine != nil {
		onLine("frame=    1 fps=15 q=-1.0")
	}
	err := f.result(n)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func TestSupervisorRestartsOnExit(t *testing.T) {
	fr := &fakeRunner{runs: make(chan int, 8), result: func(int) error { return nil }}
	sup := New(SupervisorConfig{
		Runner:      fr,
		ArgsFor:     func(model.Camera) ([]string, error) { return []string{"-i", "x"}, nil },
		BaseBackoff: time.Millisecond,
		MaxBackoff:  2 * time.Millisecond,
	})
	cam := model.Camera{ID: "c1", Name: "C", RTSPURL: "rtsp://h/s", RTMPURL: "rtmp://h/a"}
	sup.Start(cam)

	// Expect at least 3 runs (it restarts after each clean exit).
	for i := 0; i < 3; i++ {
		select {
		case <-fr.runs:
		case <-time.After(2 * time.Second):
			t.Fatalf("expected run #%d", i+1)
		}
	}
	sup.Stop("c1")
	if st := sup.Status("c1"); st.Status != model.StatusStopped {
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

func TestSupervisorArgsErrorSetsErrorStatus(t *testing.T) {
	fr := &fakeRunner{runs: make(chan int, 4), result: func(int) error { return nil }}
	sawError := make(chan struct{}, 1)
	sup := New(SupervisorConfig{
		Runner:      fr,
		ArgsFor:     func(model.Camera) ([]string, error) { return nil, context.DeadlineExceeded },
		BaseBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
		OnStatus: func(st model.CameraStatus) {
			if st.Status == model.StatusError {
				select {
				case sawError <- struct{}{}:
				default:
				}
			}
		},
	})
	sup.Start(model.Camera{ID: "c1"})
	select {
	case <-sawError:
	case <-time.After(time.Second):
		t.Fatal("expected error status")
	}
	sup.Stop("c1")
}
