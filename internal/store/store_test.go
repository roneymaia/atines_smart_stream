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
