package clipgroup

import (
	"path/filepath"
	"testing"
)

// 移动事务落盘后即使关闭编辑器、重新打开（走日志恢复），
// Undo 仍然必须还原组内单项的锁定状态与效果参数。
func TestUndoAfterReopenRestoresLocksAndEffects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "editor.log")

	ed, _, err := OpenEditor(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ed.Do(CreateGroupOp(sampleSpec())); err != nil {
		t.Fatal(err)
	}
	cleanDigest := ed.Digest()
	if _, err := ed.Do(MoveGroupOp("g1", Vec2{X: 8, Y: -3})); err != nil {
		t.Fatal(err)
	}
	if err := ed.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, report, err := OpenEditor(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if report.Corrupted() {
		t.Fatalf("clean journal reported corrupt: %s", report.Reason)
	}
	if reopened.UndoDepth() != 2 {
		t.Fatalf("undo depth after reopen = %d, want 2", reopened.UndoDepth())
	}
	if _, err := reopened.Undo(); err != nil {
		t.Fatalf("undo after reopen: %v", err)
	}

	if got := reopened.Digest(); got != cleanDigest {
		t.Fatalf("undo after reopen mismatch\nwant=%s\ngot =%s", cleanDigest, got)
	}
	main, err := reopened.Clip("g1", "c-main")
	if err != nil {
		t.Fatal(err)
	}
	if !main.Locked {
		t.Error("lock state not restored after reopen+undo")
	}
	if main.Effects[0].Params["intensity"] != 0.75 {
		t.Error("effect params not restored after reopen+undo")
	}
	if main.Position != (Vec2{X: 10, Y: 10}) {
		t.Errorf("position = %+v, want {10 10}", main.Position)
	}
}
