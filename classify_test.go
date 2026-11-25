package sdh

import (
	"reflect"
	"testing"
)

func classifyLine(t *testing.T, line string) []Segment {
	t.Helper()
	return ClassifyCue(Cue{ID: "t", Text: line})
}

func roleSequence(segs []Segment) []Role {
	roles := make([]Role, 0, len(segs))
	for _, seg := range segs {
		roles = append(roles, seg.Role)
	}
	return roles
}

func TestClassifySemanticRoles(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		wantRoles []Role
		wantPlain string
	}{
		{
			name:      "cn sound brackets",
			line:      "[雷声轰鸣]",
			wantRoles: []Role{RoleSound},
			wantPlain: "",
		},
		{
			name:      "cn sound lens brackets",
			line:      "【敲门声】",
			wantRoles: []Role{RoleSound},
			wantPlain: "",
		},
		{
			name:      "paren sound",
			line:      "(door creaks open)",
			wantRoles: []Role{RoleSound},
			wantPlain: "",
		},
		{
			name:      "all caps sound",
			line:      "[DOOR SLAMS]",
			wantRoles: []Role{RoleSound},
			wantPlain: "",
		},
		{
			name:      "cn speaker colon",
			line:      "张三：你听到了吗？",
			wantRoles: []Role{RoleSpeaker, RoleDialogue},
			wantPlain: "你听到了吗？",
		},
		{
			name:      "en speaker colon",
			line:      "JOHN: Hello there.",
			wantRoles: []Role{RoleSpeaker, RoleDialogue},
			wantPlain: "Hello there.",
		},
		{
			name:      "bracket speaker en",
			line:      "[John] How are you?",
			wantRoles: []Role{RoleSpeaker, RoleDialogue},
			wantPlain: "How are you?",
		},
		{
			name:      "bracket speaker cn",
			line:      "（张三）你好呀。",
			wantRoles: []Role{RoleSpeaker, RoleDialogue},
			wantPlain: "你好呀。",
		},
		{
			name:      "inline sound",
			line:      "李四：[笑声] 真的吗？",
			wantRoles: []Role{RoleSpeaker, RoleSound, RoleDialogue},
			wantPlain: "真的吗？",
		},
		{
			name:      "inline sound splits words",
			line:      "Hello [laughs] my friend.",
			wantRoles: []Role{RoleDialogue, RoleSound},
			wantPlain: "Hello my friend.",
		},
		{
			name:      "music description",
			line:      "[upbeat jazz music]",
			wantRoles: []Role{RoleMusic},
			wantPlain: "",
		},
		{
			name:      "lyrics",
			line:      "♪ The rain kept falling down ♪",
			wantRoles: []Role{RoleLyric},
			wantPlain: "♪ The rain kept falling down ♪",
		},
		{
			name:      "lyrics double note",
			line:      "♫ Shine on, shine on ♪",
			wantRoles: []Role{RoleLyric},
			wantPlain: "♫ Shine on, shine on ♪",
		},
		{
			name:      "english VO",
			line:      "JOHN (V.O.): I remember.",
			wantRoles: []Role{RoleSpeaker, RoleVoiceOver},
			wantPlain: "I remember.",
		},
		{
			name:      "english OS maps to voiceover layer",
			line:      "JOHN (O.S.): Can you hear?",
			wantRoles: []Role{RoleSpeaker, RoleVoiceOver},
			wantPlain: "Can you hear?",
		},
		{
			name:      "cn VO bracket",
			line:      "（画外音）列车进站了。",
			wantRoles: []Role{RoleVoiceOver},
			wantPlain: "列车进站了。",
		},
		{
			name:      "narrator name implies VO",
			line:      "旁白：很久很久以前。",
			wantRoles: []Role{RoleSpeaker, RoleVoiceOver},
			wantPlain: "很久很久以前。",
		},
		{
			name:      "dash dialogue keeps marker",
			line:      "- MARY: Wait!",
			wantRoles: []Role{RoleSpeaker, RoleDialogue},
			wantPlain: "- Wait!",
		},
		{
			name:      "plain dialogue untouched",
			line:      "我们走吧。",
			wantRoles: []Role{RoleDialogue},
			wantPlain: "我们走吧。",
		},
		{
			name:      "time-like text is not speaker",
			line:      "The time is 12:30 now.",
			wantRoles: []Role{RoleDialogue},
			wantPlain: "The time is 12:30 now.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			segs := classifyLine(t, tc.line)
			gotRoles := roleSequence(segs)
			if !reflect.DeepEqual(gotRoles, tc.wantRoles) {
				t.Fatalf("roles = %v, want %v\nsegments: %+v", gotRoles, tc.wantRoles, segs)
			}
			plain := StripSDHLine(tc.line, DefaultPlainOptions())
			if plain != tc.wantPlain {
				t.Fatalf("plain = %q, want %q", plain, tc.wantPlain)
			}
		})
	}
}

func TestSpeakerAndVOAttributes(t *testing.T) {
	segs := classifyLine(t, "JOHN (V.O.): Hello.")
	var speaker, spoken *Segment
	for i := range segs {
		switch segs[i].Role {
		case RoleSpeaker:
			speaker = &segs[i]
		case RoleVoiceOver:
			spoken = &segs[i]
		}
	}
	if speaker == nil || speaker.Text != "JOHN" || !speaker.VoiceOver {
		t.Fatalf("bad speaker segment: %+v", speaker)
	}
	if spoken == nil || spoken.Speaker != "JOHN" || !spoken.VoiceOver {
		t.Fatalf("bad VO segment: %+v", spoken)
	}
}

func TestLabelOnlyLineCarriesSpeakerToNextLine(t *testing.T) {
	segs := ClassifyCue(Cue{ID: "m", Text: "JOHN:\nHello there."})
	if len(segs) != 2 {
		t.Fatalf("segments = %+v", segs)
	}
	if segs[0].Role != RoleSpeaker || segs[0].Text != "JOHN" {
		t.Fatalf("first = %+v", segs[0])
	}
	if segs[1].Role != RoleDialogue || segs[1].Speaker != "JOHN" || segs[1].Text != "Hello there." {
		t.Fatalf("second = %+v", segs[1])
	}
}

func TestStripLyricsOption(t *testing.T) {
	line := "♪ La la la ♪"
	if got := StripSDHLine(line, DefaultPlainOptions()); got != "♪ La la la ♪" {
		t.Fatalf("default strip = %q", got)
	}
	opts := DefaultPlainOptions()
	opts.StripMusicNotes = true
	if got := StripSDHLine(line, opts); got != "La la la" {
		t.Fatalf("strip notes = %q", got)
	}
}

func TestKeepSpeakerLabelsOption(t *testing.T) {
	line := "张三：你好。"
	opts := DefaultPlainOptions()
	opts.KeepSpeakerLabels = true
	if got := StripSDHLine(line, opts); got != "张三：你好。" {
		t.Fatalf("keep labels = %q", got)
	}
}

func TestVOFlagDoesNotLeakToNextSpeaker(t *testing.T) {
	segs := ClassifyCue(Cue{ID: "m", Text: "JOHN (V.O.): far away.\nMARY: Not me."})
	var mary *Segment
	for i := range segs {
		if segs[i].Speaker == "MARY" && segs[i].Role != RoleSpeaker {
			mary = &segs[i]
		}
	}
	if mary == nil {
		t.Fatalf("missing MARY spoken segment: %+v", segs)
	}
	if mary.Role != RoleDialogue || mary.VoiceOver {
		t.Fatalf("MARY line should be plain dialogue, got %+v", mary)
	}
}

func TestMixedOneLineAllRoles(t *testing.T) {
	// Speaker label, inline environmental sound and dialogue on one line.
	segs := classifyLine(t, "张三：[雷声] 快走！")
	want := []Role{RoleSpeaker, RoleSound, RoleDialogue}
	if got := roleSequence(segs); !reflect.DeepEqual(got, want) {
		t.Fatalf("roles = %v, want %v", got, want)
	}
	if got := StripSDHLine("张三：[雷声] 快走！", DefaultPlainOptions()); got != "快走！" {
		t.Fatalf("plain = %q", got)
	}
}

func TestStyleTagsStripped(t *testing.T) {
	line := `<i>♪ Hello ♪</i>`
	segs := classifyLine(t, line)
	if len(segs) != 1 || segs[0].Role != RoleLyric {
		t.Fatalf("expected single lyric segment, got %+v", segs)
	}
	if segs[0].Text != "Hello" {
		t.Fatalf("lyric text = %q", segs[0].Text)
	}
	if got := StripSDHLine(line, DefaultPlainOptions()); got != "♪ Hello ♪" {
		t.Fatalf("plain = %q", got)
	}
}

func TestDashLabelOnlyLeavesNoDanglingDash(t *testing.T) {
	if got := StripSDHLine("- JOHN:", DefaultPlainOptions()); got != "" {
		t.Fatalf("label-only dash line plain = %q, want empty", got)
	}
	if got := StripSDHLine("♪♪", DefaultPlainOptions()); got != "♪♪" {
		t.Fatalf("note-only line = %q, want preserved", got)
	}
}

func TestAsteriskSoundDescription(t *testing.T) {
	segs := classifyLine(t, "*door slams*")
	if len(segs) != 1 || segs[0].Role != RoleSound || segs[0].Text != "door slams" {
		t.Fatalf("asterisk sound = %+v", segs)
	}
	if got := StripSDHLine("*sighs* Maybe.", DefaultPlainOptions()); got != "Maybe." {
		t.Fatalf("plain = %q", got)
	}
}
