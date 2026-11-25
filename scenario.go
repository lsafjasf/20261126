package eac3

func u8(v uint8) *uint8 { return &v }

// DemoGraph 构造一个覆盖三类 E-AC-3 节目形态的内存拓扑。
// 它只准备状态图，不执行 Switch，方便测试自行选择路径。
func DemoGraph() (*Graph, []SwitchRequest, error) {
	g := NewGraph()

	mainDRC := DRCState{
		Present:       true,
		Mode:          "line",
		Profile:       "film-standard",
		DialNormDB:    -27,
		TargetLevelDB: -24,
	}
	nightDRC := DRCState{
		Present:       true,
		Mode:          "line",
		Profile:       "speech-strong",
		DialNormDB:    -27,
		TargetLevelDB: -20,
		MetadataEpoch: 2,
	}

	mainSpec := ProgramSpec{
		SampleRateHz: 48000,
		Channel: ChannelStructure{
			Acmod: 7, LFEOn: true, Channels: 6,
			Labels: []string{"L", "C", "R", "SL", "SR", "LFE"},
		},
		DRC: mainDRC,
		Boundary: BoundaryState{
			BlocksPerFrame:  6,
			SamplesPerFrame: 1536,
			FrameSizeBytes:  1200,
		},
	}
	extendedSpec := ProgramSpec{
		SampleRateHz: 48000,
		Channel: ChannelStructure{
			ChannelMapPresent: true,
			ChannelMap:        0x07f8,
			Channels:          8,
			Labels:            []string{"L", "C", "R", "Lss", "Rss", "Lrs", "Rrs", "LFE"},
		},
		DRC: mainDRC,
		Boundary: BoundaryState{
			BlocksPerFrame:  3,
			SamplesPerFrame: 768,
			FrameSizeBytes:  940,
		},
	}
	mixedSpec := extendedSpec
	mixedSpec.DRC = nightDRC

	stereoSpec := ProgramSpec{
		SampleRateHz: 44100,
		Channel: ChannelStructure{
			Acmod: 2, Channels: 2, Labels: []string{"L", "R"},
		},
		DRC: DRCState{Present: false, Mode: "none", Profile: "none"},
		Boundary: BoundaryState{
			BlocksPerFrame:  2,
			SamplesPerFrame: 512,
			FrameSizeBytes:  700,
		},
	}

	frames := []FrameInfo{
		{ProgramID: "p1-main", Type: SubstreamIndependent, SubstreamID: 0, Spec: mainSpec},
		{ProgramID: "p1-main", Type: SubstreamDependent, SubstreamID: 0, Spec: extendedSpec},
		{ProgramID: "p2-backup", Type: SubstreamIndependent, SubstreamID: 2, Spec: stereoSpec},
	}
	for _, frame := range frames {
		if err := g.Observe(frame); err != nil {
			return nil, nil, err
		}
	}

	ind0 := SubstreamRef{Type: SubstreamIndependent, ID: 0}
	dep0 := SubstreamRef{Type: SubstreamDependent, ID: 0}
	ind2 := SubstreamRef{Type: SubstreamIndependent, ID: 2}

	states := []State{
		{
			ProgramID: "p1-main", Mode: ModeIndependent, Anchor: &ind0,
			DecodeSubstreams: []SubstreamRef{ind0}, RenderSubstreams: []SubstreamRef{ind0},
			Spec: mainSpec,
		},
		{
			ProgramID: "p1-main", Mode: ModeAssociated, Anchor: &ind0,
			DecodeSubstreams: []SubstreamRef{ind0, dep0}, RenderSubstreams: []SubstreamRef{dep0},
			Spec: extendedSpec,
		},
		{
			ProgramID: "p1-main", Mode: ModeMixed, Anchor: &ind0,
			DecodeSubstreams: []SubstreamRef{ind0, dep0}, RenderSubstreams: []SubstreamRef{ind0, dep0},
			Spec: mixedSpec,
		},
		{
			ProgramID: "p2-backup", Mode: ModeIndependent, Anchor: &ind2,
			DecodeSubstreams: []SubstreamRef{ind2}, RenderSubstreams: []SubstreamRef{ind2},
			Spec: stereoSpec,
		},
	}
	for i := range states {
		if _, err := g.Declare(states[i]); err != nil {
			return nil, nil, err
		}
	}

	requests := []SwitchRequest{
		{ProgramID: "p1-main", Mode: ModeIndependent, AnchorID: u8(0), Reason: "开播：选择主独立 substream"},
		{ProgramID: "p1-main", Mode: ModeAssociated, AnchorID: u8(0), Reason: "切换到关联声道扩展节目"},
		{ProgramID: "p1-main", Mode: ModeMixed, AnchorID: u8(0), Reason: "主节目与关联 substream 混合"},
		{ProgramID: "p2-backup", Mode: ModeIndependent, AnchorID: u8(2), Reason: "跳到备用立体声节目"},
		{ProgramID: "p2-backup", Mode: ModeIndependent, AnchorID: u8(2), Reason: "重复选择当前节目"},
	}
	return g, requests, nil
}

// RunDemo 执行内置切换路径。
func RunDemo() (*Graph, []Switch, error) {
	g, requests, err := DemoGraph()
	if err != nil {
		return nil, nil, err
	}
	switches := make([]Switch, 0, len(requests))
	for _, req := range requests {
		sw, err := g.Switch(req)
		if err != nil {
			return g, switches, err
		}
		switches = append(switches, sw)
	}
	return g, switches, nil
}
