package timeline

// DemoProject 构造一个覆盖典型问题的工程：
//   - 内层 3/2 变速与外层 2/3 变速叠加（朴素连乘会与逐级 floor 漂移）；
//   - 冻结段独立存在，不参与速率乘积；
//   - 根时间轴尾部留空（gap）；
//   - 标记分布在复合层与叶子层。
func DemoProject() *Project {
	src := &Source{ID: "srcA", Name: "camera-a", Frames: 1000}

	inner := &Compound{
		ID:  "inner",
		Dur: 8,
		Items: []*Item{
			{
				ID: "in-leaf", Kind: KindLeaf, SourceID: "srcA",
				Start: 0, Dur: 6, SrcIn: 10, SrcOut: 30,
				Speed: Speed{Num: 3, Den: 2}, // 内部变速
				Markers: []Marker{
					{ID: "mk-inner-leaf", Offset: 4, Note: "leaf marker at local 4"},
				},
			},
			{
				ID: "in-freeze", Kind: KindLeaf, SourceID: "srcA",
				Start: 6, Dur: 2, SrcIn: 42, SrcOut: 43, Freeze: true,
				Markers: []Marker{
					{ID: "mk-freeze", Offset: 1, Note: "freeze marker"},
				},
			},
		},
		Markers: []Marker{
			{ID: "mk-inner-tl", Offset: 2, Note: "inner timeline marker"},
		},
	}

	outer := &Compound{
		ID:  "outer",
		Dur: 8,
		Items: []*Item{
			{
				ID: "outer-ref", Kind: KindRef, CompoundID: "inner",
				Start: 0, Dur: 8,
				Speed: Speed{Num: 2, Den: 3}, // 外部变速
				Markers: []Marker{
					{ID: "mk-outer-ref", Offset: 5, Note: "outer ref marker"},
				},
			},
		},
	}

	root := &Compound{
		ID:  "root",
		Dur: 10,
		Items: []*Item{
			{
				ID: "root-ref", Kind: KindRef, CompoundID: "outer",
				Start: 0, Dur: 8, Speed: SpeedOne,
			},
			// 帧 8、9 为根级冻结段：独立于所有变速，恒定显示源帧 77。
			{
				ID: "root-freeze", Kind: KindLeaf, SourceID: "srcA",
				Start: 8, Dur: 2, SrcIn: 77, SrcOut: 78, Freeze: true,
			},
		},
		Markers: []Marker{
			{ID: "mk-root", Offset: 3, Note: "root marker"},
		},
	}

	return &Project{
		Name:      "compound-speed-demo",
		Sources:   map[string]*Source{"srcA": src},
		Compounds: map[string]*Compound{"inner": inner, "outer": outer},
		Root:      root,
	}
}

// CyclicProject 构造一个 A -> B -> A 的循环引用工程，用于演示
// CheckCycles 的独立检测能力。
func CyclicProject() *Project {
	a := &Compound{
		ID:  "A",
		Dur: 4,
		Items: []*Item{
			{ID: "a-ref", Kind: KindRef, CompoundID: "B", Start: 0, Dur: 4, Speed: SpeedOne},
		},
	}
	b := &Compound{
		ID:  "B",
		Dur: 4,
		Items: []*Item{
			{ID: "b-ref", Kind: KindRef, CompoundID: "A", Start: 0, Dur: 4, Speed: SpeedOne},
		},
	}
	root := &Compound{
		ID:    "root",
		Dur:   4,
		Items: []*Item{{ID: "root-a", Kind: KindRef, CompoundID: "A", Start: 0, Dur: 4, Speed: SpeedOne}},
	}
	return &Project{
		Name:      "cyclic-demo",
		Sources:   map[string]*Source{},
		Compounds: map[string]*Compound{"A": a, "B": b},
		Root:      root,
	}
}
