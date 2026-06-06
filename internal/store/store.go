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

// Upsert validates, assigns an ID if new, applies defaults, persists, and
// returns the stored camera.
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

// saveLocked atomically writes the registry with 0600 perms. Caller holds the
// write lock.
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
