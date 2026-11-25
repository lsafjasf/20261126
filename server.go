package clipmerge

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// NewHandler returns the HTTP API of the merge service:
//
//	GET  /healthz     -> {"status":"ok"}
//	POST /v1/merges   -> MergeRequest JSON in, Report JSON out
//
// The handler holds no state; every response is a pure function of its
// request body.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("POST /v1/merges", func(w http.ResponseWriter, r *http.Request) {
		var req MergeRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid merge request: %w", err))
			return
		}
		rep, err := Merge(req)
		if err != nil {
			writeJSONError(w, http.StatusUnprocessableEntity, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(rep.Marshal())
	})
	return mux
}

func writeJSONError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":%q}`, err.Error())
}
