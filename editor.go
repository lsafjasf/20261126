package clipgroup

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// 可预期的错误。
var (
	ErrNothingToUndo = errors.New("clipgroup: undo stack is empty")
	ErrNothingToRedo = errors.New("clipgroup: redo stack is empty")
)

// Aggregate 是一个组事务可以读写的隔离工作区：
// 组、按 ID 索引的成员片段、依赖资源。它们都是深拷贝，
// 变更函数返回前不会触碰线上文档，因此不可能产生部分提交。
type Aggregate struct {
	Group     *Group
	Clips     map[string]*Clip
	Resources map[string]*Resource

	groupID string
}

// Clip 取得组内指定片段；不属于该组时返回错误。
func (a *Aggregate) Clip(id string) (*Clip, error) {
	c, ok := a.Clips[id]
	if !ok {
		return nil, fmt.Errorf("clip %s is not part of group %s", id, a.groupID)
	}
	return c, nil
}

// Resource 取得该组依赖的指定资源。
func (a *Aggregate) Resource(id string) (*Resource, error) {
	r, ok := a.Resources[id]
	if !ok {
		return nil, fmt.Errorf("resource %s is not a dependency of group %s", id, a.groupID)
	}
	return r, nil
}

// Snapshot 把工作区整理成完整快照，并校验组/成员/依赖关系完整。
func (a *Aggregate) Snapshot() (*Snapshot, error) {
	if a.Group == nil {
		return nil, fmt.Errorf("group %s aggregate has no group", a.groupID)
	}
	if a.Group.ID != a.groupID {
		return nil, fmt.Errorf("group id changed: %s -> %s", a.groupID, a.Group.ID)
	}
	snap := &Snapshot{Present: true, GroupID: a.groupID, Group: cloneJSON(a.Group)}
	snap.Refs = append(snap.Refs, EntityRef{KindGroup, a.Group.ID, a.Group.Version})

	seen := map[string]struct{}{}
	for _, clipID := range a.Group.MemberIDs {
		if _, dup := seen[clipID]; dup {
			return nil, fmt.Errorf("duplicate member %s", clipID)
		}
		seen[clipID] = struct{}{}
		clip, ok := a.Clips[clipID]
		if !ok {
			return nil, fmt.Errorf("missing member clip %s", clipID)
		}
		snap.Clips = append(snap.Clips, cloneJSON(clip))
		snap.Refs = append(snap.Refs, EntityRef{KindClip, clip.ID, clip.Version})
	}

	resourceIDs := map[string]struct{}{}
	for _, clip := range a.Clips {
		for _, fx := range clip.Effects {
			if fx.ResourceID != "" {
				resourceIDs[fx.ResourceID] = struct{}{}
			}
		}
	}
	for id := range a.Resources {
		resourceIDs[id] = struct{}{}
	}
	ids := make([]string, 0, len(resourceIDs))
	for id := range resourceIDs {
		ids = append(ids, id)
	}
	for _, id := range ids {
		res, ok := a.Resources[id]
		if !ok {
			return nil, fmt.Errorf("missing dependent resource %s", id)
		}
		snap.Resources = append(snap.Resources, cloneJSON(res))
		snap.Refs = append(snap.Refs, EntityRef{KindResource, res.ID, res.Version})
	}
	return snap, nil
}

// Editor 持有文档当前状态、Undo/Redo 栈、版本仓库与持久化日志。
type Editor struct {
	mu sync.Mutex

	doc *Document

	journal *Journal
	path    string

	txs  map[string]*TxRecord
	undo []string // 栈顶在末尾
	redo []string // 栈顶在末尾

	seq     uint64 // 已写入的最大日志序号
	txCount int
	rev     int64 // 已分配的最大实体版本号

	versions *VersionStore
	script   Script
	now      func() time.Time
}

// NewEditor 创建不带磁盘日志的内存编辑器（仍完整支持事务与 Undo/Redo）。
func NewEditor() *Editor {
	return &Editor{
		doc:      NewDocument(),
		txs:      map[string]*TxRecord{},
		versions: NewVersionStore(),
		now:      time.Now,
	}
}

// RecoverEditor 只从日志前缀重放状态，不修复、不打开文件。
// 返回的报告指出最近完整点与损坏位置。
func RecoverEditor(path string) (*Editor, *RecoveryReport, error) {
	report, err := ReadJournal(path)
	if err != nil {
		return nil, nil, err
	}
	ed, err := editorFromEntries(report.Entries)
	if err != nil {
		return nil, report, err
	}
	ed.path = path
	return ed, report, nil
}

// OpenEditor 读取日志、把损坏尾部截断修复到最近完整点，
// 然后以追加模式打开日志，可继续处理新的编辑与 Undo/Redo。
func OpenEditor(path string) (*Editor, *RecoveryReport, error) {
	report, err := ReadJournal(path)
	if err != nil {
		return nil, nil, err
	}
	if report.Corrupted() {
		if err := report.Repair(path); err != nil {
			return nil, report, err
		}
	}
	ed, err := editorFromEntries(report.Entries)
	if err != nil {
		return nil, report, err
	}
	j, err := OpenJournal(path)
	if err != nil {
		return nil, report, err
	}
	ed.journal = j
	ed.path = path
	return ed, report, nil
}

func editorFromEntries(entries []Entry) (*Editor, error) {
	ed := NewEditor()
	for i := range entries {
		entry := entries[i]
		if entry.Seq > ed.seq {
			ed.seq = entry.Seq
		}
		switch entry.Kind {
		case EntryCommit:
			tx := entry.Tx
			if tx == nil {
				return nil, fmt.Errorf("replay seq %d: commit without tx", entry.Seq)
			}
			if _, dup := ed.txs[tx.ID]; dup {
				return nil, fmt.Errorf("replay seq %d: duplicate tx %s", entry.Seq, tx.ID)
			}
			ed.txs[tx.ID] = tx
			ed.txCount++
			if err := applyTxAfter(ed.doc, tx); err != nil {
				return nil, fmt.Errorf("replay tx %s after: %w", tx.ID, err)
			}
			if err := ed.versions.SaveSnapshot(tx.ID, tx.Op, tx.Before); err != nil {
				return nil, err
			}
			if err := ed.versions.SaveSnapshot(tx.ID, tx.Op, tx.After); err != nil {
				return nil, err
			}
			observeRev(ed, tx.Before)
			observeRev(ed, tx.After)
			ed.undo = append(ed.undo, tx.ID)
			ed.redo = nil

		case EntryUndo:
			if len(ed.undo) == 0 || ed.undo[len(ed.undo)-1] != entry.TargetID {
				return nil, fmt.Errorf("replay seq %d: undo target %s is not stack top", entry.Seq, entry.TargetID)
			}
			tx := ed.txs[entry.TargetID]
			if tx == nil {
				return nil, fmt.Errorf("replay seq %d: unknown undo target %s", entry.Seq, entry.TargetID)
			}
			if err := applyTxBefore(ed.doc, tx); err != nil {
				return nil, fmt.Errorf("replay tx %s before: %w", tx.ID, err)
			}
			ed.undo = ed.undo[:len(ed.undo)-1]
			ed.redo = append(ed.redo, tx.ID)

		case EntryRedo:
			if len(ed.redo) == 0 || ed.redo[len(ed.redo)-1] != entry.TargetID {
				return nil, fmt.Errorf("replay seq %d: redo target %s is not stack top", entry.Seq, entry.TargetID)
			}
			tx := ed.txs[entry.TargetID]
			if tx == nil {
				return nil, fmt.Errorf("replay seq %d: unknown redo target %s", entry.Seq, entry.TargetID)
			}
			if err := applyTxAfter(ed.doc, tx); err != nil {
				return nil, fmt.Errorf("replay tx %s after: %w", tx.ID, err)
			}
			ed.redo = ed.redo[:len(ed.redo)-1]
			ed.undo = append(ed.undo, tx.ID)

		default:
			return nil, fmt.Errorf("replay seq %d: unknown entry kind %q", entry.Seq, entry.Kind)
		}
	}
	return ed, nil
}

func observeRev(ed *Editor, snap *Snapshot) {
	if snap == nil {
		return
	}
	for _, ref := range snap.Refs {
		if ref.Version > ed.rev {
			ed.rev = ref.Version
		}
	}
}

func applyTxAfter(doc *Document, tx *TxRecord) error {
	if tx.After == nil || !tx.After.Present {
		return fmt.Errorf("tx %s has no present After snapshot", tx.ID)
	}
	return tx.After.Apply(doc)
}

func applyTxBefore(doc *Document, tx *TxRecord) error {
	if tx.Before == nil {
		return fmt.Errorf("tx %s has no Before snapshot", tx.ID)
	}
	if tx.Before.Present {
		return tx.Before.Apply(doc)
	}
	ApplyAbsentCreate(doc, tx.After)
	return nil
}

// Close 关闭底层日志文件。
func (e *Editor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.journal == nil {
		return nil
	}
	return e.journal.Close()
}

// Transact 在一个隔离的组聚合上执行 mutate 并原子提交。
// mutate 返回错误、快照不完整或日志落盘失败时，线上状态保持不变。
func (e *Editor) Transact(op Op, mutate func(agg *Aggregate) error) (*TxRecord, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.commitLocked(op, mutate)
}

func (e *Editor) commitLocked(op Op, mutate func(agg *Aggregate) error) (*TxRecord, error) {
	groupID := op.GroupID
	if op.Type == OpCreateGroup && op.Spec != nil {
		groupID = op.Spec.ID
	}
	if groupID == "" {
		return nil, errors.New("clipgroup: op requires group id")
	}

	before, err := BuildSnapshot(e.doc, groupID)
	if err != nil {
		return nil, err
	}
	if op.Type == OpCreateGroup && before.Present {
		return nil, fmt.Errorf("clipgroup: group %s already exists", groupID)
	}
	if op.Type != OpCreateGroup && !before.Present {
		return nil, fmt.Errorf("clipgroup: group %s not found", groupID)
	}

	agg := &Aggregate{
		groupID:   groupID,
		Clips:     map[string]*Clip{},
		Resources: map[string]*Resource{},
	}
	if before.Present {
		agg.Group = cloneJSON(before.Group)
		for _, clip := range before.Clips {
			agg.Clips[clip.ID] = cloneJSON(clip)
		}
		for _, res := range before.Resources {
			agg.Resources[res.ID] = cloneJSON(res)
		}
	}

	// 关键：用户回调只接触隔离副本。任何错误都在这里中止，
	// e.doc、版本号、栈、日志全部没有变化。
	if err := mutate(agg); err != nil {
		return nil, fmt.Errorf("clipgroup: op %q aborted, no partial commit: %w", op.Type, err)
	}

	after, err := agg.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("clipgroup: op %q produced invalid aggregate: %w", op.Type, err)
	}

	changed := collectChangedVersions(before, after)
	if len(changed) > 0 {
		e.rev++
		for _, ref := range changed {
			setEntityVersion(after, ref.Kind, ref.ID, e.rev)
		}
		after.Refs = rebuildRefs(after)
	}

	e.txCount++
	tx := &TxRecord{
		ID:      fmt.Sprintf("tx-%d", e.txCount),
		Op:      op.Type,
		GroupID: groupID,
		Before:  before,
		After:   after,
	}
	entry := &Entry{Seq: e.seq + 1, Kind: EntryCommit, At: e.now(), Tx: tx}

	// 先持久化完整帧并 fsync，再切换内存状态。
	if e.journal != nil {
		if err := e.journal.Append(entry); err != nil {
			e.txCount--
			return nil, fmt.Errorf("clipgroup: commit refused, in-memory state unchanged: %w", err)
		}
	}

	if err := applyTxAfter(e.doc, tx); err != nil {
		// 理论上不会发生：快照来自已校验聚合。保守起见不推进栈。
		return nil, fmt.Errorf("clipgroup: fatal apply after commit: %w", err)
	}
	e.seq = entry.Seq
	e.txs[tx.ID] = tx
	e.undo = append(e.undo, tx.ID)
	e.redo = nil
	_ = e.versions.SaveSnapshot(tx.ID, tx.Op, tx.Before)
	_ = e.versions.SaveSnapshot(tx.ID, tx.Op, tx.After)
	return tx, nil
}

// Undo 原子恢复栈顶事务的 Before 完整点（组、成员、依赖资源一起恢复）。
func (e *Editor) Undo() (*TxRecord, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.undo) == 0 {
		return nil, ErrNothingToUndo
	}
	id := e.undo[len(e.undo)-1]
	tx := e.txs[id]
	if tx == nil {
		return nil, fmt.Errorf("clipgroup: corrupt in-memory history: missing tx %s", id)
	}

	entry := &Entry{Seq: e.seq + 1, Kind: EntryUndo, At: e.now(), TargetID: id}
	if e.journal != nil {
		if err := e.journal.Append(entry); err != nil {
			return nil, fmt.Errorf("clipgroup: undo not persisted, state unchanged: %w", err)
		}
	}
	if err := applyTxBefore(e.doc, tx); err != nil {
		return nil, err
	}
	e.seq = entry.Seq
	e.undo = e.undo[:len(e.undo)-1]
	e.redo = append(e.redo, id)
	e.script.Actions = append(e.script.Actions, Action{Kind: ActionUndo})
	return tx, nil
}

// Redo 原子重放栈顶事务的 After 完整点。
func (e *Editor) Redo() (*TxRecord, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.redo) == 0 {
		return nil, ErrNothingToRedo
	}
	id := e.redo[len(e.redo)-1]
	tx := e.txs[id]
	if tx == nil {
		return nil, fmt.Errorf("clipgroup: corrupt in-memory redo stack: missing tx %s", id)
	}

	entry := &Entry{Seq: e.seq + 1, Kind: EntryRedo, At: e.now(), TargetID: id}
	if e.journal != nil {
		if err := e.journal.Append(entry); err != nil {
			return nil, fmt.Errorf("clipgroup: redo not persisted, state unchanged: %w", err)
		}
	}
	if err := applyTxAfter(e.doc, tx); err != nil {
		return nil, err
	}
	e.seq = entry.Seq
	e.redo = e.redo[:len(e.redo)-1]
	e.undo = append(e.undo, id)
	e.script.Actions = append(e.script.Actions, Action{Kind: ActionRedo})
	return tx, nil
}

// collectChangedVersions 用提交前的旧版本号比较实体内容，
// 返回内容确实变化、需要分配新版本的实体引用。
func collectChangedVersions(before, after *Snapshot) []EntityRef {
	var changed []EntityRef
	oldClips := map[string]*Clip{}
	oldRes := map[string]*Resource{}
	var oldGroup *Group
	if before != nil && before.Present {
		oldGroup = before.Group
		for _, c := range before.Clips {
			oldClips[c.ID] = c
		}
		for _, r := range before.Resources {
			oldRes[r.ID] = r
		}
	}

	if after.Group != nil {
		if oldGroup == nil || !reflect.DeepEqual(oldGroup, after.Group) {
			changed = append(changed, EntityRef{KindGroup, after.Group.ID, 0})
		}
	}
	for _, c := range after.Clips {
		old := oldClips[c.ID]
		if old == nil || !reflect.DeepEqual(old, c) {
			changed = append(changed, EntityRef{KindClip, c.ID, 0})
		}
	}
	for _, r := range after.Resources {
		old := oldRes[r.ID]
		if old == nil || !reflect.DeepEqual(old, r) {
			changed = append(changed, EntityRef{KindResource, r.ID, 0})
		}
	}
	return changed
}

func setEntityVersion(snap *Snapshot, kind, id string, version int64) {
	switch kind {
	case KindGroup:
		snap.Group.Version = version
	case KindClip:
		for _, c := range snap.Clips {
			if c.ID == id {
				c.Version = version
			}
		}
	case KindResource:
		for _, r := range snap.Resources {
			if r.ID == id {
				r.Version = version
			}
		}
	}
}

func rebuildRefs(snap *Snapshot) []EntityRef {
	refs := make([]EntityRef, 0, 1+len(snap.Clips)+len(snap.Resources))
	refs = append(refs, EntityRef{KindGroup, snap.Group.ID, snap.Group.Version})
	for _, c := range snap.Clips {
		refs = append(refs, EntityRef{KindClip, c.ID, c.Version})
	}
	for _, r := range snap.Resources {
		refs = append(refs, EntityRef{KindResource, r.ID, r.Version})
	}
	return refs
}

// Digest 返回当前文档状态的内容摘要，是重放等价性的主要判据。
func (e *Editor) Digest() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return Digest(e.doc)
}

// Document 返回当前文档的深拷贝。
func (e *Editor) Document() Document {
	e.mu.Lock()
	defer e.mu.Unlock()
	return *cloneJSON(e.doc)
}

// Versions 返回版本仓库。
func (e *Editor) Versions() *VersionStore { return e.versions }

// UndoDepth 返回可撤销事务数。
func (e *Editor) UndoDepth() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.undo)
}

// RedoDepth 返回可重做事务数。
func (e *Editor) RedoDepth() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.redo)
}

// CanUndo 报告是否可撤销。
func (e *Editor) CanUndo() bool { return e.UndoDepth() > 0 }

// CanRedo 报告是否可重做。
func (e *Editor) CanRedo() bool { return e.RedoDepth() > 0 }

// Path 返回持久化日志路径（内存编辑器为空）。
func (e *Editor) Path() string { return e.path }

// Clip 查询组内片段当前状态的深拷贝。
func (e *Editor) Clip(groupID, clipID string) (Clip, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	g, ok := e.doc.Groups[groupID]
	if !ok {
		return Clip{}, fmt.Errorf("group %s not found", groupID)
	}
	found := false
	for _, id := range g.MemberIDs {
		if id == clipID {
			found = true
		}
	}
	if !found {
		return Clip{}, fmt.Errorf("clip %s not in group %s", clipID, groupID)
	}
	c, ok := e.doc.Clips[clipID]
	if !ok {
		return Clip{}, fmt.Errorf("clip %s missing in document", clipID)
	}
	return *cloneJSON(c), nil
}

// Group 查询组当前状态的深拷贝。
func (e *Editor) Group(groupID string) (Group, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	g, ok := e.doc.Groups[groupID]
	if !ok {
		return Group{}, fmt.Errorf("group %s not found", groupID)
	}
	return *cloneJSON(g), nil
}

// Resource 查询依赖资源当前状态的深拷贝。
func (e *Editor) Resource(id string) (Resource, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.doc.Resources[id]
	if !ok {
		return Resource{}, fmt.Errorf("resource %s not found", id)
	}
	return *cloneJSON(r), nil
}
