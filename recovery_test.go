package clipgroup

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildFiveTxJournal 创建 5 个提交（1 个建组 + 4 个更新），
// 返回日志路径与每个提交后的文档摘要。
func buildFiveTxJournal(t *testing.T) (string, []string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tx.log")
	ed, report, err := OpenEditor(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Corrupted() {
		t.Fatal("fresh journal reported corrupted")
	}

	ops := []Op{
		CreateGroupOp(sampleSpec()),
		MoveGroupOp("g1", Vec2{X: 12, Y: 8}),
		SetClipLockedOp("g1", "c-sub", true),
		SetEffectParamOp("g1", "c-main", "fx-color", "intensity", 0.42),
		ReviseResourceOp("g1", "res-color", map[string]string{"version-tag": "v2"}),
	}
	points := make([]string, 0, len(ops))
	for _, op := range ops {
		if _, err := ed.Do(op); err != nil {
			t.Fatalf("setup op %s: %v", op.Type, err)
		}
		points = append(points, ed.Digest())
	}
	if err := ed.Close(); err != nil {
		t.Fatal(err)
	}
	return path, points
}

func copyJournal(t *testing.T, src, dst string) string {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestRecoverTornTailToLatestCompletePoint(t *testing.T) {
	src, points := buildFiveTxJournal(t)
	torn := copyJournal(t, src, filepath.Join(t.TempDir(), "torn.log"))

	info, err := os.Stat(torn)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(torn, info.Size()-7); err != nil {
		t.Fatal(err)
	}

	ed, report, err := RecoverEditor(torn)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Corrupted() || report.Reason != "torn frame: missing payload or checksum" {
		t.Fatalf("expected torn-frame report, got %+v", report)
	}
	if len(report.Entries) != 4 {
		t.Fatalf("complete entries = %d, want 4", len(report.Entries))
	}
	if got := ed.Digest(); got != points[3] {
		t.Fatalf("recovered state is not latest complete point\nwant=%s\ngot =%s", points[3], got)
	}
}

func TestRecoverMidLogCRCCorruption(t *testing.T) {
	src, points := buildFiveTxJournal(t)
	bad := copyJournal(t, src, filepath.Join(t.TempDir(), "bad.log"))

	data, err := os.ReadFile(bad)
	if err != nil {
		t.Fatal(err)
	}
	starts := frameStarts(t, data)
	if len(starts) != 5 {
		t.Fatalf("frame starts = %d, want 5", len(starts))
	}
	// 翻转第 3 帧 payload 内的一个字节，CRC 必然失配。
	off := starts[2] + frameHeader + 2
	data[off] ^= 0xFF
	if err := os.WriteFile(bad, data, 0o600); err != nil {
		t.Fatal(err)
	}

	ed, report, err := RecoverEditor(bad)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Corrupted() || report.Reason != "crc32 mismatch" {
		t.Fatalf("expected crc mismatch, got %+v", report)
	}
	if len(report.Entries) != 2 {
		t.Fatalf("entries before corruption = %d, want 2", len(report.Entries))
	}
	if got := ed.Digest(); got != points[1] {
		t.Fatalf("recovery should stop at complete point 2\nwant=%s\ngot =%s", points[1], got)
	}
}

func TestRecoverGarbageTailKeepsAllCompleteTxs(t *testing.T) {
	src, points := buildFiveTxJournal(t)
	garbage := copyJournal(t, src, filepath.Join(t.TempDir(), "garbage.log"))

	f, err := os.OpenFile(garbage, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("THIS-IS-NOT-A-FRAME"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	ed, report, err := RecoverEditor(garbage)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Corrupted() || report.Reason != "bad frame magic" {
		t.Fatalf("expected bad magic, got %+v", report)
	}
	if len(report.Entries) != 5 {
		t.Fatalf("valid entries = %d, want 5", len(report.Entries))
	}
	if ed.Digest() != points[4] {
		t.Fatal("all five complete txs must survive garbage tail")
	}
}

func TestOpenEditorRepairsAndContinues(t *testing.T) {
	src, points := buildFiveTxJournal(t)
	torn := copyJournal(t, src, filepath.Join(t.TempDir(), "repair.log"))
	info, _ := os.Stat(torn)
	_ = os.Truncate(torn, info.Size()-7)

	ed, report, err := OpenEditor(torn)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Corrupted() || len(report.Entries) != 4 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if ed.Digest() != points[3] {
		t.Fatal("repaired editor did not open at latest complete point")
	}
	if ed.UndoDepth() != 4 {
		t.Fatalf("undo depth after recovery = %d, want 4", ed.UndoDepth())
	}

	// 修复后可以继续提交、撤销，并且重新打开结果稳定。
	if _, err := ed.Do(SetClipLockedOp("g1", "c-main", false)); err != nil {
		t.Fatal(err)
	}
	wantDigest := ed.Digest()
	if err := ed.Close(); err != nil {
		t.Fatal(err)
	}

	again, report2, err := OpenEditor(torn)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if report2.Corrupted() {
		t.Fatalf("repaired journal still corrupt: %s", report2.Reason)
	}
	if got := again.Digest(); got != wantDigest {
		t.Fatalf("post-repair continuation not durable\nwant=%s\ngot =%s", wantDigest, got)
	}
	if _, err := again.Undo(); err != nil {
		t.Fatalf("undo after repaired reopen: %v", err)
	}
	if again.Digest() != points[3] {
		t.Fatal("undo of post-repair tx should return to repaired point")
	}
}

// frameStarts 按帧布局解析出每一完整帧的起始偏移。
func frameStarts(t *testing.T, data []byte) []int {
	t.Helper()
	var starts []int
	off := 0
	for off+frameHeader <= len(data) {
		if string(data[off:off+4]) != journalMagic {
			break
		}
		n := int(binary.BigEndian.Uint32(data[off+4 : off+8]))
		end := off + frameHeader + n + frameTrailer
		if end > len(data) {
			break
		}
		starts = append(starts, off)
		off = end
	}
	return starts
}
