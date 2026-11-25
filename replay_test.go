package clipgroup

import (
	"path/filepath"
	"reflect"
	"testing"
)

func doAction(op Op) Action { return Action{Kind: ActionDo, Op: &op} }

// interleavedScripts 给出三段不同的交错 Undo/Redo 序列。
// 第三段包含三轮 undo/redo 交替。
func interleavedScripts() map[string]Script {
	create := CreateGroupOp(sampleSpec())
	move := MoveGroupOp("g1", Vec2{X: 10, Y: 5})
	lockSub := SetClipLockedOp("g1", "c-sub", true)
	lockMainFalse := SetClipLockedOp("g1", "c-main", false)
	param := SetEffectParamOp("g1", "c-main", "fx-color", "intensity", 0.9)
	hue := SetEffectParamOp("g1", "c-main", "fx-color", "hue", 99)
	revise := ReviseResourceOp("g1", "res-color", map[string]string{"version-tag": "v2"})

	scriptA := Script{Actions: []Action{
		doAction(create),
		doAction(move),
		doAction(lockSub),
		doAction(param),
		{Kind: ActionUndo},
		{Kind: ActionUndo},
		{Kind: ActionRedo},
	}}

	scriptB := Script{Actions: []Action{
		doAction(create),
		doAction(move),
		doAction(lockSub),
		doAction(param),
		{Kind: ActionUndo}, // 撤参数
		{Kind: ActionUndo}, // 撤锁定
		{Kind: ActionRedo}, // 重锁定
		{Kind: ActionUndo}, // 再撤锁定
		{Kind: ActionRedo}, // 再重锁定
		{Kind: ActionUndo}, // 回到 move 点
		doAction(revise),   // 分叉新提交，清空 redo 分支
		{Kind: ActionUndo},
		{Kind: ActionRedo},
	}}

	scriptC := Script{Actions: []Action{
		doAction(create),
		doAction(move),
		doAction(lockMainFalse),
		doAction(hue),
		// 第一轮交错
		{Kind: ActionUndo},
		{Kind: ActionRedo},
		// 第二轮交错
		{Kind: ActionUndo},
		{Kind: ActionUndo},
		{Kind: ActionRedo},
		// 第三轮交错
		{Kind: ActionUndo},
		{Kind: ActionRedo},
		{Kind: ActionUndo},
	}}

	return map[string]Script{"A_undoUndoRedo": scriptA, "B_branch": scriptB, "C_threeCycles": scriptC}
}

func assertEditorEqual(t *testing.T, label string, want, got *Editor) {
	t.Helper()
	if got.Digest() != want.Digest() {
		t.Fatalf("%s: document digest mismatch\nwant=%s\ngot =%s", label, want.Digest(), got.Digest())
	}
	want.mu.Lock()
	wantUndo := append([]string(nil), want.undo...)
	wantRedo := append([]string(nil), want.redo...)
	want.mu.Unlock()

	got.mu.Lock()
	gotUndo := append([]string(nil), got.undo...)
	gotRedo := append([]string(nil), got.redo...)
	got.mu.Unlock()

	if !reflect.DeepEqual(gotUndo, wantUndo) {
		t.Fatalf("%s: undo stack %v != %v", label, gotUndo, wantUndo)
	}
	if !reflect.DeepEqual(gotRedo, wantRedo) {
		t.Fatalf("%s: redo stack %v != %v", label, gotRedo, wantRedo)
	}
}

// 三段交错序列分别通过：内存重放、JSON 脚本往返重放、
// 日志文件重放 + 损坏恢复式重开，结果必须完全一致。
func TestThreeInterleavedUndoRedoAreReplayable(t *testing.T) {
	for name, script := range interleavedScripts() {
		t.Run(name, func(t *testing.T) {
			want, err := Replay(script)
			if err != nil {
				t.Fatalf("baseline replay: %v", err)
			}

			// 通道 1：直接内存重放。
			mem, err := Replay(script)
			if err != nil {
				t.Fatalf("memory replay: %v", err)
			}
			assertEditorEqual(t, "memory", want, mem)

			// 通道 2：脚本落盘为 JSON 后读回再重放。
			scriptPath := filepath.Join(t.TempDir(), "script.json")
			if err := SaveScript(scriptPath, script); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadScript(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			fromJSON, err := Replay(loaded)
			if err != nil {
				t.Fatalf("json replay: %v", err)
			}
			assertEditorEqual(t, "json", want, fromJSON)

			// 通道 3：把交错动作写入分帧日志，关闭后按“恢复”路径重放。
			journalPath := filepath.Join(t.TempDir(), "interleaved.log")
			journaled, err := ReplayAt(script, journalPath)
			if err != nil {
				t.Fatalf("journal replay: %v", err)
			}
			if err := journaled.Close(); err != nil {
				t.Fatal(err)
			}

			report, err := ReadJournal(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			if report.Corrupted() {
				t.Fatalf("interleaved journal unexpectedly corrupt: %s", report.Reason)
			}
			if len(report.Entries) != len(script.Actions) {
				t.Fatalf("journal entries = %d, actions = %d", len(report.Entries), len(script.Actions))
			}

			recovered, recoveryReport, err := RecoverEditor(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			if recoveryReport.LastCompleteSeq() != uint64(len(script.Actions)) {
				t.Fatalf("last complete seq = %d, want %d", recoveryReport.LastCompleteSeq(), len(script.Actions))
			}
			assertEditorEqual(t, "recovered", want, recovered)

			// 恢复出的编辑器还能继续 Undo/Redo，并且与内存参考同步。
			reopened, _, err := OpenEditor(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			assertEditorEqual(t, "reopened", want, reopened)

			if want.CanUndo() {
				if _, err := want.Undo(); err != nil {
					t.Fatal(err)
				}
				if _, err := reopened.Undo(); err != nil {
					t.Fatal(err)
				}
				assertEditorEqual(t, "post-reopen undo", want, reopened)
			}
		})
	}
}

// 校验三段脚本最终业务状态符合预期（而不只是彼此相同）。
func TestInterleavedScriptsExpectedState(t *testing.T) {
	scripts := interleavedScripts()

	t.Run("A_lockRedone_paramUndone", func(t *testing.T) {
		ed, err := Replay(scripts["A_undoUndoRedo"])
		if err != nil {
			t.Fatal(err)
		}
		sub, _ := ed.Clip("g1", "c-sub")
		if !sub.Locked {
			t.Error("c-sub lock should be redone")
		}
		main, _ := ed.Clip("g1", "c-main")
		if main.Effects[0].Params["intensity"] != 0.75 {
			t.Error("intensity edit should remain undone")
		}
		if ed.UndoDepth() != 3 || ed.RedoDepth() != 1 {
			t.Fatalf("stacks = %d/%d, want 3/1", ed.UndoDepth(), ed.RedoDepth())
		}
	})

	t.Run("B_branch_clearsRedo", func(t *testing.T) {
		ed, err := Replay(scripts["B_branch"])
		if err != nil {
			t.Fatal(err)
		}
		sub, _ := ed.Clip("g1", "c-sub")
		if sub.Locked {
			t.Error("lock should be undone at branch point")
		}
		res, _ := ed.Resource("res-color")
		if res.Meta["version-tag"] != "v2" || res.Version <= 1 {
			t.Fatalf("branched resource revision wrong: %+v", res)
		}
		if hist := ed.Versions().History(KindResource, "res-color"); len(hist) != 2 {
			t.Fatalf("resource history len = %d, want 2", len(hist))
		}
		if ed.UndoDepth() != 3 || ed.RedoDepth() != 0 {
			t.Fatalf("stacks = %d/%d, want 3/0", ed.UndoDepth(), ed.RedoDepth())
		}
	})

	t.Run("C_threeCycles_endAtMovedPoint", func(t *testing.T) {
		ed, err := Replay(scripts["C_threeCycles"])
		if err != nil {
			t.Fatal(err)
		}
		main, _ := ed.Clip("g1", "c-main")
		if !main.Locked {
			t.Error("lock toggle should end undone, restoring locked=true")
		}
		if main.Effects[0].Params["hue"] != 12 {
			t.Error("hue edit should end undone")
		}
		if main.Position != (Vec2{X: 20, Y: 15}) {
			t.Errorf("move should remain applied: %+v", main.Position)
		}
		if ed.UndoDepth() != 2 || ed.RedoDepth() != 2 {
			t.Fatalf("stacks = %d/%d, want 2/2", ed.UndoDepth(), ed.RedoDepth())
		}
	})
}

// 真实 API 驱动出的脚本应当与手写交错脚本一致，且同样可重放。
func TestRecordedScriptMatchesHandWritten(t *testing.T) {
	for name, script := range interleavedScripts() {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "recorded.log")
			ed, _, err := OpenEditor(path)
			if err != nil {
				t.Fatal(err)
			}
			for i, action := range script.Actions {
				switch action.Kind {
				case ActionDo:
					if _, err := ed.Do(*action.Op); err != nil {
						t.Fatalf("action %d: %v", i, err)
					}
				case ActionUndo:
					if _, err := ed.Undo(); err != nil {
						t.Fatalf("action %d: %v", i, err)
					}
				case ActionRedo:
					if _, err := ed.Redo(); err != nil {
						t.Fatalf("action %d: %v", i, err)
					}
				}
			}
			recorded := ed.Script()
			if !reflect.DeepEqual(recorded, script) {
				t.Fatalf("recorded script != hand-written script\nrecorded=%+v\nwanted  =%+v", recorded, script)
			}
			if err := ed.Close(); err != nil {
				t.Fatal(err)
			}

			recovered, _, err := RecoverEditor(path)
			if err != nil {
				t.Fatal(err)
			}
			want, err := Replay(script)
			if err != nil {
				t.Fatal(err)
			}
			assertEditorEqual(t, "recorded-journal", want, recovered)
		})
	}
}
