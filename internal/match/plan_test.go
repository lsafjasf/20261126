package match

import (
	"testing"

	"mediarelink/internal/media"
)

func expected() Expected {
	return Expected{
		MediaID:          "m01",
		Name:             "intro",
		OldPath:          "old_media/intro.mov",
		Codec:            "AVC1",
		DurationMS:       4000,
		FPSNum:           24,
		FPSDen:           1,
		FrameCount:       96,
		FrameHash:        "aaaa",
		AudioFingerprint: media.FingerprintHex(0xFFFFFFFFFFFFFFFF),
	}
}

func probe(path, codec string, dur int64, hash string, fp uint64) media.Probe {
	return media.Probe{
		Path:             path,
		Codec:            codec,
		DurationMS:       dur,
		FrameHash:        hash,
		AudioFingerprint: media.FingerprintHex(fp),
	}
}

func TestExactContentRelinks(t *testing.T) {
	plans := Plan([]Expected{expected()}, []media.Probe{
		probe("intro.mp4", "AVC1", 4000, "aaaa", 0xFFFFFFFFFFFFFFFF),
	})
	if plans[0].Status != StatusRelinked || plans[0].NewPath != "intro.mp4" {
		t.Fatalf("expected relink, got %+v", plans[0])
	}
	if plans[0].Confidence < autoConfidence {
		t.Fatalf("confidence too low: %.3f", plans[0].Confidence)
	}
}

func TestSameNameDecoyRejected(t *testing.T) {
	plans := Plan([]Expected{expected()}, []media.Probe{
		probe("intro.mov", "AVC1", 4000, "bbbb", 0x0F0F0F0F0F0F0F0F),
		probe("renamed/intro_master.mov", "AVC1", 4000, "aaaa", 0xFFFFFFFFFFFFFFFF),
	})
	p := plans[0]
	if p.Status != StatusRelinked || p.NewPath != "renamed/intro_master.mov" {
		t.Fatalf("decoy must not win: %+v", p)
	}
	var decoy *Candidate
	for i := range p.Candidates {
		if p.Candidates[i].Path == "intro.mov" {
			decoy = &p.Candidates[i]
		}
	}
	if decoy == nil || !decoy.Rejected {
		t.Fatalf("decoy not marked rejected: %+v", p.Candidates)
	}
}

func TestReencodeBlocks(t *testing.T) {
	plans := Plan([]Expected{expected()}, []media.Probe{
		probe("intro.mov", "AVC1", 4000, "bbbb", 0xFFFFFFFFFFFFFFFF),
	})
	p := plans[0]
	if p.Status != StatusBlocked || p.Reason != ReasonLowConfidence {
		t.Fatalf("re-encode must block with low_confidence, got %+v", p)
	}
}

func TestMissingBlocks(t *testing.T) {
	plans := Plan([]Expected{expected()}, []media.Probe{
		probe("unrelated.mov", "HEVC", 9000, "zzzz", 0x1111111111111111),
	})
	if plans[0].Status != StatusBlocked || plans[0].Reason != ReasonNoCandidate {
		t.Fatalf("expected no_candidate block, got %+v", plans[0])
	}
}

func TestConflictingClaimsBlock(t *testing.T) {
	exp1 := expected()
	exp2 := expected()
	exp2.MediaID = "m02"
	exp2.OldPath = "old_media/other.mov"
	plans := Plan([]Expected{exp1, exp2}, []media.Probe{
		probe("only_copy.mov", "AVC1", 4000, "aaaa", 0xFFFFFFFFFFFFFFFF),
	})
	for _, p := range plans {
		if p.Status != StatusBlocked || p.Reason != ReasonConflict {
			t.Fatalf("conflicting claims must block, got %+v", p)
		}
	}
}

func TestDurationDriftDecaysScore(t *testing.T) {
	exp := expected()
	near := probe("near.mov", "AVC1", 4040, "aaaa", 0xFFFFFFFFFFFFFFFF)
	confNear, _ := Score(exp, near)
	far := probe("far.mov", "AVC1", 8000, "aaaa", 0xFFFFFFFFFFFFFFFF)
	confFar, _ := Score(exp, far)
	if confNear <= confFar {
		t.Fatalf("duration drift not penalised: near %.3f far %.3f", confNear, confFar)
	}
}
