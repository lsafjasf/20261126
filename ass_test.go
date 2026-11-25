package sdh

import (
	"strings"
	"testing"
)

func TestRenderASSContainsStylesAndEvents(t *testing.T) {
	p := Split(loadFixtureCues(t))
	data := RenderASS(p, DefaultASSOptions())
	out := string(data)

	for _, section := range []string{"[Script Info]", "[V4+ Styles]", "[Events]"} {
		if !strings.Contains(out, section) {
			t.Fatalf("ASS missing section %s", section)
		}
	}
	for _, style := range []string{
		"Style: SDH_Dialogue_L0",
		"Style: SDH_Voiceover_L0",
		"Style: SDH_Lyric_L0",
		"Style: SDH_Speaker_L",
		"Style: SDH_Sound_L0",
		"Style: SDH_Music_L0",
	} {
		if !strings.Contains(out, style) {
			t.Errorf("ASS missing style prefix %q", style)
		}
	}
	for _, text := range []string{"你听到了吗？", "I still remember", "雷声轰鸣", "敲门声"} {
		if !strings.Contains(out, text) {
			t.Errorf("ASS missing event text %q", text)
		}
	}
}

func TestRenderASSLaneMarginsDiffer(t *testing.T) {
	p := Split(loadFixtureCues(t))
	out := string(RenderASS(p, DefaultASSOptions()))
	// Two top-band sound lanes must get distinct MarginV values.
	if !strings.Contains(out, "SDH_Sound_L0,") || !strings.Contains(out, "SDH_Sound_L1,") {
		t.Fatalf("expected two sound lane styles")
	}
	// The L1 style line must carry a larger MarginV than L0 (last int before
	// Encoding). Compare the full style lines.
	var l0, l1 string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Style: SDH_Sound_L0,") {
			l0 = line
		}
		if strings.HasPrefix(line, "Style: SDH_Sound_L1,") {
			l1 = line
		}
	}
	if l0 == "" || l1 == "" {
		t.Fatalf("missing sound styles:\n%s", out)
	}
	fields0 := strings.Split(l0, ",")
	fields1 := strings.Split(l1, ",")
	if fields0[len(fields0)-2] == fields1[len(fields1)-2] {
		t.Fatalf("lane margins should differ:\n%s\n%s", l0, l1)
	}
}

func TestASSEscape(t *testing.T) {
	got := assEscape("line1\nline2 {tag}")
	if got != `line1\Nline2 (tag)` {
		t.Fatalf("assEscape = %q", got)
	}
}
