package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"subselect"
)

func apiCatalog() *subselect.Catalog {
	cat, err := subselect.NewCatalog(
		subselect.Track{
			ID: "zh-main", Language: "zh", Version: "theatrical", Default: true,
			TimeBase: subselect.TimeBase{Timescale: 1000},
			Cues: []subselect.Cue{
				{ID: "zh-1", Start: 4000, End: 7000, Text: "走吧"},
			},
		},
		subselect.Track{
			ID: "zh-dir", Language: "zh", Version: "director-cut",
			TimeBase: subselect.TimeBase{Timescale: 1000, Offset: 500 * time.Millisecond},
			Cues: []subselect.Cue{
				{ID: "zhd-1", Start: 4600, End: 7600, Text: "离开这"},
			},
		},
		subselect.Track{
			ID: "en-main", Language: "en", Version: "theatrical",
			TimeBase: subselect.TimeBase{Timescale: 90000},
			Cues: []subselect.Cue{
				{ID: "en-1", Start: 405000, End: 675000, Text: "Let's go"},
			},
		},
		subselect.Track{
			ID: "en-sdh", Language: "en", Version: "sdh", HearingImpaired: true,
			Kind:     subselect.KindCaptions,
			TimeBase: subselect.TimeBase{Timescale: 48000},
			Cues: []subselect.Cue{
				{ID: "ensdh-1", Start: 216000, End: 348000, Text: "[STEPS] Go"},
			},
		},
	)
	if err != nil {
		panic(err)
	}
	return cat
}

func newHarness(t *testing.T) (*httptest.Server, *subselect.Catalog) {
	t.Helper()
	cat := apiCatalog()
	sel := subselect.NewSelector(cat, subselect.SelectorConfig{FallbackLanguages: []string{"en"}})
	hub := subselect.NewHub(sel)
	ts := httptest.NewServer(NewServer(hub).Handler())
	t.Cleanup(ts.Close)
	return ts, cat
}

func createSession(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	resp, err := http.Post(ts.URL+"/api/sessions", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var created sessionCreatedDTO
	decodeBody(t, resp, &created)
	if created.SessionID == "" {
		t.Fatal("empty session id")
	}
	return created.SessionID
}

func post(t *testing.T, ts *httptest.Server, path string, body any) (*http.Response, stateDTO) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	resp, err := http.Post(ts.URL+path, "application/json", reader)
	if err != nil {
		t.Fatal(err)
	}
	var dto stateDTO
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(data) > 0 {
		if err := json.Unmarshal(data, &dto); err != nil {
			t.Fatalf("decode %s: %v body=%s", path, err, data)
		}
	}
	return resp, dto
}

func getState(t *testing.T, ts *httptest.Server, id string) stateDTO {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/sessions/%s", ts.URL, id))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var dto stateDTO
	decodeBody(t, resp, &dto)
	return dto
}

func decodeBody(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}

// checkDTOConsistency independently re-derives cue timing from the catalog.
func checkDTOConsistency(cat *subselect.Catalog, dto stateDTO) error {
	if dto.SessionID == "" {
		return fmt.Errorf("missing session id")
	}
	if dto.Off || dto.TrackID == "" {
		return nil
	}
	track, ok := cat.Get(dto.TrackID)
	if !ok {
		return fmt.Errorf("unknown track %q", dto.TrackID)
	}
	if dto.TimeBase == nil || dto.TimeBase.Timescale != track.TimeBase.Timescale {
		return fmt.Errorf("timebase dto %+v does not match track %q", dto.TimeBase, track.ID)
	}
	if dto.Cue == nil {
		return nil
	}
	var src *subselect.Cue
	for i := range track.Cues {
		if track.Cues[i].ID == dto.Cue.ID {
			src = &track.Cues[i]
			break
		}
	}
	if src == nil {
		return fmt.Errorf("cue %q not found in track %q", dto.Cue.ID, track.ID)
	}
	wantStart := track.TimeBase.ToDuration(src.Start).Milliseconds()
	wantEnd := track.TimeBase.ToDuration(src.End).Milliseconds()
	if dto.Cue.StartMS != wantStart || dto.Cue.EndMS != wantEnd {
		return fmt.Errorf("cue %q timing (%d,%d) not rebased from %q timebase, want (%d,%d)",
			dto.Cue.ID, dto.Cue.StartMS, dto.Cue.EndMS, track.ID, wantStart, wantEnd)
	}
	if dto.PositionMS < dto.Cue.StartMS || dto.PositionMS >= dto.Cue.EndMS {
		return fmt.Errorf("cue [%d,%d) does not cover position %d",
			dto.Cue.StartMS, dto.Cue.EndMS, dto.PositionMS)
	}
	return nil
}
