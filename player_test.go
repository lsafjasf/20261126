package trackselect

import (
	"testing"
	"time"
)

func TestSwitchPreservesPosition(t *testing.T) {
	tracks := sampleTracks()
	p := NewPlayer(Select(tracks, DefaultPreferences(ScenarioDefault)))
	p.Advance(90 * time.Second)

	before, after := p.Switch(tracks, DefaultPreferences(ScenarioNoisy))
	if before != after || after != 90*time.Second {
		t.Fatalf("切换不应改变播放位置: before=%v after=%v", before, after)
	}
	if p.Selection().Subtitle == nil || p.Selection().Subtitle.ID != "s-zh-sdh" {
		t.Fatalf("切换后应生效 SDH 字幕，得到 %+v", p.Selection().Subtitle)
	}
	if len(p.History()) != 1 || p.History()[0].At != 90*time.Second {
		t.Fatalf("切换历史应记录位置 90s，得到 %+v", p.History())
	}
}

func TestMultipleSwitchesKeepClock(t *testing.T) {
	tracks := sampleTracks()
	p := NewPlayer(Select(tracks, DefaultPreferences(ScenarioDefault)))
	scenarios := []Scenario{ScenarioNoisy, ScenarioImpaired, ScenarioLanguage, ScenarioDefault}
	for i, s := range scenarios {
		p.Advance(time.Minute)
		before, after := p.Switch(tracks, DefaultPreferences(s))
		want := time.Duration(i+1) * time.Minute
		if before != want || after != want {
			t.Fatalf("第 %d 次切换位置漂移: before=%v after=%v want=%v", i, before, after, want)
		}
	}
	if len(p.History()) != len(scenarios) {
		t.Fatalf("应记录 %d 次切换，得到 %d", len(scenarios), len(p.History()))
	}
}

func TestSeekNegativeRejected(t *testing.T) {
	p := NewPlayer(Selection{})
	p.Advance(10 * time.Second)
	if err := p.Seek(-time.Second); err == nil {
		t.Fatal("负位置应被拒绝")
	}
	if p.Position() != 10*time.Second {
		t.Fatalf("拒绝后位置应保持 10s，得到 %v", p.Position())
	}
}
