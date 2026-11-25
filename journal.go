package clipgroup

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"sync"
	"time"
)

// JournalEntryKind 是日志条目的类型。
type JournalEntryKind string

const (
	// EntryCommit 表示一个已原子提交的组事务，内含 Before/After 完整快照。
	EntryCommit JournalEntryKind = "commit"
	// EntryUndo 记录一次 Undo 导航，TargetID 指向被撤销的事务。
	EntryUndo JournalEntryKind = "undo"
	// EntryRedo 记录一次 Redo 导航，TargetID 指向被重做的事务。
	EntryRedo JournalEntryKind = "redo"
)

// Entry 是日志中的一个完整条目。所有交错的 Do/Undo/Redo
// 都按顺序写入日志，因此重放日志即可还原任意时刻的状态与撤销栈。
type Entry struct {
	Seq      uint64           `json:"seq"`
	Kind     JournalEntryKind `json:"kind"`
	At       time.Time        `json:"at"`
	TargetID string           `json:"targetId,omitempty"`
	Tx       *TxRecord        `json:"tx,omitempty"`
}

const (
	journalMagic = "CGTX"
	frameHeader  = 4 + 4 // 4 字节 magic + 4 字节 payload 长度
	frameTrailer = 4     // 4 字节 CRC32
	maxFrameSize = 64 << 20
)

// Journal 是追加式、自带帧校验的事务日志。
//
// 磁盘帧布局：
//
//	+--------+----------+---------+----------+
//	| "CGTX" | uint32 N | N bytes | CRC32(P) |
//	+--------+----------+---------+----------+
//
// 每一帧都是自描述的完整提交点；写入时先写帧并 fsync，
// 之后才更新内存状态，因此崩溃/损坏最多丢掉最后一个不完整点。
type Journal struct {
	mu   sync.Mutex
	path string
	f    *os.File
}

// OpenJournal 打开（必要时创建）日志文件用于追加。
func OpenJournal(path string) (*Journal, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open journal %s: %w", path, err)
	}
	return &Journal{path: path, f: f}, nil
}

// Path 返回日志文件路径。
func (j *Journal) Path() string { return j.path }

// Append 序列化并落盘一个完整帧。返回写入的帧序号由调用方写入 Entry.Seq。
func (j *Journal) Append(entry *Entry) error {
	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal journal entry: %w", err)
	}
	if len(payload) > maxFrameSize {
		return fmt.Errorf("journal frame too large: %d", len(payload))
	}

	frame := make([]byte, frameHeader+len(payload)+frameTrailer)
	copy(frame[0:4], journalMagic)
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
	copy(frame[frameHeader:frameHeader+len(payload)], payload)
	checksum := crc32.ChecksumIEEE(payload)
	binary.BigEndian.PutUint32(frame[frameHeader+len(payload):], checksum)

	j.mu.Lock()
	defer j.mu.Unlock()
	if _, err := j.f.Write(frame); err != nil {
		return fmt.Errorf("append journal frame: %w", err)
	}
	if err := j.f.Sync(); err != nil {
		return fmt.Errorf("fsync journal: %w", err)
	}
	return nil
}

// Close 关闭日志文件。
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.f == nil {
		return nil
	}
	err := j.f.Close()
	j.f = nil
	return err
}

// RecoveryReport 描述一次日志扫描的结果。
type RecoveryReport struct {
	// Entries 是从文件开头连续有效的全部条目。
	Entries []Entry
	// Cutoff 是最后一个完整帧之后的字节偏移，即“最近完整点”。
	Cutoff int64
	// CorruptAt 是检测到损坏的偏移；-1 表示日志完好。
	CorruptAt int64
	// Reason 是损坏原因（若有）。
	Reason string
}

// LastCompleteSeq 返回最近完整点对应的日志序号；空日志为 0。
func (r *RecoveryReport) LastCompleteSeq() uint64 {
	if len(r.Entries) == 0 {
		return 0
	}
	return r.Entries[len(r.Entries)-1].Seq
}

// Corrupted 报告日志是否存在损坏/截断。
func (r *RecoveryReport) Corrupted() bool { return r.CorruptAt >= 0 }

// ReadJournal 扫描日志，返回连续有效的条目前缀与最近完整点。
// 遇到第一个坏帧即停止：坏帧之后的字节不再可信，
// Cutoff 之前的前缀保证每一帧都完整且校验通过。
func ReadJournal(path string) (*RecoveryReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &RecoveryReport{CorruptAt: -1}, nil
		}
		return nil, fmt.Errorf("read journal %s: %w", path, err)
	}

	report := &RecoveryReport{CorruptAt: -1}
	off := 0
	for off < len(data) {
		frameStart := off
		if len(data)-off < frameHeader {
			report.CorruptAt = int64(frameStart)
			report.Reason = "truncated frame header"
			break
		}
		if string(data[off:off+4]) != journalMagic {
			report.CorruptAt = int64(frameStart)
			report.Reason = "bad frame magic"
			break
		}
		payloadLen := int(binary.BigEndian.Uint32(data[off+4 : off+8]))
		if payloadLen > maxFrameSize {
			report.CorruptAt = int64(frameStart)
			report.Reason = fmt.Sprintf("implausible frame length %d", payloadLen)
			break
		}
		frameEnd := off + frameHeader + payloadLen + frameTrailer
		if frameEnd > len(data) {
			report.CorruptAt = int64(frameStart)
			report.Reason = "torn frame: missing payload or checksum"
			break
		}
		payload := data[off+frameHeader : off+frameHeader+payloadLen]
		wantCRC := binary.BigEndian.Uint32(data[off+frameHeader+payloadLen : frameEnd])
		if gotCRC := crc32.ChecksumIEEE(payload); gotCRC != wantCRC {
			report.CorruptAt = int64(frameStart)
			report.Reason = "crc32 mismatch"
			break
		}
		var entry Entry
		if err := json.Unmarshal(payload, &entry); err != nil {
			report.CorruptAt = int64(frameStart)
			report.Reason = fmt.Sprintf("undecodable entry: %v", err)
			break
		}
		report.Entries = append(report.Entries, entry)
		off = frameEnd
	}
	report.Cutoff = int64(off)
	return report, nil
}

// Repair 把日志文件截断到最近完整点，丢弃损坏/残缺的尾部，
// 使后续追加不会与坏帧相邻。
func (r *RecoveryReport) Repair(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if r.Cutoff >= info.Size() {
		return nil
	}
	if err := os.Truncate(path, r.Cutoff); err != nil {
		return fmt.Errorf("repair journal %s: %w", path, err)
	}
	return nil
}
