package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"testing"
)

// TestHTTPAcceptance_TwoBrowsersSwitchConcurrently drives two real HTTP
// clients against one server and shared catalog, switching simultaneously.
func TestHTTPAcceptance_TwoBrowsersSwitchConcurrently(t *testing.T) {
	ts, cat := newHarness(t)
	browserA := createSession(t, ts)
	browserB := createSession(t, ts)

	aBodies := []map[string]string{
		{"language": "zh"},
		{"language": "zh", "version": "director-cut"},
	}
	bBodies := []map[string]string{
		{"language": "en"},
		{"language": "en", "version": "sdh"},
	}

	var wg sync.WaitGroup
	worker := func(session string, bodies []map[string]string, wantLang string) {
		defer wg.Done()
		for i := 0; i < 40; i++ {
			_, dto := post(t, ts, "/api/sessions/"+session+"/switch", bodies[i%len(bodies)])
			if dto.SessionID != session {
				t.Errorf("session mismatch: %q != %q", dto.SessionID, session)
				return
			}
			if !strings.HasPrefix(dto.Language, wantLang) {
				t.Errorf("browser %s got language %q, want prefix %q (shared selector state)",
					session, dto.Language, wantLang)
				return
			}
			if err := checkDTOConsistency(cat, dto); err != nil {
				t.Errorf("inconsistent switch state: %v", err)
				return
			}
			dto = getState(t, ts, session)
			if err := checkDTOConsistency(cat, dto); err != nil {
				t.Errorf("inconsistent GET state: %v", err)
				return
			}
		}
	}

	wg.Add(2)
	go worker(browserA, aBodies, "zh")
	go worker(browserB, bBodies, "en")

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for _, session := range []string{browserA, browserB} {
		readers.Add(1)
		go func(session string) {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				dto := getState(t, ts, session)
				if err := checkDTOConsistency(cat, dto); err != nil {
					t.Errorf("reader inconsistency: %v", err)
					return
				}
			}
		}(session)
	}

	wg.Wait()
	close(stop)
	readers.Wait()
}

// TestHTTPAcceptance_PauseSwitchResume covers the transport acceptance path.
func TestHTTPAcceptance_PauseSwitchResume(t *testing.T) {
	ts, cat := newHarness(t)
	session := createSession(t, ts)

	if _, dto := post(t, ts, "/api/sessions/"+session+"/seek",
		map[string]int64{"position_ms": 5000}); dto.PositionMS != 5000 {
		t.Fatalf("seek position = %d", dto.PositionMS)
	}

	resp, paused := post(t, ts, "/api/sessions/"+session+"/pause", nil)
	if resp.StatusCode != http.StatusOK || paused.Status != "paused" {
		t.Fatalf("pause failed: code=%d status=%q", resp.StatusCode, paused.Status)
	}

	_, switched := post(t, ts, "/api/sessions/"+session+"/switch",
		map[string]string{"language": "en"})
	if switched.Status != "paused" {
		t.Fatalf("switch while paused changed status to %q", switched.Status)
	}
	if switched.PositionMS != 5000 {
		t.Fatalf("paused switch moved position to %d", switched.PositionMS)
	}
	if switched.Cue == nil || switched.Cue.ID != "en-1" {
		t.Fatalf("expected en-1 cue at 5s, got %+v", switched.Cue)
	}
	if switched.Cue.StartMS != 4500 || switched.Cue.EndMS != 7500 {
		t.Fatalf("90kHz rebase over HTTP = %d..%d, want 4500..7500",
			switched.Cue.StartMS, switched.Cue.EndMS)
	}
	if err := checkDTOConsistency(cat, switched); err != nil {
		t.Fatal(err)
	}

	_, resumed := post(t, ts, "/api/sessions/"+session+"/resume", nil)
	if resumed.Status != "playing" {
		t.Fatalf("resume status = %q", resumed.Status)
	}

	later := getState(t, ts, session)
	if later.Status != "playing" || later.PositionMS < 5000 {
		t.Fatalf("post-resume state = %+v", later)
	}
	if err := checkDTOConsistency(cat, later); err != nil {
		t.Fatal(err)
	}

	_, repaused := post(t, ts, "/api/sessions/"+session+"/pause", nil)
	if repaused.Status != "paused" || repaused.PositionMS < later.PositionMS {
		t.Fatalf("re-pause regressed: %+v vs later %d", repaused, later.PositionMS)
	}
}

func TestHTTP_MissingLanguageFallsBack(t *testing.T) {
	ts, _ := newHarness(t)
	session := createSession(t, ts)

	_, dto := post(t, ts, "/api/sessions/"+session+"/switch",
		map[string]string{"language": "ja"})
	if dto.Reason != "fallback_language" || dto.TrackID != "en-main" {
		t.Fatalf("fallback = reason %q track %q", dto.Reason, dto.TrackID)
	}
	if len(dto.Candidates) == 0 {
		t.Fatal("expected ordered candidates in response")
	}
}

func TestHTTP_UnknownSessionIs404(t *testing.T) {
	ts, _ := newHarness(t)
	resp, err := http.Get(ts.URL + "/api/sessions/nope")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
