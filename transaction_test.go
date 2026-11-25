package clipgroup

import (
	"encoding/json"
	"errors"
	"testing"
)

// 变更函数中途失败时，文档、版本号、Undo 栈都不能发生部分提交。
func TestMutatorErrorMeansNoPartialCommit(t *testing.T) {
	ed := newSeededEditor(t)
	before := ed.Digest()
	versionsBefore := ed.Versions().Count()
	undoBefore := ed.UndoDepth()

	boom := errors.New("disk full (simulated)")
	_, err := ed.Transact(Op{Type: "boom_edit", GroupID: "g1"}, func(agg *Aggregate) error {
		main, err := agg.Clip("c-main")
		if err != nil {
			return err
		}
		main.Locked = false
		main.Effects[0].Params["intensity"] = 42
		agg.Group.Position.X += 999
		if _, err := agg.Resource("res-color"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped boom error, got %v", err)
	}

	if got := ed.Digest(); got != before {
		t.Fatalf("state changed after aborted tx\nbefore=%s\nafter =%s", before, got)
	}
	if ed.Versions().Count() != versionsBefore {
		t.Fatal("version store gained versions for an aborted tx")
	}
	if ed.UndoDepth() != undoBefore {
		t.Fatal("undo stack changed for an aborted tx")
	}

	// 中止后编辑器仍可正常提交后续事务。
	if _, err := ed.Do(SetClipLockedOp("g1", "c-sub", true)); err != nil {
		t.Fatalf("follow-up op after abort failed: %v", err)
	}
	sub, _ := ed.Clip("g1", "c-sub")
	if !sub.Locked {
		t.Fatal("follow-up commit did not land")
	}
}

// 日志落盘失败时，内存状态同样不能前进（提交顺序：先持久化，后切状态）。
func TestJournalAppendFailureRefusesCommit(t *testing.T) {
	ed := newSeededEditor(t)
	before := ed.Digest()
	undoBefore := ed.UndoDepth()

	if ed.journal != nil {
		t.Fatal("test editor should start journal-less")
	}
	j, err := OpenJournal(t.TempDir() + "/strict.log")
	if err != nil {
		t.Fatal(err)
	}
	ed.journal = j
	if err := j.Close(); err != nil { // 关闭底层文件，令 Append 失败
		t.Fatal(err)
	}

	if _, err := ed.Do(MoveGroupOp("g1", Vec2{X: 1, Y: 1})); err == nil {
		t.Fatal("expected append failure to surface")
	}
	if got := ed.Digest(); got != before {
		t.Fatal("in-memory state advanced despite failed durable commit")
	}
	if ed.UndoDepth() != undoBefore {
		t.Fatal("undo stack advanced despite failed commit")
	}
}

// 每个提交点都必须保存组、全部成员与依赖资源的版本。
func TestCommitSavesVersionsForGroupMembersAndResources(t *testing.T) {
	ed := NewEditor()
	tx, err := ed.Do(CreateGroupOp(sampleSpec()))
	if err != nil {
		t.Fatal(err)
	}

	kinds := map[string]bool{}
	for _, ref := range tx.After.Refs {
		kinds[ref.Kind+":"+ref.ID] = true
		if ref.Version != 1 {
			t.Errorf("first version of %s/%s = %d, want 1", ref.Kind, ref.ID, ref.Version)
		}
	}
	for _, want := range []string{
		"group:g1", "clip:c-main", "clip:c-sub", "resource:res-color",
	} {
		if !kinds[want] {
			t.Errorf("snapshot missing version ref %s; refs=%+v", want, tx.After.Refs)
		}
	}
	if len(tx.Before.Refs) != 0 || tx.Before.Present {
		t.Fatalf("create tx Before should be absent marker, got %+v", tx.Before)
	}

	if _, err := ed.Do(SetEffectParamOp("g1", "c-main", "fx-color", "intensity", 0.2)); err != nil {
		t.Fatal(err)
	}

	history := ed.Versions().History(KindClip, "c-main")
	if len(history) != 2 {
		t.Fatalf("c-main version history len = %d, want 2", len(history))
	}
	if history[0].Version != 1 || history[1].Version != 2 {
		t.Fatalf("versions not monotonic: %+v", history)
	}
	var restored Clip
	if err := json.Unmarshal(history[0].Data, &restored); err != nil {
		t.Fatal(err)
	}
	if !restored.Locked || restored.Effects[0].Params["intensity"] != 0.75 {
		t.Fatalf("stored v1 lost lock/effect data: %+v", restored)
	}

	resHistory := ed.Versions().History(KindResource, "res-color")
	if len(resHistory) != 1 {
		t.Fatalf("resource should have one preserved version, got %d", len(resHistory))
	}
}

// Undo 创建事务应当移除该事务引入的全部实体（absent Before 路径）。
func TestUndoCreateGroupRemovesAggregate(t *testing.T) {
	ed := newSeededEditor(t)
	if ed.Versions().Count() == 0 {
		t.Fatal("expected seeded versions")
	}
	if _, err := ed.Undo(); err != nil {
		t.Fatal(err)
	}
	doc := ed.Document()
	if _, ok := doc.Groups["g1"]; ok {
		t.Error("group still present after undo create")
	}
	if _, ok := doc.Clips["c-main"]; ok {
		t.Error("member clip still present after undo create")
	}
	if _, ok := doc.Resources["res-color"]; ok {
		t.Error("dependent resource still present after undo create")
	}
	if _, err := ed.Redo(); err != nil {
		t.Fatal(err)
	}
	other := newSeededEditor(t)
	if ed.Digest() != other.Digest() {
		t.Fatal("redo create did not restore equivalent aggregate")
	}
}
