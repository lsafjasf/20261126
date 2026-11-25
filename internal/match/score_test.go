package match

import (
	"testing"

	"proxyrelink/internal/media"
)

func pair() (media.MediaInfo, media.MediaInfo) {
	p := media.MediaInfo{
		ID: "p1", Path: "proxies/shot01_proxy.mov", Kind: "proxy",
		DurationMs: 8000, FrameCount: 200, FPSNum: 25, FPSDen: 1,
		Width: 960, Height: 540,
		FirstFrameHash:   "3a7c91e24f08b6d1",
		AudioFingerprint: "9f2c41aa07b3d5e811cc6a9042f7e0b1",
	}
	o := media.MediaInfo{
		ID: "o1", Path: "originals/shot01.mov", Kind: "original",
		DurationMs: 8000, FrameCount: 200, FPSNum: 25, FPSDen: 1,
		Width: 1920, Height: 1080,
		FirstFrameHash:   "c85f03a1d97b4e26",
		AudioFingerprint: "8f0c51ab17a3d5e811cc6a9042f7e0b1",
	}
	return p, o
}

func TestScorePairSameDurationDifferentHashes(t *testing.T) {
	p, o := pair()
	r := ScorePair(p, o)
	if r.Score < 0.7 {
		t.Fatalf("expected strong score for same duration/frames/fps, got %.3f", r.Score)
	}
	if r.Confidence <= 0 || r.Confidence > 1 {
		t.Fatalf("confidence out of range: %.3f", r.Confidence)
	}
	var sawDuration, sawAudio, sawFirstFrame bool
	for _, e := range r.Evidence {
		switch e.Dimension {
		case "duration":
			sawDuration = true
			if e.Score != 1 {
				t.Fatalf("identical duration should score 1, got %.3f", e.Score)
			}
		case "audio_fingerprint":
			sawAudio = true
			if !e.Available {
				t.Fatal("audio evidence should be available")
			}
		case "first_frame_hash":
			sawFirstFrame = true
			if e.Weight > 0.15 {
				t.Fatalf("first-frame hash must stay low-weight for re-encodes, got %.2f", e.Weight)
			}
		}
	}
	if !sawDuration || !sawAudio || !sawFirstFrame {
		t.Fatal("missing expected evidence dimensions")
	}
}

func TestScorePairMismatch(t *testing.T) {
	p, o := pair()
	o.DurationMs = 12000
	o.FrameCount = 300
	o.AudioFingerprint = "deadbeef00112233445566778899aabb"
	o.Path = "originals/other.mov"
	r := ScorePair(p, o)
	if r.Score > 0.6 {
		t.Fatalf("mismatched pair should score low, got %.3f", r.Score)
	}
	if r.Verdict == "auto" {
		t.Fatalf("mismatched pair must not auto-match")
	}
}

func TestHexHammingSimilarity(t *testing.T) {
	if sim, ok := hexHammingSimilarity("ff00", "ff00"); !ok || sim != 1 {
		t.Fatalf("identical hashes: sim=%v ok=%v", sim, ok)
	}
	if sim, ok := hexHammingSimilarity("ff", "00"); !ok || sim != 0 {
		t.Fatalf("opposite hashes: sim=%v ok=%v", sim, ok)
	}
	if _, ok := hexHammingSimilarity("", "ff"); ok {
		t.Fatal("empty input must be unavailable")
	}
	if _, ok := hexHammingSimilarity("ff", "ff00"); ok {
		t.Fatal("length mismatch must be unavailable")
	}
}

func TestNameSimilarityStripsProxySuffix(t *testing.T) {
	if s := nameSimilarity("proxies/shot01_proxy.mov", "originals/shot01.mov"); s != 1 {
		t.Fatalf("proxy suffix should normalize away, got %.3f", s)
	}
	if s := nameSimilarity("a/shot01.mov", "b/shot02.mov"); s >= 1 {
		t.Fatalf("different clips must not score 1, got %.3f", s)
	}
}

func TestDecisionsLockRejectClear(t *testing.T) {
	d := &Decisions{}
	if err := d.Lock("p1", "o1", 0.9); err != nil {
		t.Fatal(err)
	}
	if err := d.Reject("p1", "o1", 0.9); err == nil {
		t.Fatal("rejecting a locked pair must fail")
	}
	if !d.LockedProxySet()["p1"] || !d.LockedOriginalSet()["o1"] {
		t.Fatal("locked sets should include the pair")
	}
	d.Clear("p1", "o1")
	if err := d.Reject("p1", "o1", 0.4); err != nil {
		t.Fatal(err)
	}
	if !d.IsRejected("p1", "o1") {
		t.Fatal("pair should be rejected")
	}
	if err := d.Lock("p1", "o1", 0.9); err == nil {
		t.Fatal("locking a rejected pair must fail")
	}
	d.Clear("p1", "o1")
	if err := d.Lock("p1", "o1", 0.9); err != nil {
		t.Fatal(err)
	}
}

func TestRankedCandidatesExcludesLockedAndRejected(t *testing.T) {
	ix := media.Index{Media: []media.MediaInfo{
		{ID: "p1", Kind: "proxy", DurationMs: 8000, FrameCount: 200, FPSNum: 25, FPSDen: 1, Width: 960, Height: 540, Path: "x/a_proxy.mov"},
		{ID: "p2", Kind: "proxy", DurationMs: 9000, FrameCount: 225, FPSNum: 25, FPSDen: 1, Width: 960, Height: 540, Path: "x/b_proxy.mov"},
		{ID: "o1", Kind: "original", DurationMs: 8000, FrameCount: 200, FPSNum: 25, FPSDen: 1, Width: 1920, Height: 1080, Path: "y/a.mov"},
		{ID: "o2", Kind: "original", DurationMs: 9000, FrameCount: 225, FPSNum: 25, FPSDen: 1, Width: 1920, Height: 1080, Path: "y/b.mov"},
	}}
	d := &Decisions{}
	cands := RankedCandidates(ix, d)
	if len(cands) != 4 {
		t.Fatalf("expected 4 candidates, got %d", len(cands))
	}
	if err := d.Lock("p1", "o1", 0.99); err != nil {
		t.Fatal(err)
	}
	if err := d.Reject("p2", "o1", 0.1); err != nil {
		t.Fatal(err)
	}
	cands = RankedCandidates(ix, d)
	if len(cands) != 1 || cands[0].ProxyID != "p2" || cands[0].OriginalID != "o2" {
		t.Fatalf("expected only p2->o2 to remain, got %+v", cands)
	}
}
