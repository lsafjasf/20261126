package timeline

import "fmt"

// MarkerMismatch 描述标记在展开前后的根帧覆盖不一致。
type MarkerMismatch struct {
	MarkerID    string  `json:"markerID"`
	FlatRoot    int64   `json:"flatRoot"`
	ResolverSet []int64 `json:"resolverFrames"`
}

func (m MarkerMismatch) String() string {
	return fmt.Sprintf("marker %s flatRoot=%d not in resolver frames %v",
		m.MarkerID, m.FlatRoot, m.ResolverSet)
}

// VerifyMarkers 校验展开后的每个标记根帧都被 Resolver 在同一根帧命中。
// 变速会让一个局部标记覆盖多个根帧，因此这里做集合包含校验。
func VerifyMarkers(r *Resolver, ft *FlatTimeline) ([]MarkerMismatch, error) {
	hit := map[string][]int64{}
	for f := int64(0); f < ft.Dur; f++ {
		rec, err := r.Frame(f)
		if err != nil {
			return nil, err
		}
		for _, id := range rec.Markers {
			hit[id] = append(hit[id], f)
		}
	}
	var out []MarkerMismatch
	for _, m := range ft.Markers {
		frames := hit[m.ID]
		if !containsFrame(frames, m.RootFrame) {
			out = append(out, MarkerMismatch{
				MarkerID: m.ID, FlatRoot: m.RootFrame, ResolverSet: frames,
			})
		}
	}
	return out, nil
}

func containsFrame(frames []int64, want int64) bool {
	for _, f := range frames {
		if f == want {
			return true
		}
	}
	return false
}
