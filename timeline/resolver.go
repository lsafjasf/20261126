package timeline

import "fmt"

// Resolver 在已校验、无环的工程上做父子时间映射。
type Resolver struct {
	project *Project
}

// NewResolver 先做结构校验与循环检测，任一失败都拒绝构建，
// 保证后续帧解析不会进入死循环。
func NewResolver(p *Project) (*Resolver, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := p.CheckCycles(); err != nil {
		return nil, err
	}
	return &Resolver{project: p}, nil
}

// Locate 在给定时间轴上找到覆盖局部帧 local 的条目；无覆盖时返回 nil。
func Locate(c *Compound, local int64) *Item {
	for _, it := range c.Items {
		if local >= it.Start && local < it.Start+it.Dur {
			return it
		}
	}
	return nil
}

// Frame 解析根时间轴上的一帧，返回从根到源媒体的完整映射记录。
func (r *Resolver) Frame(rootFrame int64) (FrameRecord, error) {
	if rootFrame < 0 || rootFrame >= r.project.Root.Dur {
		return FrameRecord{}, fmt.Errorf("timeline: root frame %d out of bounds [0,%d)", rootFrame, r.project.Root.Dur)
	}
	rec := FrameRecord{
		RootFrame:     rootFrame,
		Kind:          FrameGap,
		Path:          []string{"root"},
		InternalSpeed: SpeedOne,
		ExternalSpeed: SpeedOne,
		CombinedSpeed: SpeedOne,
		product:       SpeedOne,
	}
	if err := r.resolve(r.project.Root, rootFrame, &rec, 0); err != nil {
		return FrameRecord{}, err
	}
	return rec, nil
}

// resolve 递归向下解析。external 是从根到当前层入口处的速率累积，
// 因此叶子看到的 ExternalSpeed 就是所有父层速率的乘积。
func (r *Resolver) resolve(c *Compound, local int64, rec *FrameRecord, depth int) error {
	if depth > maxResolveDepth {
		return fmt.Errorf("timeline: nesting exceeds depth %d", maxResolveDepth)
	}
	for _, m := range c.Markers {
		if m.Offset == local {
			rec.Markers = append(rec.Markers, m.ID)
		}
	}

	it := Locate(c, local)
	if it == nil {
		return nil // 空隙：保持 FrameGap
	}
	rec.Path = append(rec.Path, it.ID)
	for _, m := range it.Markers {
		if m.Offset == local-it.Start {
			rec.Markers = append(rec.Markers, m.ID)
		}
	}

	switch it.Kind {
	case KindRef:
		// 引用层自身的速率即作用于子时间轴的“外部速率”。
		layerSpeed := it.Speed.Normalize()
		nextProduct := rec.product.Mul(layerSpeed)
		child := r.project.Compounds[it.CompoundID]
		childLocal, err := layerSpeed.Map(local - it.Start)
		if err != nil {
			return err
		}
		rec.product = nextProduct
		return r.resolve(child, childLocal, rec, depth+1)

	case KindLeaf:
		return r.resolveLeaf(it, local-it.Start, rec)

	default:
		return fmt.Errorf("timeline: item %q has unknown kind %q", it.ID, it.Kind)
	}
}

// resolveLeaf 处理叶子：冻结段与变速段是两条独立分支。
func (r *Resolver) resolveLeaf(it *Item, localOffset int64, rec *FrameRecord) error {
	internal := it.Speed.Normalize()

	// 冻结段：速率恒为 1，源帧固定，不参与任何变速乘积。
	if it.Freeze {
		rec.Kind = FrameFreeze
		rec.SourceID = it.SourceID
		rec.SourceFrame = it.SrcIn
		rec.InternalSpeed = SpeedOne
		rec.ExternalSpeed = rec.product
		rec.CombinedSpeed = rec.product
		return nil
	}

	mapped, err := internal.Map(localOffset)
	if err != nil {
		return err
	}
	rec.Kind = FrameMedia
	rec.SourceID = it.SourceID
	rec.SourceFrame = it.SrcIn + mapped
	rec.InternalSpeed = internal
	rec.ExternalSpeed = rec.product
	rec.CombinedSpeed = rec.product.Mul(internal)
	return nil
}

// Frames 解析根时间轴上 [0, Dur) 的全部帧。
func (r *Resolver) Frames() ([]FrameRecord, error) {
	out := make([]FrameRecord, 0, r.project.Root.Dur)
	for f := int64(0); f < r.project.Root.Dur; f++ {
		rec, err := r.Frame(f)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}
