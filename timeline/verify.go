package timeline

import "fmt"

// FrameMismatch 描述同一根帧在“未展开解析”与“展开后查询”之间的差异。
type FrameMismatch struct {
	RootFrame int64  `json:"rootFrame"`
	Field     string `json:"field"`
	Before    string `json:"before"`
	After     string `json:"after"`
}

func (m FrameMismatch) String() string {
	return fmt.Sprintf("frame %d field %s: before=%s after=%s", m.RootFrame, m.Field, m.Before, m.After)
}

// VerifyFrames 逐帧对比 Resolver（展开前）与 FlatTimeline（展开后），
// 要求同一根帧的画面索引、源、内外部速率完全一致。
func VerifyFrames(r *Resolver, ft *FlatTimeline) ([]FrameMismatch, error) {
	var mismatches []FrameMismatch
	for f := int64(0); f < ft.Dur; f++ {
		before, err := r.Frame(f)
		if err != nil {
			return nil, err
		}
		after, err := ft.Frame(f)
		if err != nil {
			return nil, err
		}
		mismatches = append(mismatches, compareRecord(f, before, after)...)
	}
	return mismatches, nil
}

func compareRecord(f int64, a, b FrameRecord) []FrameMismatch {
	var out []FrameMismatch
	add := func(field, av, bv string) {
		if av != bv {
			out = append(out, FrameMismatch{RootFrame: f, Field: field, Before: av, After: bv})
		}
	}
	add("kind", string(a.Kind), string(b.Kind))
	add("sourceID", a.SourceID, b.SourceID)
	add("sourceFrame", fmt.Sprint(a.SourceFrame), fmt.Sprint(b.SourceFrame))
	add("internalSpeed", a.InternalSpeed.String(), b.InternalSpeed.String())
	add("externalSpeed", a.ExternalSpeed.String(), b.ExternalSpeed.String())
	add("combinedSpeed", a.CombinedSpeed.String(), b.CombinedSpeed.String())
	return out
}

func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += "|"
		}
		out += id
	}
	return out
}
