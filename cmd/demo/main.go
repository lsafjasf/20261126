// Command demo 端到端演示：
//  1. 组整体移动后 Undo，锁定状态与效果参数完整恢复；
//  2. 组/成员/依赖资源的版本均被保存；
//  3. 三次交错 Undo/Redo 脚本可重放；
//  4. 日志损坏后恢复到最近完整点并继续工作。
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	clipgroup "clipgroup"
)

func main() {
	dir, err := os.MkdirTemp("", "clipgroup-demo-*")
	if err != nil {
		log.Fatal(err)
	}
	journalPath := filepath.Join(dir, "journal.log")
	scriptPath := filepath.Join(dir, "interleaved.json")

	ed, report, err := clipgroup.OpenEditor(journalPath)
	must(err)
	if report.Corrupted() {
		log.Fatal("fresh journal should not be corrupt")
	}

	spec := clipgroup.GroupSpec{
		ID: "g1", Name: "片头组", Position: clipgroup.Vec2{X: 100, Y: 200},
		Resources: []clipgroup.Resource{{
			ID: "res-color", Kind: "effect-preset", URI: "res://teal-orange",
			Meta: map[string]string{"version-tag": "v1"},
		}},
		Clips: []clipgroup.ClipSpec{
			{
				ID: "c-main", Name: "主视频", Locked: true, Position: clipgroup.Vec2{X: 10, Y: 10},
				Effects: []clipgroup.Effect{{
					ID: "fx-color", Kind: "color-grade", Enabled: true,
					Params:          map[string]float64{"intensity": 0.75, "hue": 12},
					ResourceID:      "res-color",
					ResourceVersion: 1,
				}},
			},
			{
				ID: "c-sub", Name: "字幕", Locked: false, Position: clipgroup.Vec2{X: 40, Y: 10},
				Effects: []clipgroup.Effect{{
					ID: "fx-shadow", Kind: "shadow", Enabled: true,
					Params: map[string]float64{"opacity": 0.6},
				}},
			},
		},
	}

	if _, err := ed.Do(clipgroup.CreateGroupOp(spec)); err != nil {
		log.Fatal(err)
	}
	cleanDigest := ed.Digest()

	fmt.Println("== 1. 整体移动组 ==")
	if _, err := ed.Do(clipgroup.MoveGroupOp("g1", clipgroup.Vec2{X: 37, Y: -15})); err != nil {
		log.Fatal(err)
	}
	main := mustClip(ed, "c-main")
	fmt.Printf("移动后 c-main 位置=%+v 锁定=%v intensity=%.2f\n",
		main.Position, main.Locked, main.Effects[0].Params["intensity"])

	fmt.Println("== 2. Undo：完整事务点恢复 ==")
	if _, err := ed.Undo(); err != nil {
		log.Fatal(err)
	}
	main = mustClip(ed, "c-main")
	sub := mustClip(ed, "c-sub")
	fmt.Printf("撤销后 c-main 位置=%+v 锁定=%v intensity=%.2f hue=%.0f\n",
		main.Position, main.Locked, main.Effects[0].Params["intensity"], main.Effects[0].Params["hue"])
	fmt.Printf("撤销后 c-sub  锁定=%v opacity=%.2f\n", sub.Locked, sub.Effects[0].Params["opacity"])
	if ed.Digest() != cleanDigest {
		log.Fatal("FAIL: undo did not restore the exact aggregate")
	}
	fmt.Println("OK: 组、成员锁定状态、效果参数与依赖资源全部回到事务前版本")

	fmt.Println("== 3. 版本仓库 ==")
	history := ed.Versions().History(clipgroup.KindClip, "c-main")
	fmt.Printf("c-main 保存的版本数=%d，仓库实体版本总数=%d\n", len(history), ed.Versions().Count())

	fmt.Println("== 4. 三次交错 Undo/Redo 重放 ==")
	script := clipgroup.Script{Actions: []clipgroup.Action{
		{Kind: clipgroup.ActionDo, Op: opPtr(clipgroup.CreateGroupOp(spec))},
		{Kind: clipgroup.ActionDo, Op: opPtr(clipgroup.MoveGroupOp("g1", clipgroup.Vec2{X: 5, Y: 5}))},
		{Kind: clipgroup.ActionDo, Op: opPtr(clipgroup.SetClipLockedOp("g1", "c-sub", true))},
		{Kind: clipgroup.ActionDo, Op: opPtr(clipgroup.SetEffectParamOp("g1", "c-main", "fx-color", "intensity", 0.2))},
		{Kind: clipgroup.ActionUndo},
		{Kind: clipgroup.ActionRedo},
		{Kind: clipgroup.ActionUndo},
		{Kind: clipgroup.ActionUndo},
		{Kind: clipgroup.ActionRedo},
		{Kind: clipgroup.ActionUndo},
		{Kind: clipgroup.ActionRedo},
	}}
	must(clipgroup.SaveScript(scriptPath, script))
	loaded, err := clipgroup.LoadScript(scriptPath)
	must(err)
	replayed, err := clipgroup.Replay(loaded)
	must(err)

	again, _, err := clipgroup.OpenEditor(filepath.Join(dir, "replay.log"))
	must(err)
	for i, a := range loaded.Actions {
		switch a.Kind {
		case clipgroup.ActionDo:
			_, err = again.Do(*a.Op)
		case clipgroup.ActionUndo:
			_, err = again.Undo()
		case clipgroup.ActionRedo:
			_, err = again.Redo()
		}
		if err != nil {
			log.Fatalf("replay step %d: %v", i, err)
		}
	}
	if replayed.Digest() != again.Digest() {
		log.Fatal("FAIL: in-memory and journaled replay differ")
	}
	fmt.Printf("OK: %d 步交错序列在内存与日志中重放一致，digest=%s\n",
		len(script.Actions), short(replayed.Digest()))
	must(again.Close())
	must(ed.Close())

	fmt.Println("== 5. 日志损坏恢复到最近完整点 ==")
	raw, err := os.ReadFile(journalPath)
	must(err)
	raw = append(raw, []byte("BROKEN-FRAME-TAIL")...)
	must(os.WriteFile(journalPath, raw, 0o600))

	report, err = clipgroup.ReadJournal(journalPath)
	must(err)
	fmt.Printf("扫描到损坏: %q @offset=%d，最近完整点 seq=%d，有效条目=%d\n",
		report.Reason, report.CorruptAt, report.LastCompleteSeq(), len(report.Entries))

	recovered, repairReport, err := clipgroup.OpenEditor(journalPath)
	must(err)
	if !repairReport.Corrupted() {
		log.Fatal("expected repair report to mention corruption")
	}
	if recovered.Digest() != cleanDigest {
		log.Fatal("FAIL: recovery did not land on latest complete point")
	}
	if _, err := recovered.Do(clipgroup.SetClipLockedOp("g1", "c-sub", true)); err != nil {
		log.Fatalf("journal not usable after repair: %v", err)
	}
	must(recovered.Close())
	fmt.Println("OK: 已截断坏帧到最近完整点，修复后可继续提交")
	fmt.Println("\n全部演示通过 ✔")
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func mustClip(ed *clipgroup.Editor, id string) clipgroup.Clip {
	c, err := ed.Clip("g1", id)
	if err != nil {
		log.Fatal(err)
	}
	return c
}

func opPtr(op clipgroup.Op) *clipgroup.Op { return &op }

func short(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}
