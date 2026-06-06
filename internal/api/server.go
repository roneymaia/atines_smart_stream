package api

import (
	"encoding/json"
	"net/http"

	"atines_smart_stream/internal/model"
	"atines_smart_stream/internal/store"
	"atines_smart_stream/internal/supervisor"
	"atines_smart_stream/internal/webui"
)

// cameraView is a camera merged with its runtime status for the UI.
type cameraView struct {
	model.Camera
	Status    model.Status `json:"status"`
	LastError string       `json:"last_error,omitempty"`
}

// Server holds API dependencies and routes.
type Server struct {
	store *store.Store
	sup   *supervisor.Supervisor
	hub   *sseHub
	mux   http.Handler
}

// NewServer wires the HTTP handler (API + SSE + embedded UI).
func NewServer(st *store.Store, sup *supervisor.Supervisor) *Server {
	s := &Server{store: st, sup: sup, hub: newSSEHub()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/cameras", s.handleList)
	mux.HandleFunc("POST /api/cameras", s.handleUpsert)
	mux.HandleFunc("DELETE /api/cameras/{id}", s.handleDelete)
	mux.HandleFunc("POST /api/cameras/{id}/enable", s.handleEnable)
	mux.HandleFunc("POST /api/cameras/{id}/disable", s.handleDisable)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.Handle("GET /", http.FileServer(http.FS(webui.FS())))
	s.mux = mux
	return s
}

// ServeHTTP dispatches to the internal mux.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// StatusSink returns a callback that feeds supervisor status into SSE.
func (s *Server) StatusSink() func(model.CameraStatus) { return s.hub.Broadcast }

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
	// If enabled, (re)start to apply any changes.
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
