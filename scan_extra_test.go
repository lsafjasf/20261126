package ac3select

import "testing"

func TestTruncatedTailIsReportedWithoutPanic(t *testing.T) {
	good := makeTestFrame(testFrameSpec{bsmod: BSModeCommentary, acmod: ACMod2_0})
	truncated := makeTestFrame(testFrameSpec{acmod: ACMod2_0})[:100]

	report := ParseBytes(concat(good, truncated))
	if report.FrameCount != 1 {
		t.Fatalf("FrameCount=%d, want complete frame; bad=%+v", report.FrameCount, report.BadFrames)
	}
	if report.BadCount != 1 || report.BadFrames[0].Offset != int64(len(good)) ||
		report.BadFrames[0].Length != 100 {
		t.Fatalf("BadFrames=%+v, want truncated 100-byte tail", report.BadFrames)
	}
}

func TestLeadingJunkAndDeclaredBadFrameAreBothReported(t *testing.T) {
	junk := []byte("not an AC-3 sync; just arbitrary prefix bytes!!!!")
	bad := makeTestFrame(testFrameSpec{acmod: ACMod1_0, couple: true})
	good := makeTestFrame(testFrameSpec{acmod: ACMod2_0})

	report := ParseBytes(concat(junk, bad, good))
	if report.FrameCount != 1 {
		t.Fatalf("FrameCount=%d, want one good frame", report.FrameCount)
	}
	if report.BadCount != 2 {
		t.Fatalf("BadCount=%d, want junk gap plus bad frame: %+v", report.BadCount, report.BadFrames)
	}
	if report.BadFrames[0].Offset != 0 || report.BadFrames[0].EndOffset != int64(len(junk)) {
		t.Fatalf("first bad range=%+v, want leading junk", report.BadFrames[0])
	}
	if report.BadFrames[1].Offset != int64(len(junk)) ||
		report.BadFrames[1].Length != int64(len(bad)) {
		t.Fatalf("second bad range=%+v, want declared bad frame", report.BadFrames[1])
	}
}

func TestBitstreamModeTrackCategories(t *testing.T) {
	cases := []struct {
		mode     BitstreamMode
		acmod    AudioCodingMode
		channels int
		main     bool
		comment  bool
		annot    bool
		role     string
	}{
		{BSModeCompleteMain, ACMod3_2, 6, true, false, false, "complete_main"},
		{BSModeMusicAndEffects, ACMod2_0, 2, true, false, false, "music_and_effects"},
		{BSModeCommentary, ACMod2_0, 2, false, true, true, "commentary"},
		{BSModeVisuallyImpaired, ACMod1_0, 1, false, false, true, "visually_impaired"},
		{BSModeHearingImpaired, ACMod2_0, 2, false, false, true, "hearing_impaired"},
		{BSModeDialogue, ACMod1_0, 1, false, false, true, "dialogue"},
		{BSModeVoiceOverOrKaraoke, ACMod1_0, 1, false, false, true, "voice_over_or_karaoke"},
		{BSModeVoiceOverOrKaraoke, ACMod3_2, 6, false, false, false, "karaoke"},
	}
	for _, tc := range cases {
		got := tc.mode.trackCategory(tc.acmod, tc.channels)
		if got.IsMain != tc.main || got.IsCommentary != tc.comment || got.IsAnnotation != tc.annot {
			t.Errorf("mode=%d flags main/comment/annot=%v/%v/%v, want %v/%v/%v",
				tc.mode, got.IsMain, got.IsCommentary, got.IsAnnotation, tc.main, tc.comment, tc.annot)
		}
		if got.Role != tc.role {
			t.Errorf("mode=%d role=%q, want %q", tc.mode, got.Role, tc.role)
		}
	}
}
