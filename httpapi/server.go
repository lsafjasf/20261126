// Package httpapi exposes per-browser subtitle sessions over HTTP so the
// two-browser concurrent-switch scenario can be exercised end to end.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"subselect"
)

// Server wires a subselect.Hub to an http.Handler.
type Server struct {
	hub *subselect.Hub
	mux *http.ServeMux
}

// NewServer creates the API server.
func NewServer(hub *subselect.Hub) *Server {
	s := &Server{hub: hub, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler returns the routed handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/sessions", s.createSession)
	s.mux.HandleFunc("GET /api/sessions/{id}", s.getState)
	s.mux.HandleFunc("POST /api/sessions/{id}/switch", s.switchTrack)
	s.mux.HandleFunc("POST /api/sessions/{id}/pause", s.pause)
	s.mux.HandleFunc("POST /api/sessions/{id}/resume", s.resume)
	s.mux.HandleFunc("POST /api/sessions/{id}/play", s.play)
	s.mux.HandleFunc("POST /api/sessions/{id}/seek", s.seek)
}

func (s *Server) createSession(w http.ResponseWriter, _ *http.Request) {
	p := s.hub.Create()
	writeJSON(w, http.StatusCreated, sessionCreatedDTO{SessionID: p.ID()})
}

func (s *Server) getState(w http.ResponseWriter, r *http.Request) {
	p, ok := s.hub.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, toStateDTO(p.Snapshot()))
}

func (s *Server) switchTrack(w http.ResponseWriter, r *http.Request) {
	p, ok := s.hub.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var req switchRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
	}

	criteria := subselect.Criteria{
		Language:        req.Language,
		TrackID:         req.TrackID,
		Version:         req.Version,
		Kind:            req.Kind,
		Forced:          req.Forced,
		HearingImpaired: req.HearingImpaired,
	}

	result, err := p.Switch(r.Context(), criteria)
	dto := toStateDTO(result.View)
	for _, le := range result.LoadErrors {
		dto.LoadErrors = append(dto.LoadErrors, loadErrorDTO{TrackID: le.TrackID, Error: le.Err.Error()})
	}

	switch {
	case errors.Is(err, subselect.ErrStaleSwitch):
		dto.Stale = true
		writeJSON(w, http.StatusConflict, dto)
	case errors.Is(err, subselect.ErrNoAvailableTrack):
		// Terminal fallback: subtitles off, details in load_errors.
		writeJSON(w, http.StatusOK, dto)
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, http.StatusOK, dto)
	}
}

func (s *Server) pause(w http.ResponseWriter, r *http.Request) {
	p, ok := s.hub.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	p.Pause()
	writeJSON(w, http.StatusOK, toStateDTO(p.Snapshot()))
}

func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	p, ok := s.hub.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	p.Resume()
	writeJSON(w, http.StatusOK, toStateDTO(p.Snapshot()))
}

func (s *Server) play(w http.ResponseWriter, r *http.Request) {
	p, ok := s.hub.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	p.Play()
	writeJSON(w, http.StatusOK, toStateDTO(p.Snapshot()))
}

func (s *Server) seek(w http.ResponseWriter, r *http.Request) {
	p, ok := s.hub.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var req seekRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	p.Seek(time.Duration(req.PositionMS) * time.Millisecond)
	writeJSON(w, http.StatusOK, toStateDTO(p.Snapshot()))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
