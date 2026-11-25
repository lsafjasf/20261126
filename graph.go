package eac3

import (
	"errors"
	"fmt"
	"hash/fnv"
)

var (
	ErrProgramNotFound  = errors.New("eac3: program not found")
	ErrStateNotFound    = errors.New("eac3: program state not found")
	ErrInvalidMode      = errors.New("eac3: invalid program mode")
	ErrAnchorMissing    = errors.New("eac3: independent anchor substream missing")
	ErrDependentMissing = errors.New("eac3: dependent substream missing")
	ErrAmbiguousTarget  = errors.New("eac3: ambiguous switch target")
	ErrInvalidSpec      = errors.New("eac3: invalid program spec")
)

type SubstreamRecord struct {
	Info         FrameInfo `json:"info"`
	Observations int       `json:"observations"`
	FirstOffset  int64     `json:"first_offset"`
	LastOffset   int64     `json:"last_offset"`
}

type Program struct {
	ID          string                     `json:"id"`
	Independent map[uint8]*SubstreamRecord `json:"independent"`
	Dependent   map[uint8]*SubstreamRecord `json:"dependent"`
	Version     uint64                     `json:"version"`
	ObservedAt  int                        `json:"observed_frames"`
}

// Graph 同时保存解码前节目拓扑、可切换状态节点和切换边。
type Graph struct {
	Programs    map[string]*Program `json:"programs"`
	StateOrder  []string            `json:"state_order"`
	States      map[string]*State   `json:"states"`
	Transitions []Switch            `json:"transitions"`
	CurrentID   string              `json:"current_id,omitempty"`
}

func NewGraph() *Graph {
	return &Graph{
		Programs: make(map[string]*Program),
		States:   make(map[string]*State),
	}
}

func defaultProgramID(f FrameInfo) string {
	if f.ProgramID != "" {
		return f.ProgramID
	}
	return fmt.Sprintf("program-%d", f.SubstreamID)
}

// Observe 吸收一个已解析帧头，更新 substream 拓扑。相同 substream 的后续
// 帧刷新最新规格，但不会把逐帧元数据误当成新的节目节点。
func (g *Graph) Observe(f FrameInfo) error {
	if f.Type == SubstreamReserved {
		return ErrFrameType
	}
	normalizeSpec(&f.Spec)
	if f.Spec.SampleRateHz <= 0 || f.Spec.Boundary.SamplesPerFrame <= 0 {
		return fmt.Errorf("%w: sample rate and frame boundary are required", ErrInvalidSpec)
	}
	if g.Programs == nil {
		g.Programs = make(map[string]*Program)
	}
	if g.States == nil {
		g.States = make(map[string]*State)
	}

	id := defaultProgramID(f)
	p := g.Programs[id]
	if p == nil {
		p = &Program{
			ID:          id,
			Independent: make(map[uint8]*SubstreamRecord),
			Dependent:   make(map[uint8]*SubstreamRecord),
		}
		g.Programs[id] = p
	}

	group := p.Independent
	if f.Type == SubstreamDependent {
		group = p.Dependent
	}
	rec := group[f.SubstreamID]
	if rec == nil {
		rec = &SubstreamRecord{Info: f, FirstOffset: f.ByteOffset}
		group[f.SubstreamID] = rec
	} else {
		rec.Info = f
	}
	rec.Observations++
	rec.LastOffset = f.ByteOffset
	p.ObservedAt++
	p.Version++
	return nil
}

func normalizeSpec(spec *ProgramSpec) {
	if spec.Channel.Channels == 0 && spec.Channel.Acmod <= 7 {
		spec.Channel.Channels = acmodChannels[spec.Channel.Acmod] + boolToInt(spec.Channel.LFEOn)
		if len(spec.Channel.Labels) == 0 {
			spec.Channel.Labels = append([]string(nil), acmodLabels[spec.Channel.Acmod]...)
			if spec.Channel.LFEOn {
				spec.Channel.Labels = append(spec.Channel.Labels, "LFE")
			}
		}
	}
	if spec.Boundary.SamplesPerFrame == 0 && spec.Boundary.BlocksPerFrame > 0 {
		spec.Boundary.SamplesPerFrame = int(spec.Boundary.BlocksPerFrame) * 256
	}
}

func stateID(s State) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s.key()))
	return fmt.Sprintf("state-%016x", h.Sum64())
}

// Declare 登记一个可切换的解析后节目状态。复合声道结构必须由上层节目
// 描述给出：dependent 可能增加或替换声道，不能只靠 acmod 相加推断。
func (g *Graph) Declare(s State) (*State, error) {
	if g.States == nil {
		g.States = make(map[string]*State)
	}
	if s.ProgramID == "" {
		return nil, fmt.Errorf("%w: program id is empty", ErrInvalidSpec)
	}
	if s.Mode == "" {
		s.Mode = ModeIndependent
	}
	if s.Mode != ModeIndependent && s.Mode != ModeAssociated && s.Mode != ModeMixed {
		return nil, fmt.Errorf("%w: %q", ErrInvalidMode, s.Mode)
	}
	normalizeSpec(&s.Spec)
	if s.Spec.SampleRateHz <= 0 || s.Spec.Boundary.SamplesPerFrame <= 0 || s.Spec.Channel.Channels <= 0 {
		return nil, fmt.Errorf("%w: sample rate, channels and boundary are required", ErrInvalidSpec)
	}

	if p := g.Programs[s.ProgramID]; p != nil {
		s.TopologyVersion = p.Version
		if s.Anchor != nil {
			if _, ok := p.Independent[s.Anchor.ID]; s.Anchor.Type != SubstreamIndependent || !ok {
				return nil, fmt.Errorf("%w: program=%s ref=%s", ErrAnchorMissing, s.ProgramID, refKey(*s.Anchor))
			}
		}
		for _, ref := range s.DecodeSubstreams {
			if ref.Type != SubstreamDependent {
				continue
			}
			if _, ok := p.Dependent[ref.ID]; !ok {
				return nil, fmt.Errorf("%w: program=%s ref=%s", ErrDependentMissing, s.ProgramID, refKey(ref))
			}
		}
	}

	if s.ID == "" {
		s.ID = stateID(s)
	}
	if existing, ok := g.States[s.ID]; ok {
		return existing, nil
	}
	copyState := s
	if s.Anchor != nil {
		anchor := *s.Anchor
		copyState.Anchor = &anchor
	}
	copyState.DecodeSubstreams = append([]SubstreamRef(nil), s.DecodeSubstreams...)
	copyState.RenderSubstreams = append([]SubstreamRef(nil), s.RenderSubstreams...)
	g.States[s.ID] = &copyState
	g.StateOrder = append(g.StateOrder, s.ID)
	return &copyState, nil
}

func (g *Graph) Current() *State {
	if g == nil || g.CurrentID == "" {
		return nil
	}
	return g.States[g.CurrentID]
}
