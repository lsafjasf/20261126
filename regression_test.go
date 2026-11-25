package clipgroup

import (
	"testing"
)

func sampleSpec() GroupSpec {
	return GroupSpec{
		ID:       "g1",
		Name:     "片头片段组",
		Position: Vec2{X: 100, Y: 200},
		Resources: []Resource{
			{
				ID:   "res-color",
				Kind: "effect-preset",
				URI:  "res://presets/teal-orange",
				Meta: map[string]string{"lut": "teal-orange.cube", "version-tag": "v1"},
			},
		},
		Clips: []ClipSpec{
			{
				ID:       "c-main",
				Name:     "主视频",
				Locked:   true,
				Position: Vec2{X: 10, Y: 10},
				Effects: []Effect{
					{
						ID: "fx-color", Kind: "color-grade", Enabled: true,
						Params:          map[string]float64{"intensity": 0.75, "hue": 12},
						ResourceID:      "res-color",
						ResourceVersion: 1,
					},
					{
						ID: "fx-blur", Kind: "blur", Enabled: false,
						Params: map[string]float64{"radius": 4},
					},
				},
			},
			{
				ID:       "c-sub",
				Name:     "字幕条",
				Locked:   false,
				Position: Vec2{X: 40, Y: 10},
				Effects: []Effect{
					{
						ID: "fx-shadow", Kind: "shadow", Enabled: true,
						Params: map[string]float64{"offset": 2, "opacity": 0.6},
					},
				},
			},
		},
	}
}

func newSeededEditor(t *testing.T) *Editor {
	t.Helper()
	ed := NewEditor()
	if _, err := ed.Do(CreateGroupOp(sampleSpec())); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	return ed
}

// 复现用户反馈：整体移动片段组后撤销，
// 组内单项的锁定状态与效果参数必须随事务完整恢复。
func TestMoveGroupUndoRestoresLocksAndEffects(t *testing.T) {
	ed := newSeededEditor(t)
	before := ed.Document()
	beforeDigest := Digest(before)

	if _, err := ed.Do(MoveGroupOp("g1", Vec2{X: 37, Y: -15})); err != nil {
		t.Fatalf("move group: %v", err)
	}

	mainAfterMove, err := ed.Clip("g1", "c-main")
	if err != nil {
		t.Fatal(err)
	}
	if got := mainAfterMove.Position; got != (Vec2{X: 47, Y: -5}) {
		t.Fatalf("member not moved with group: got %+v", got)
	}
	if !mainAfterMove.Locked {
		t.Fatal("sanity: c-main should stay locked after move")
	}

	if _, err := ed.Undo(); err != nil {
		t.Fatalf("undo move: %v", err)
	}

	if got := ed.Digest(); got != beforeDigest {
		t.Fatalf("undo move did not restore full aggregate\nbefore=%s\nafter =%s", beforeDigest, got)
	}

	main, err := ed.Clip("g1", "c-main")
	if err != nil {
		t.Fatal(err)
	}
	if !main.Locked {
		t.Error("locked state of c-main was not restored")
	}
	if main.Position != (Vec2{X: 10, Y: 10}) {
		t.Errorf("position not restored: %+v", main.Position)
	}
	wantIntensity := 0.75
	if got := main.Effects[0].Params["intensity"]; got != wantIntensity {
		t.Errorf("effect param intensity = %v, want %v", got, wantIntensity)
	}
	if got := main.Effects[0].Params["hue"]; got != 12 {
		t.Errorf("effect param hue = %v, want 12", got)
	}
	if main.Effects[1].Params["radius"] != 4 {
		t.Error("secondary effect params were not restored")
	}

	sub, err := ed.Clip("g1", "c-sub")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Locked {
		t.Error("unlocked state of c-sub was not restored as unlocked")
	}
	if sub.Effects[0].Params["opacity"] != 0.6 {
		t.Error("subtitle effect params were not restored")
	}

	group, _ := ed.Group("g1")
	if group.Position != (Vec2{X: 100, Y: 200}) {
		t.Errorf("group origin not restored: %+v", group.Position)
	}
}

// 组合组事务同时改动位置、锁定、效果参数与依赖资源；
// 一次 Undo 必须把四类改动作为一个原子点全部恢复。
func TestCompositeGroupTxUndoRestoresEverything(t *testing.T) {
	ed := newSeededEditor(t)
	beforeDigest := ed.Digest()

	_, err := ed.Transact(Op{Type: "composite_edit", GroupID: "g1"}, func(agg *Aggregate) error {
		agg.Group.Position.X += 5
		for _, id := range agg.Group.MemberIDs {
			c, err := agg.Clip(id)
			if err != nil {
				return err
			}
			c.Position.X += 5
		}
		main, err := agg.Clip("c-main")
		if err != nil {
			return err
		}
		main.Locked = false
		main.Effects[0].Params["intensity"] = 1.0

		res, err := agg.Resource("res-color")
		if err != nil {
			return err
		}
		res.Meta["version-tag"] = "v2-draft"
		return nil
	})
	if err != nil {
		t.Fatalf("composite tx: %v", err)
	}

	main, _ := ed.Clip("g1", "c-main")
	if main.Locked {
		t.Fatal("composite tx should have unlocked c-main")
	}
	if main.Effects[0].Params["intensity"] != 1.0 {
		t.Fatal("composite tx should have changed intensity")
	}
	res, _ := ed.Resource("res-color")
	if res.Meta["version-tag"] != "v2-draft" || res.Version < 2 {
		t.Fatalf("resource revision not committed: %+v", res)
	}

	if _, err := ed.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := ed.Digest(); got != beforeDigest {
		t.Fatalf("composite undo mismatch\nbefore=%s\nafter =%s", beforeDigest, got)
	}

	main, _ = ed.Clip("g1", "c-main")
	if !main.Locked || main.Effects[0].Params["intensity"] != 0.75 {
		t.Fatalf("lock/effect not restored: locked=%v fx=%+v", main.Locked, main.Effects[0])
	}
	res, _ = ed.Resource("res-color")
	if res.Meta["version-tag"] != "v1" || res.Version != 1 {
		t.Fatalf("resource version not restored: %+v", res)
	}
}

// 反证：只恢复位置的“部分撤销”无法还原锁定与效果参数，
// 说明完整聚合快照是必要的，而不是测试本身没有区分度。
func TestPositionOnlyPartialUndoIsInsufficient(t *testing.T) {
	ed := newSeededEditor(t)
	before := ed.Document()

	_, err := ed.Transact(Op{Type: "composite_edit", GroupID: "g1"}, func(agg *Aggregate) error {
		main, _ := agg.Clip("c-main")
		main.Locked = false
		main.Effects[0].Params["intensity"] = 99
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := ed.Undo()
	if err != nil {
		t.Fatal(err)
	}
	if ed.Digest() != Digest(before) {
		t.Fatal("full undo should restore everything")
	}

	// 重新执行事务后，用“只回滚坐标”的朴素方式处理。
	if _, err := ed.Redo(); err != nil {
		t.Fatal(err)
	}
	partial := ed.Document()
	oldPositions := map[string]Vec2{}
	for _, c := range tx.Before.Clips {
		oldPositions[c.ID] = c.Position
	}
	for id, pos := range oldPositions {
		partial.Clips[id].Position = pos
	}
	if Digest(partial) == Digest(before) {
		t.Fatal("position-only restore unexpectedly matched full snapshot")
	}
	if partial.Clips["c-main"].Locked {
		t.Fatal("partial undo unexpectedly restored lock state")
	}
	if got := partial.Clips["c-main"].Effects[0].Params["intensity"]; got != 99 {
		t.Fatalf("partial undo should leave effect param at committed value, got %v", got)
	}
}
