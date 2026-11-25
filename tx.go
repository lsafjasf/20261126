package clipgroup

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// TxRecord 是一个片段组事务的完整证据：
// 操作名、事务 ID，以及提交前后“组 + 成员 + 依赖资源”的完整版本。
// Undo/Redo 直接在 Before/After 两个完整点之间切换，因此不可能只恢复一部分。
type TxRecord struct {
	ID      string    `json:"id"`
	Op      string    `json:"op"`
	GroupID string    `json:"groupId"`
	Before  *Snapshot `json:"before"`
	After   *Snapshot `json:"after"`
}

// StoredVersion 是某个实体的一个不可变历史版本。
type StoredVersion struct {
	Kind    string          `json:"kind"`
	ID      string          `json:"id"`
	Version int64           `json:"version"`
	TxID    string          `json:"txId"`
	Op      string          `json:"op"`
	Data    json.RawMessage `json:"data"`
}

type versionKey struct {
	kind    string
	id      string
	version int64
}

// VersionStore 保存组、成员片段、依赖资源出现过的每一个版本。
type VersionStore struct {
	mu      sync.Mutex
	items   map[versionKey]StoredVersion
	history []versionKey
}

// NewVersionStore 创建空的版本仓库。
func NewVersionStore() *VersionStore {
	return &VersionStore{items: map[versionKey]StoredVersion{}}
}

// SaveSnapshot 把快照中的全部实体版本固化下来；同一版本只保存一次。
func (vs *VersionStore) SaveSnapshot(txID, op string, snap *Snapshot) error {
	if snap == nil || !snap.Present {
		return nil
	}
	vs.mu.Lock()
	defer vs.mu.Unlock()

	put := func(kind, id string, version int64, entity any) error {
		key := versionKey{kind, id, version}
		if _, exists := vs.items[key]; exists {
			return nil
		}
		data, err := json.Marshal(entity)
		if err != nil {
			return fmt.Errorf("version store: marshal %s/%s: %w", kind, id, err)
		}
		vs.items[key] = StoredVersion{
			Kind: kind, ID: id, Version: version,
			TxID: txID, Op: op, Data: data,
		}
		vs.history = append(vs.history, key)
		return nil
	}

	if err := put(KindGroup, snap.Group.ID, snap.Group.Version, snap.Group); err != nil {
		return err
	}
	for _, clip := range snap.Clips {
		if err := put(KindClip, clip.ID, clip.Version, clip); err != nil {
			return err
		}
	}
	for _, res := range snap.Resources {
		if err := put(KindResource, res.ID, res.Version, res); err != nil {
			return err
		}
	}
	return nil
}

// History 返回某实体按版本号升序排列的全部已保存版本。
func (vs *VersionStore) History(kind, id string) []StoredVersion {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	var out []StoredVersion
	for _, rec := range vs.items {
		if rec.Kind == kind && rec.ID == id {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}

// Count 返回已保存的唯一实体版本总数。
func (vs *VersionStore) Count() int {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	return len(vs.items)
}
