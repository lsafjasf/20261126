package clipgroup

import (
	"encoding/json"
	"fmt"
	"os"
)

// 支持的组操作类型。
const (
	OpCreateGroup    = "create_group"
	OpMoveGroup      = "move_group"
	OpSetClipLocked  = "set_clip_locked"
	OpSetEffectParam = "set_effect_param"
	OpReviseResource = "revise_resource"
)

// ClipSpec 描述建组时的一个片段。
type ClipSpec struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Locked   bool     `json:"locked"`
	Position Vec2     `json:"position"`
	Effects  []Effect `json:"effects,omitempty"`
}

// GroupSpec 描述一个新片段组及其成员、依赖资源。
type GroupSpec struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Position  Vec2       `json:"position"`
	Clips     []ClipSpec `json:"clips"`
	Resources []Resource `json:"resources,omitempty"`
}

// Op 是可 JSON 序列化、可重放的用户操作。
// 不同 Type 使用其中相关字段。
type Op struct {
	Type       string            `json:"type"`
	GroupID    string            `json:"groupId,omitempty"`
	Spec       *GroupSpec        `json:"spec,omitempty"`
	Delta      Vec2              `json:"delta,omitempty"`
	ClipID     string            `json:"clipId,omitempty"`
	Locked     bool              `json:"locked,omitempty"`
	EffectID   string            `json:"effectId,omitempty"`
	Key        string            `json:"key,omitempty"`
	Value      float64           `json:"value,omitempty"`
	ResourceID string            `json:"resourceId,omitempty"`
	Meta       map[string]string `json:"meta,omitempty"`
}

// CreateGroupOp 构造“创建片段组”操作。
func CreateGroupOp(spec GroupSpec) Op {
	return Op{Type: OpCreateGroup, GroupID: spec.ID, Spec: &spec}
}

// MoveGroupOp 构造“整体移动片段组”操作：组原点与每个成员一起平移。
func MoveGroupOp(groupID string, delta Vec2) Op {
	return Op{Type: OpMoveGroup, GroupID: groupID, Delta: delta}
}

// SetClipLockedOp 构造“设置组内单项锁定状态”操作。
func SetClipLockedOp(groupID, clipID string, locked bool) Op {
	return Op{Type: OpSetClipLocked, GroupID: groupID, ClipID: clipID, Locked: locked}
}

// SetEffectParamOp 构造“修改某个效果参数”操作。
func SetEffectParamOp(groupID, clipID, effectID, key string, value float64) Op {
	return Op{
		Type: OpSetEffectParam, GroupID: groupID, ClipID: clipID,
		EffectID: effectID, Key: key, Value: value,
	}
}

// ReviseResourceOp 构造“修订依赖资源元数据”操作（资源版本在提交时统一递增）。
func ReviseResourceOp(groupID, resourceID string, meta map[string]string) Op {
	return Op{Type: OpReviseResource, GroupID: groupID, ResourceID: resourceID, Meta: meta}
}

// Do 执行一个可重放操作：在事务中应用、原子提交，并记录到重放脚本。
func (e *Editor) Do(op Op) (*TxRecord, error) {
	mutate, err := op.mutator()
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	tx, err := e.commitLocked(op, mutate)
	if err != nil {
		return nil, err
	}
	e.script.Actions = append(e.script.Actions, Action{Kind: ActionDo, Op: &op})
	return tx, nil
}

func (op Op) mutator() (func(*Aggregate) error, error) {
	switch op.Type {
	case OpCreateGroup:
		if op.Spec == nil {
			return nil, fmt.Errorf("clipgroup: create_group requires spec")
		}
		return createGroupMutator(*op.Spec), nil
	case OpMoveGroup:
		return func(agg *Aggregate) error {
			agg.Group.Position.X += op.Delta.X
			agg.Group.Position.Y += op.Delta.Y
			for _, id := range agg.Group.MemberIDs {
				clip, err := agg.Clip(id)
				if err != nil {
					return err
				}
				clip.Position.X += op.Delta.X
				clip.Position.Y += op.Delta.Y
			}
			return nil
		}, nil
	case OpSetClipLocked:
		return func(agg *Aggregate) error {
			clip, err := agg.Clip(op.ClipID)
			if err != nil {
				return err
			}
			clip.Locked = op.Locked
			return nil
		}, nil
	case OpSetEffectParam:
		return func(agg *Aggregate) error {
			clip, err := agg.Clip(op.ClipID)
			if err != nil {
				return err
			}
			for i := range clip.Effects {
				fx := &clip.Effects[i]
				if fx.ID != op.EffectID {
					continue
				}
				if fx.Params == nil {
					fx.Params = map[string]float64{}
				}
				fx.Params[op.Key] = op.Value
				return nil
			}
			return fmt.Errorf("effect %s not found on clip %s", op.EffectID, op.ClipID)
		}, nil
	case OpReviseResource:
		return func(agg *Aggregate) error {
			id := op.ResourceID
			if id == "" {
				return fmt.Errorf("revise_resource requires resourceId")
			}
			res, err := agg.Resource(id)
			if err != nil {
				return err
			}
			if res.Meta == nil {
				res.Meta = map[string]string{}
			}
			for k, v := range op.Meta {
				res.Meta[k] = v
			}
			return nil
		}, nil
	default:
		return nil, fmt.Errorf("clipgroup: unknown op type %q", op.Type)
	}
}

func createGroupMutator(spec GroupSpec) func(*Aggregate) error {
	return func(agg *Aggregate) error {
		if spec.ID == "" {
			return fmt.Errorf("group spec requires id")
		}
		if len(spec.Clips) == 0 {
			return fmt.Errorf("group %s requires at least one clip", spec.ID)
		}
		agg.Group = &Group{
			ID:       spec.ID,
			Name:     spec.Name,
			Position: spec.Position,
		}
		for _, cs := range spec.Clips {
			if cs.ID == "" {
				return fmt.Errorf("clip spec requires id in group %s", spec.ID)
			}
			if _, dup := agg.Clips[cs.ID]; dup {
				return fmt.Errorf("duplicate clip id %s", cs.ID)
			}
			effects := make([]Effect, len(cs.Effects))
			copy(effects, cs.Effects)
			clip := &Clip{
				ID:       cs.ID,
				Name:     cs.Name,
				Locked:   cs.Locked,
				Position: cs.Position,
				Effects:  effects,
			}
			agg.Clips[clip.ID] = clip
			agg.Group.MemberIDs = append(agg.Group.MemberIDs, clip.ID)
		}
		for i := range spec.Resources {
			res := cloneJSON(&spec.Resources[i])
			if res.ID == "" {
				return fmt.Errorf("resource spec requires id")
			}
			if _, dup := agg.Resources[res.ID]; dup {
				return fmt.Errorf("duplicate resource id %s", res.ID)
			}
			agg.Resources[res.ID] = res
		}
		return nil
	}
}

// ActionKind 是重放脚本中的动作类型。
type ActionKind string

const (
	ActionDo   ActionKind = "do"
	ActionUndo ActionKind = "undo"
	ActionRedo ActionKind = "redo"
)

// Action 是一步可重放的用户动作。
type Action struct {
	Kind ActionKind `json:"kind"`
	Op   *Op        `json:"op,omitempty"`
}

// Script 是一段交错 Do/Undo/Redo 序列，可以序列化、保存、重放。
type Script struct {
	Actions []Action `json:"actions"`
}

// Script 返回编辑器自创建以来记录的全部用户动作（深拷贝）。
func (e *Editor) Script() Script {
	e.mu.Lock()
	defer e.mu.Unlock()
	return *cloneJSON(&e.script)
}

// ScriptJSON 返回脚本的规范化 JSON。
func (e *Editor) ScriptJSON() ([]byte, error) {
	e.mu.Lock()
	script := *cloneJSON(&e.script)
	e.mu.Unlock()
	return json.MarshalIndent(script, "", "  ")
}

// SaveScript 把重放脚本写入文件。
func SaveScript(path string, script Script) error {
	b, err := json.MarshalIndent(script, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// LoadScript 读取脚本文件。
func LoadScript(path string) (Script, error) {
	var script Script
	b, err := os.ReadFile(path)
	if err != nil {
		return script, err
	}
	if err := json.Unmarshal(b, &script); err != nil {
		return script, err
	}
	return script, nil
}

// Replay 在纯内存编辑器上重放整段脚本。
func Replay(script Script) (*Editor, error) {
	return ReplayAt(script, "")
}

// ReplayAt 在指定日志路径上重放脚本；path 为空时只走内存。
// 重放产生的 Do/Undo/Redo 会再次写入日志，因此也验证了日志路径。
func ReplayAt(script Script, path string) (*Editor, error) {
	var ed *Editor
	if path == "" {
		ed = NewEditor()
	} else {
		var err error
		ed, _, err = OpenEditor(path)
		if err != nil {
			return nil, err
		}
	}
	for i, action := range script.Actions {
		switch action.Kind {
		case ActionDo:
			if action.Op == nil {
				return nil, fmt.Errorf("replay action %d: do without op", i)
			}
			if _, err := ed.Do(*action.Op); err != nil {
				return nil, fmt.Errorf("replay action %d (%s): %w", i, action.Kind, err)
			}
		case ActionUndo:
			if _, err := ed.Undo(); err != nil {
				return nil, fmt.Errorf("replay action %d (undo): %w", i, err)
			}
		case ActionRedo:
			if _, err := ed.Redo(); err != nil {
				return nil, fmt.Errorf("replay action %d (redo): %w", i, err)
			}
		default:
			return nil, fmt.Errorf("replay action %d: unknown kind %q", i, action.Kind)
		}
	}
	return ed, nil
}
