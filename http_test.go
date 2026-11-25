package captionhub

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPConnectionStates(t *testing.T) {
	h := NewHub()
	mustSwitch(t, h, "NEWS", "seg-1")
	mustAddCue(t, h, Cue{ID: "c1", StartMS: 0, EndMS: 5_000, Text: "hello"})
	if _, err := h.ConnectAt("conn-1", 1_000); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(h.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/connections")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"connection_id": "conn-1"`) || !strings.Contains(body, "hello") {
		t.Fatalf("response missing connection/cue: %s", body)
	}

	resp2, err := http.Get(server.URL + "/connections/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404", resp2.StatusCode)
	}
}
