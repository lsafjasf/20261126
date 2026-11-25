package clipmerge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func baseRequest() MergeRequest {
	return MergeRequest{
		Source: Source{ID: "media-1", Duration: 10000, MinTimelineDuration: 100},
		Base: Clip{
			ID: "clip-1", SourceID: "media-1", Revision: 7,
			InPoint: 1000, OutPoint: 9000, Speed: Speed{Numerator: 1, Denominator: 1},
		},
	}
}

func edit(id, client, property string, value interface{}) Edit {
	return Edit{ID: id, ClientID: client, BaseRevision: 7, Property: property, Value: value}
}

func decisionCode(t *testing.T, req MergeRequest) (*Report, string) {
	t.Helper()
	rep, err := Merge(req)
	if err != nil {
		t.Fatal(err)
	}
	return rep, rep.Decision.Code
}

// TestBoundaryGating verifies auto-merge only happens when the combined
// property values satisfy every media boundary.
func TestBoundaryGating(t *testing.T) {
	cases := []struct {
		name      string
		edits     []Edit
		wantCode  string
		wantCheck string // failed check name, when conflicting
	}{
		{
			name:     "in point beyond media",
			edits:    []Edit{edit("e1", "c1", PropertyInPoint, float64(10001))},
			wantCode: CodeMediaBoundaryViolation, wantCheck: CheckSourceInBounds,
		},
		{
			name:     "out point beyond media",
			edits:    []Edit{edit("e1", "c1", PropertyOutPoint, float64(20000))},
			wantCode: CodeMediaBoundaryViolation, wantCheck: CheckSourceOutBounds,
		},
		{
			name: "combined in/out invert the interval",
			edits: []Edit{
				edit("e1", "c1", PropertyInPoint, float64(9500)),
				edit("e2", "c2", PropertyOutPoint, float64(9000)),
			},
			wantCode: CodeMediaBoundaryViolation, wantCheck: CheckSourceIntervalOrder,
		},
		{
			name:     "zero speed",
			edits:    []Edit{edit("e1", "c1", PropertySpeed, map[string]interface{}{"numerator": 0.0, "denominator": 1.0})},
			wantCode: CodeMediaBoundaryViolation, wantCheck: CheckSpeedPositive,
		},
		{
			name: "combined span and speed below minimum timeline duration",
			edits: []Edit{
				edit("e1", "c1", PropertyInPoint, float64(8000)),
				edit("e2", "c2", PropertyOutPoint, float64(8100)),
				edit("e3", "c1", PropertySpeed, map[string]interface{}{"numerator": 5.0, "denominator": 1.0}),
			},
			wantCode: CodeMediaBoundaryViolation, wantCheck: CheckTimelineDurationMin,
		},
		{
			name: "timeline duration exactly at minimum still merges",
			edits: []Edit{
				edit("e1", "c1", PropertyInPoint, float64(1000)),
				edit("e2", "c2", PropertyOutPoint, float64(1100)),
			},
			wantCode: CodeAutoMerged,
		},
		{
			name: "each edit valid alone but invalid combined",
			edits: []Edit{
				edit("e1", "c1", PropertyInPoint, float64(8500)),
				edit("e2", "c2", PropertySpeed, map[string]interface{}{"numerator": 100.0, "denominator": 1.0}),
			},
			wantCode: CodeMediaBoundaryViolation, wantCheck: CheckTimelineDurationMin,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := baseRequest()
			req.Edits = tc.edits
			rep, code := decisionCode(t, req)
			if code != tc.wantCode {
				t.Fatalf("decision = %s, want %s", code, tc.wantCode)
			}
			if tc.wantCheck == "" {
				if rep.MergedClip == nil {
					t.Fatal("auto-merged report must carry mergedClip")
				}
				return
			}
			if rep.MergedClip != nil {
				t.Fatal("conflicted report must not carry mergedClip")
			}
			if len(rep.Conflicts) == 0 || rep.Conflicts[0].Reason != tc.wantCheck {
				t.Fatalf("expected dependency conflict on %s, got %+v", tc.wantCheck, rep.Conflicts)
			}
			if rep.Conflicts[0].Kind != KindDependencyViolation {
				t.Fatalf("kind = %s", rep.Conflicts[0].Kind)
			}
		})
	}
}

// TestStaleEditIgnored: an edit against the wrong base revision never
// participates and is reported under ignoredEdits.
func TestStaleEditIgnored(t *testing.T) {
	req := baseRequest()
	stale := edit("e-stale", "c1", PropertyOutPoint, float64(100))
	stale.BaseRevision = 6
	req.Edits = []Edit{stale, edit("e1", "c2", PropertyInPoint, float64(1200))}
	rep, code := decisionCode(t, req)
	if code != CodeAutoMerged {
		t.Fatalf("code = %s", code)
	}
	if len(rep.IgnoredEdits) != 1 || rep.IgnoredEdits[0].ID != "e-stale" {
		t.Fatalf("ignoredEdits = %+v", rep.IgnoredEdits)
	}
	if rep.IgnoredEdits[0].Disposition != DispositionStaleIgnored {
		t.Fatalf("disposition = %s", rep.IgnoredEdits[0].Disposition)
	}
	if rep.MergedClip.OutPoint != 9000 {
		t.Fatalf("stale edit leaked into merge: outPoint = %d", rep.MergedClip.OutPoint)
	}
}

// TestConcurrentAgreement: two clients proposing the same value collapse to
// one candidate and auto-merge.
func TestConcurrentAgreement(t *testing.T) {
	req := baseRequest()
	req.Edits = []Edit{
		edit("e1", "c1", PropertyOutPoint, float64(8500)),
		edit("e2", "c2", PropertyOutPoint, float64(8500)),
	}
	rep, code := decisionCode(t, req)
	if code != CodeAutoMerged {
		t.Fatalf("code = %s", code)
	}
	res := rep.PropertyResolutions[1]
	if len(res.Candidates) != 1 || len(res.Candidates[0].EditIDs) != 2 {
		t.Fatalf("candidates = %+v", res.Candidates)
	}
	if rep.MergedClip.OutPoint != 8500 {
		t.Fatalf("outPoint = %d", rep.MergedClip.OutPoint)
	}
}

// TestNoOpEdit: an edit restating the base value proposes nothing.
func TestNoOpEdit(t *testing.T) {
	req := baseRequest()
	req.Edits = []Edit{edit("e1", "c1", PropertyInPoint, float64(1000))}
	rep, code := decisionCode(t, req)
	if code != CodeAutoMerged {
		t.Fatalf("code = %s", code)
	}
	if len(rep.PropertyResolutions[0].Candidates) != 0 {
		t.Fatalf("no-op edit produced a candidate")
	}
}

// TestMultipleFailures: every failed boundary check yields its own
// dependency conflict, in canonical check order.
func TestMultipleFailures(t *testing.T) {
	req := baseRequest()
	req.Edits = []Edit{
		edit("e1", "c1", PropertyInPoint, float64(9900)),
		edit("e2", "c2", PropertyOutPoint, float64(9950)),
		edit("e3", "c1", PropertySpeed, map[string]interface{}{"numerator": 100.0, "denominator": 1.0}),
	}
	rep, code := decisionCode(t, req)
	if code != CodeMediaBoundaryViolation {
		t.Fatalf("code = %s", code)
	}
	if len(rep.Conflicts) != 1 || rep.Conflicts[0].Reason != CheckTimelineDurationMin {
		t.Fatalf("conflicts = %+v", rep.Conflicts)
	}
	if !strings.Contains(rep.Decision.Reason, CheckTimelineDurationMin) {
		t.Fatalf("reason = %s", rep.Decision.Reason)
	}
}

// TestValidationErrors rejects malformed requests before merging.
func TestValidationErrors(t *testing.T) {
	req := baseRequest()
	req.Edits = []Edit{edit("e1", "c1", "frameRate", float64(1))}
	if _, err := Merge(req); err == nil {
		t.Fatal("unknown property accepted")
	}
	req = baseRequest()
	req.Edits = []Edit{
		edit("e1", "c1", PropertyInPoint, float64(1)),
		edit("e1", "c2", PropertyInPoint, float64(2)),
	}
	if _, err := Merge(req); err == nil {
		t.Fatal("duplicate edit id accepted")
	}
	req = baseRequest()
	req.Edits = []Edit{edit("e1", "c1", PropertyInPoint, float64(1.5))}
	if _, err := Merge(req); err == nil {
		t.Fatal("non-integral point accepted")
	}
}

// TestHTTPHandler exercises the service endpoint end to end.
func TestHTTPHandler(t *testing.T) {
	body := `{"source":{"id":"media-1","duration":10000,"minTimelineDuration":100},
"base":{"id":"clip-1","sourceId":"media-1","revision":7,"inPoint":1000,"outPoint":9000,"speed":{"numerator":1,"denominator":1}},
"edits":[{"id":"e1","clientId":"c1","baseRevision":7,"property":"inPoint","value":1200}]}`
	srv := httptest.NewServer(NewHandler())
	defer srv.Close()

	post := func(payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/merges", strings.NewReader(payload))
		rec := httptest.NewRecorder()
		NewHandler().ServeHTTP(rec, req)
		return rec
	}
	rec := post(body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var rep Report
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Decision.Code != CodeAutoMerged || rep.ReportID == "" {
		t.Fatalf("unexpected report: %+v", rep.Decision)
	}
	// Same request, same reportId: the service is reproducible.
	rec2 := post(body)
	if rec.Body.String() != rec2.Body.String() {
		t.Fatal("same HTTP request produced different reports")
	}
	rec3 := post(`{"source":`)
	if rec3.Code != http.StatusBadRequest {
		t.Fatalf("bad JSON status = %d", rec3.Code)
	}
	_ = srv
}
