package captionhub

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Handler exposes read-only caption state endpoints:
//
//	GET /snapshot
//	GET /connections
//	GET /connections/{id}
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /snapshot", h.handleSnapshot)
	mux.HandleFunc("GET /connections", h.handleConnections)
	mux.HandleFunc("GET /connections/{id}", h.handleConnection)
	return mux
}

func (h *Hub) handleSnapshot(w http.ResponseWriter, _ *http.Request) {
	writeJSONResponse(w, http.StatusOK, h.Snapshot())
}

func (h *Hub) handleConnections(w http.ResponseWriter, _ *http.Request) {
	writeJSONResponse(w, http.StatusOK, h.States())
}

func (h *Hub) handleConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	state, err := h.State(id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrConnectionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, state)
}

func writeJSONResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSONResponse(w, status, map[string]string{"error": err.Error()})
}
