package timeline

import "fmt"

// Stage 记录展开过程中一层父子映射，保留内部/外部速率，
// 而不是把它们压成一个乘积——这是标记不漂移的关键。
type Stage struct {
	CompoundID string `json:"compoundID"`
	ItemID     string `json:"itemID"`
	Speed      Speed  `json:"speed"`
}

// FlatItem 是展开后落在根时间轴上的叶子条目。
type FlatItem struct {
	LeafID        string  `json:"leafID"`
	SourceID      string  `json:"sourceID"`
	Start         int64   `json:"start"`
	Dur           int64   `json:"dur"`
	Freeze        bool    `json:"freeze"`
	Stages        []Stage `json:"stages"`
	InternalSpeed Speed   `json:"internalSpeed"`
	ExternalSpeed Speed   `json:"externalSpeed"`

	// key 用于聚合连续同路径帧；srcIn/freezeSrcIn 供帧查询重放。
	key         string
	srcIn       int64
	freezeSrcIn int64
}

// FlatMarker 是展开后的标记，RootFrame 由分阶段映射逐级 floor 得到。
type FlatMarker struct {
	ID        string `json:"id"`
	RootFrame int64  `json:"rootFrame"`
	Origin    string `json:"origin"`
	Note      string `json:"note,omitempty"`
}

// FlatTimeline 是展开结果。
type FlatTimeline struct {
	Dur     int64        `json:"dur"`
	Items   []FlatItem   `json:"items"`
	Markers []FlatMarker `json:"markers"`
}

// Flatten 以“分阶段 floor 映射”的方式把整个工程展开到根时间轴。
// 它逐帧驱动：先用 Resolver 得到每根帧的真实父子映射，再把连续同路径
// 帧聚合成根坐标上的叶子条目。每层复合引用都保留独立 Stage，
// 禁止把多层速率预先连乘，保证展开前后画面索引完全一致。
func Flatten(p *Project) (*FlatTimeline, error) {
	r, err := NewResolver(p)
	if err != nil {
		return nil, err
	}
	out := &FlatTimeline{Dur: p.Root.Dur}
	for _, m := range p.Root.Markers {
		out.Markers = append(out.Markers, FlatMarker{
			ID: m.ID, RootFrame: m.Offset, Origin: "root", Note: m.Note,
		})
	}

	var group *FlatItem
	flush := func(end int64) {
		if group != nil {
			group.Dur = end - group.Start
			out.Items = append(out.Items, *group)
			group = nil
		}
	}

	for f := int64(0); f < out.Dur; f++ {
		rec, err := r.Frame(f)
		if err != nil {
			return nil, err
		}
		for _, markerID := range rec.Markers {
			if markerID == "" || hasRootMarker(p, markerID) || hasFlatMarker(out.Markers, markerID) {
				continue
			}
			note, origin := markerOrigin(p, markerID)
			out.Markers = append(out.Markers, FlatMarker{
				ID: markerID, RootFrame: f, Origin: origin, Note: note,
			})
		}

		key := groupKey(rec)
		if group != nil && group.key == key {
			continue
		}
		flush(f)
		if rec.Kind == FrameGap {
			continue
		}
		group = &FlatItem{
			key:           key,
			LeafID:        leafID(rec.Path),
			SourceID:      rec.SourceID,
			Start:         f,
			Freeze:        rec.Kind == FrameFreeze,
			Stages:        buildStages(p, rec.Path),
			InternalSpeed: rec.InternalSpeed,
			ExternalSpeed: rec.ExternalSpeed,
			srcIn:         firstSourceFrame(p, rec),
			freezeSrcIn:   rec.SourceFrame,
		}
	}
	flush(out.Dur)
	return out, nil
}

func hasFlatMarker(markers []FlatMarker, id string) bool {
	for _, m := range markers {
		if m.ID == id {
			return true
		}
	}
	return false
}

func groupKey(rec FrameRecord) string {
	return string(rec.Kind) + "|" + joinIDs(rec.Path) + "|" + rec.SourceID +
		"|" + rec.InternalSpeed.String() + "|" + rec.ExternalSpeed.String()
}

func leafID(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return path[len(path)-1]
}

func hasRootMarker(p *Project, id string) bool {
	for _, m := range p.Root.Markers {
		if m.ID == id {
			return true
		}
	}
	return false
}

// markerOrigin 在全工程中反查标记的注释与归属。
func markerOrigin(p *Project, id string) (note, origin string) {
	scan := func(compoundID string, c *Compound) {
		for _, m := range c.Markers {
			if m.ID == id {
				note, origin = m.Note, compoundID
			}
		}
		for _, it := range c.Items {
			for _, m := range it.Markers {
				if m.ID == id {
					note, origin = m.Note, compoundID+"/"+it.ID
				}
			}
		}
	}
	for cid, c := range p.Compounds {
		scan(cid, c)
	}
	scan("root", p.Root)
	return note, origin
}

// buildStages 依据解析路径重建各引用层的速率 Stage。
func buildStages(p *Project, path []string) []Stage {
	if len(path) < 3 {
		return nil
	}
	stages := make([]Stage, 0, len(path)-2)
	c := p.Root
	for _, itemID := range path[1 : len(path)-1] {
		it := findItem(c, itemID)
		if it == nil {
			break
		}
		stages = append(stages, Stage{
			CompoundID: c.ID, ItemID: it.ID, Speed: it.Speed.Normalize(),
		})
		if it.Kind == KindRef {
			c = p.Compounds[it.CompoundID]
		}
	}
	return stages
}

func findItem(c *Compound, id string) *Item {
	for _, it := range c.Items {
		if it.ID == id {
			return it
		}
	}
	return nil
}

// firstSourceFrame 取该叶子在源媒体上的起始帧，供展开后逐帧重放。
func firstSourceFrame(p *Project, rec FrameRecord) int64 {
	for _, c := range p.Compounds {
		if it := findItem(c, leafID(rec.Path)); it != nil {
			return it.SrcIn
		}
	}
	if it := findItem(p.Root, leafID(rec.Path)); it != nil {
		return it.SrcIn
	}
	return rec.SourceFrame
}

// Frame 在展开后的时间轴上查询一根帧，返回与 Resolver.Frame 同构的记录。
// 必须按各父层 Stage 逐级 floor 重放，不能用根偏移做单次映射；
// 冻结段走独立分支：源帧恒定、内部速率记为 1。
func (ft *FlatTimeline) Frame(rootFrame int64) (FrameRecord, error) {
	if rootFrame < 0 || rootFrame >= ft.Dur {
		return FrameRecord{}, fmt.Errorf("timeline: flat frame %d out of bounds [0,%d)", rootFrame, ft.Dur)
	}
	rec := FrameRecord{
		RootFrame:     rootFrame,
		Kind:          FrameGap,
		Path:          []string{"root", "flat"},
		InternalSpeed: SpeedOne,
		ExternalSpeed: SpeedOne,
		CombinedSpeed: SpeedOne,
	}
	for _, fi := range ft.Items {
		if rootFrame < fi.Start || rootFrame >= fi.Start+fi.Dur {
			continue
		}
		local := rootFrame - fi.Start
		for i := len(fi.Stages) - 1; i >= 0; i-- {
			mapped, err := fi.Stages[i].Speed.Map(local)
			if err != nil {
				return FrameRecord{}, err
			}
			local = mapped
		}
		rec.ExternalSpeed = fi.ExternalSpeed
		rec.SourceID = fi.SourceID
		rec.Path = append(rec.Path, fi.LeafID)
		if fi.Freeze {
			rec.Kind = FrameFreeze
			rec.InternalSpeed = SpeedOne
			rec.CombinedSpeed = fi.ExternalSpeed
			rec.SourceFrame = fi.freezeSrcIn
			break
		}
		mapped, err := fi.InternalSpeed.Map(local)
		if err != nil {
			return FrameRecord{}, err
		}
		rec.Kind = FrameMedia
		rec.InternalSpeed = fi.InternalSpeed
		rec.CombinedSpeed = fi.ExternalSpeed.Mul(fi.InternalSpeed)
		rec.SourceFrame = fi.srcIn + mapped
		break
	}
	for _, m := range ft.Markers {
		if m.RootFrame == rootFrame {
			rec.Markers = append(rec.Markers, m.ID)
		}
	}
	return rec, nil
}
