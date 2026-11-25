package timeline

// NaiveMarker 是“朴素连乘”展开的标记位置：把所有内外速率先乘成一个
// 有理数，再做一次 floor。它会与逐级 floor 的正确结果产生 off-by-one。
type NaiveMarker struct {
	ID            string `json:"id"`
	RootFrame     int64  `json:"rootFrame"`
	CombinedSpeed Speed  `json:"combinedSpeed"`
}

// NaiveFlattenMarkers 复现错误做法：内外变速一次性连乘、单次取整。
// 仅用于漂移对照，不要用于真实时间映射。
func NaiveFlattenMarkers(p *Project) ([]NaiveMarker, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := p.CheckCycles(); err != nil {
		return nil, err
	}
	var out []NaiveMarker
	for _, it := range p.Root.Items {
		naiveEmit(p, "root", it, it.Start, SpeedOne, &out)
	}
	return out, nil
}

func naiveEmit(p *Project, compoundID string, it *Item, rootStart int64,
	external Speed, out *[]NaiveMarker) {

	switch it.Kind {
	case KindRef:
		layer := it.Speed.Normalize()
		child := p.Compounds[it.CompoundID]
		for _, ci := range child.Items {
			naiveEmit(p, it.CompoundID, ci, rootStart, external.Mul(layer), out)
		}
	case KindLeaf:
		for _, m := range it.Markers {
			// 冻结段不参与速率乘积：这是朴素实现也必须遵守的独立分支。
			mapped := int64(0)
			combined := external
			if !it.Freeze {
				combined = external.Mul(it.Speed.Normalize())
				mapped, _ = combined.Map(m.Offset)
			}
			*out = append(*out, NaiveMarker{
				ID: m.ID, RootFrame: rootStart + mapped, CombinedSpeed: combined,
			})
		}
	}
}

// MarkerDrift 描述同一标记在两种展开方式下的根帧差异。
type MarkerDrift struct {
	MarkerID    string `json:"markerID"`
	CorrectRoot int64  `json:"correctRoot"`
	NaiveRoot   int64  `json:"naiveRoot"`
	DriftFrames int64  `json:"driftFrames"`
}

// CompareMarkers 对比分阶段展开与朴素连乘展开的标记位置，返回所有漂移项。
func CompareMarkers(p *Project) ([]MarkerDrift, error) {
	flat, err := Flatten(p)
	if err != nil {
		return nil, err
	}
	naive, err := NaiveFlattenMarkers(p)
	if err != nil {
		return nil, err
	}
	naiveAt := map[string]int64{}
	for _, nm := range naive {
		if _, exists := naiveAt[nm.ID]; !exists {
			naiveAt[nm.ID] = nm.RootFrame
		}
	}
	var drifts []MarkerDrift
	for _, fm := range flat.Markers {
		nr, ok := naiveAt[fm.ID]
		if !ok {
			continue
		}
		if d := nr - fm.RootFrame; d != 0 {
			drifts = append(drifts, MarkerDrift{
				MarkerID: fm.ID, CorrectRoot: fm.RootFrame, NaiveRoot: nr, DriftFrames: d,
			})
		}
	}
	return drifts, nil
}
