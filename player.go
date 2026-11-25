package trackselect

import (
	"errors"
	"time"
)

// SwitchEvent 记录一次轨道切换，Position 是切换发生时的播放位置。
type SwitchEvent struct {
	At       time.Duration
	Scenario Scenario
	Previous Selection
	Current  Selection
}

// Player 模拟播放器内核：维护播放位置与当前轨道选择。
// 切换轨道只更新选择，不触碰时钟，因此播放位置天然连续。
type Player struct {
	position  time.Duration
	selection Selection
	history   []SwitchEvent
}

// NewPlayer 以初始选择创建播放器，起始位置为 0。
func NewPlayer(initial Selection) *Player {
	return &Player{selection: initial}
}

// Position 返回当前播放位置。
func (p *Player) Position() time.Duration { return p.position }

// Selection 返回当前生效的轨道选择。
func (p *Player) Selection() Selection { return p.selection }

// History 返回全部切换记录。
func (p *Player) History() []SwitchEvent {
	return append([]SwitchEvent{}, p.history...)
}

// Advance 推进播放时钟，模拟正常播放。
func (p *Player) Advance(d time.Duration) {
	if d > 0 {
		p.position += d
	}
}

// Seek 跳转到指定位置；负值报错并保持原位置。
func (p *Player) Seek(pos time.Duration) error {
	if pos < 0 {
		return errors.New("trackselect: seek 位置不能为负")
	}
	p.position = pos
	return nil
}

// Switch 在给定场景下重新解析轨道并保持播放位置不变。
// 返回切换前后的位置，二者必然相等。
func (p *Player) Switch(tracks []Track, prefs Preferences) (before, after time.Duration) {
	before = p.position
	next := Select(tracks, prefs)
	p.history = append(p.history, SwitchEvent{
		At:       p.position,
		Scenario: prefs.Scenario,
		Previous: p.selection,
		Current:  next,
	})
	p.selection = next
	return before, p.position
}

// Apply 直接应用一次外部计算的选择，同样保持播放位置。
func (p *Player) Apply(sel Selection, prefs Preferences) (before, after time.Duration) {
	before = p.position
	p.history = append(p.history, SwitchEvent{
		At:       p.position,
		Scenario: prefs.Scenario,
		Previous: p.selection,
		Current:  sel,
	})
	p.selection = sel
	return before, p.position
}
