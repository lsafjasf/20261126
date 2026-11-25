package clipmerge

import (
	"encoding/json"
	"fmt"
	"io"
)

// DecodeRequest strictly decodes one merge request JSON value from r.
func DecodeRequest(r io.Reader) (MergeRequest, error) {
	var req MergeRequest
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, fmt.Errorf("invalid merge request: %w", err)
	}
	return req, nil
}
