package clipgroup

import "testing"

// 目标组不存在时必须返回错误，而不是在空聚合上 panic。
func TestOpOnMissingGroupErrorsNotPanics(t *testing.T) {
	ed := NewEditor()
	ops := []Op{
		MoveGroupOp("nope", Vec2{X: 1, Y: 1}),
		SetClipLockedOp("nope", "c1", true),
		SetEffectParamOp("nope", "c1", "fx", "k", 1),
		ReviseResourceOp("nope", "r1", map[string]string{"a": "b"}),
	}
	for _, op := range ops {
		if _, err := ed.Do(op); err == nil {
			t.Errorf("op %s on missing group should fail", op.Type)
		}
	}

	// 未知操作类型也要报错。
	if _, err := ed.Do(Op{Type: "does_not_exist", GroupID: "g1"}); err == nil {
		t.Error("unknown op type should fail")
	}

	// 空 Undo/Redo 返回可识别的哨兵错误。
	if _, err := ed.Undo(); err != ErrNothingToUndo {
		t.Errorf("undo empty error = %v, want ErrNothingToUndo", err)
	}
	if _, err := ed.Redo(); err != ErrNothingToRedo {
		t.Errorf("redo empty error = %v, want ErrNothingToRedo", err)
	}
}
