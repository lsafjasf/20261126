package sdh

// LayerReport summarizes one semantic layer for JSON/editor tooling.
type LayerReport struct {
	Role   string `json:"role"`
	Label  string `json:"label"`
	Count  int    `json:"count"`
	Anchor string `json:"anchor"`
	Vocal  bool   `json:"vocal"`
}

// Report is the deterministic analysis result emitted by the CLI and usable
// directly by an editor backend.
type Report struct {
	CueCount      int                 `json:"cue_count"`
	EntryCount    int                 `json:"entry_count"`
	Layers        []LayerReport       `json:"layers"`
	OverlapGroups int                 `json:"overlap_groups"`
	MaxLanes      map[string]int      `json:"max_lanes"`
	Overflow      []string            `json:"overflow,omitempty"`
	PlainCueCount int                 `json:"plain_cue_count"`
	Completeness  *CompletenessReport `json:"completeness"`
}

// BuildReport summarizes a split project and its SDH-off projection.
func BuildReport(p *Project, plain []Cue, verification *CompletenessReport) *Report {
	report := &Report{
		CueCount:      len(p.Cues),
		EntryCount:    len(p.Entries),
		PlainCueCount: len(plain),
		MaxLanes:      map[string]int{},
		Completeness:  verification,
	}
	for _, role := range p.LayerOrder {
		layer := p.Layers[role]
		anchor := "bottom"
		if role.Anchor() == AnchorTop {
			anchor = "top"
		}
		report.Layers = append(report.Layers, LayerReport{
			Role:   string(role),
			Label:  role.Label(),
			Count:  len(layer.Entries),
			Anchor: anchor,
			Vocal:  role.Vocal(),
		})
	}
	if p.Layout != nil {
		report.OverlapGroups = len(p.Layout.Groups)
		for anchor, lanes := range p.Layout.MaxLanes {
			name := "bottom"
			if anchor == AnchorTop {
				name = "top"
			}
			report.MaxLanes[name] = lanes
		}
		report.Overflow = p.Layout.Overflow
	}
	return report
}
