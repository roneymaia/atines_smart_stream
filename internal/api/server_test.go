package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"atines_smart_stream/internal/model"
	"atines_smart_stream/internal/store"
	"atines_smart_stream/internal/supervisor"
)

type noopRunner struct{}

func (noopRunner) Run(ctx context.Context, name string, args []string, onLine func(string)) error {
	return nil
}

func newTestServer(t *testing.T) (*Server, *store.Store, *supervisor.Supervisor) {
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
	if list[0].Status != model.StatusStopped {
		t.Fatalf("expected stopped status, got %q", list[0].Status)
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

func TestServesUIIndex(t *testing.T) {
	srv, _, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("index code = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Fatal("expected a content type for index")
	}
}
