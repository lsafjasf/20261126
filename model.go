// Package clipgroup 实现片段组的事务化编辑、版本快照、
// 可恢复日志以及可重放的 Undo/Redo。
package clipgroup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// Vec2 是片段在画布上的坐标。
type Vec2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Effect 是片段上的一个效果，Params 即“效果参数”。
// ResourceID/ResourceVersion 指向该效果依赖的资源（预设、素材等）。
type Effect struct {
	ID              string             `json:"id"`
	Kind            string             `json:"kind"`
	Enabled         bool               `json:"enabled"`
	Params          map[string]float64 `json:"params,omitempty"`
	ResourceID      string             `json:"resourceId,omitempty"`
	ResourceVersion int64              `json:"resourceVersion,omitempty"`
}

// Clip 是组内的单个片段；Locked 是单项锁定状态。
type Clip struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Locked   bool     `json:"locked"`
	Position Vec2     `json:"position"`
	Effects  []Effect `json:"effects,omitempty"`
	Version  int64    `json:"version"`
}

// Group 是片段组，MemberIDs 决定组成员及其顺序。
type Group struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	MemberIDs []string `json:"memberIds"`
	Position  Vec2     `json:"position"`
	Version   int64    `json:"version"`
}

// Resource 是效果依赖的资源，带独立版本号与可校验内容。
type Resource struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	URI      string            `json:"uri,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
	Checksum string            `json:"checksum,omitempty"`
	Version  int64             `json:"version"`
}

// Document 是编辑器内全部实体的当前状态。
type Document struct {
	Groups    map[string]*Group    `json:"groups"`
	Clips     map[string]*Clip     `json:"clips"`
	Resources map[string]*Resource `json:"resources"`
}

// NewDocument 返回空文档。
func NewDocument() *Document {
	return &Document{
		Groups:    map[string]*Group{},
		Clips:     map[string]*Clip{},
		Resources: map[string]*Resource{},
	}
}

// EntityRef 精确定位某个版本化实体。
type EntityRef struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

// 实体类别常量。
const (
	KindGroup    = "group"
	KindClip     = "clip"
	KindResource = "resource"
)

// Snapshot 是某个片段组聚合的完整版本：
// 组本身、全部成员片段（含锁定状态与效果参数）、以及效果依赖的资源。
// Present=false 表示该组在快照点尚不存在（用于“创建组”事务的 Before）。
type Snapshot struct {
	Present   bool        `json:"present"`
	GroupID   string      `json:"groupId"`
	Group     *Group      `json:"group,omitempty"`
	Clips     []*Clip     `json:"clips,omitempty"`
	Resources []*Resource `json:"resources,omitempty"`
	Refs      []EntityRef `json:"refs"`
}

// BuildSnapshot 捕获 groupID 聚合当前的完整深拷贝版本。
// 组成员缺失或效果引用的依赖资源缺失都会返回错误，
// 这样损坏的聚合不可能进入事务。
func BuildSnapshot(doc *Document, groupID string) (*Snapshot, error) {
	group, ok := doc.Groups[groupID]
	if !ok {
		return &Snapshot{Present: false, GroupID: groupID}, nil
	}

	snap := &Snapshot{Present: true, GroupID: groupID, Group: cloneJSON(group)}
	snap.Refs = append(snap.Refs, EntityRef{KindGroup, group.ID, group.Version})

	resourceIDs := map[string]struct{}{}
	for _, clipID := range group.MemberIDs {
		clip, ok := doc.Clips[clipID]
		if !ok {
			return nil, fmt.Errorf("snapshot group %s: missing member clip %s", groupID, clipID)
		}
		snap.Clips = append(snap.Clips, cloneJSON(clip))
		snap.Refs = append(snap.Refs, EntityRef{KindClip, clip.ID, clip.Version})
		for _, fx := range clip.Effects {
			if fx.ResourceID != "" {
				resourceIDs[fx.ResourceID] = struct{}{}
			}
		}
	}

	ids := make([]string, 0, len(resourceIDs))
	for id := range resourceIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		res, ok := doc.Resources[id]
		if !ok {
			return nil, fmt.Errorf("snapshot group %s: missing dependent resource %s", groupID, id)
		}
		snap.Resources = append(snap.Resources, cloneJSON(res))
		snap.Refs = append(snap.Refs, EntityRef{KindResource, res.ID, res.Version})
	}
	return snap, nil
}

// Apply 把快照中的实体整体写回文档（深拷贝，避免共享底层数据）。
func (s *Snapshot) Apply(doc *Document) error {
	if s == nil {
		return fmt.Errorf("nil snapshot")
	}
	if !s.Present {
		return fmt.Errorf("snapshot for %s is an absent marker; use ApplyAbsentCreate", s.GroupID)
	}
	doc.Groups[s.Group.ID] = cloneJSON(s.Group)
	for _, clip := range s.Clips {
		doc.Clips[clip.ID] = cloneJSON(clip)
	}
	for _, res := range s.Resources {
		doc.Resources[res.ID] = cloneJSON(res)
	}
	return nil
}

// ApplyAbsentCreate 回滚一个“创建组”事务：删除 After 快照里引入的全部实体。
func ApplyAbsentCreate(doc *Document, created *Snapshot) {
	if created == nil {
		return
	}
	delete(doc.Groups, created.GroupID)
	for _, clip := range created.Clips {
		delete(doc.Clips, clip.ID)
	}
	for _, res := range created.Resources {
		delete(doc.Resources, res.ID)
	}
}

// Digest 返回任意状态的规范化 SHA-256 摘要，用于校验重放一致性。
func Digest(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("clipgroup: digest marshal failed: %v", err))
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// canonicalJSON 返回稳定的 JSON 字节（encoding/json 对 map key 排序）。
func canonicalJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("clipgroup: canonical marshal failed: %v", err))
	}
	return b
}

func cloneJSON[T any](in *T) *T {
	if in == nil {
		return nil
	}
	var out T
	b, err := json.Marshal(in)
	if err != nil {
		panic(fmt.Sprintf("clipgroup: clone marshal failed: %v", err))
	}
	if err := json.Unmarshal(b, &out); err != nil {
		panic(fmt.Sprintf("clipgroup: clone unmarshal failed: %v", err))
	}
	return &out
}
