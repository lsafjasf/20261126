package main

import (
	"fmt"
	"math"
	"strings"
)

// confidenceThreshold 是自动采用推断值的最低置信度。
// 低于该值时只报告、不修正。
const confidenceThreshold = 0.70

// group 聚合某一卷、某一媒体或整条时间线的统计证据。
type group struct {
	kind         string // 卷 / 媒体 / 时间线
	name         string
	nEvents      int
	startTC      string
	maxFrame     int
	samples      int
	dfImpossible int // 在丢帧边界观测到帧号 00/01，与 DF 矛盾
	dfPositive   int // 在丢帧边界观测到帧号 >=02，支持 DF
	durMatch     []int
	durTotal     int
	decls        []Decl
}

// GroupResult 是一个分组的推断结论与依据。
type GroupResult struct {
	Kind       string
	Name       string
	Events     int
	StartTC    string
	Decision   string
	Best       string
	Confidence float64
	Rationale  []string
}

func newGroup(kind, name string) *group {
	return &group{kind: kind, name: name, durMatch: make([]int, len(candidates))}
}

func (g *group) observe(tc Timecode) {
	g.samples++
	if tc.F > g.maxFrame {
		g.maxFrame = tc.F
	}
	if at, dropped := tc.boundaryInfo(); at {
		if dropped {
			g.dfImpossible++
		} else {
			g.dfPositive++
		}
	}
}

// Analyze 按卷、媒体和时间线三个维度分组推断帧率。
func Analyze(e *EDL) []GroupResult {
	reels := map[string]*group{}
	medias := map[string]*group{}
	var order []*group
	get := func(m map[string]*group, kind, name string) *group {
		g, ok := m[name]
		if !ok {
			g = newGroup(kind, name)
			m[name] = g
			order = append(order, g)
		}
		return g
	}
	rec := newGroup("时间线", "(记录侧)")
	rec.decls = append(rec.decls, e.RecordDecl...)
	order = append(order, rec)

	for _, ev := range e.Events {
		rg := get(reels, "卷", ev.Reel)
		if rg.nEvents == 0 {
			rg.startTC = ev.SrcIn.Raw
		}
		rg.nEvents++
		rg.observe(ev.SrcIn)
		rg.observe(ev.SrcOut)
		rg.decls = append(rg.decls, ev.Decls...)

		var mg *group
		if ev.Clip != "" {
			mg = get(medias, "媒体", ev.Clip)
			if mg.nEvents == 0 {
				mg.startTC = ev.SrcIn.Raw
			}
			mg.nEvents++
			mg.observe(ev.SrcIn)
			mg.observe(ev.SrcOut)
			mg.decls = append(mg.decls, ev.Decls...)
		}

		if rec.nEvents == 0 {
			rec.startTC = ev.RecIn.Raw
		}
		rec.nEvents++
		rec.observe(ev.RecIn)
		rec.observe(ev.RecOut)

		// 时长一致性：源出-入帧长应等于记录出-入帧长。
		// 该判定只对 DF/NDF 计数方式敏感，对标准称率不敏感。
		for i, c := range candidates {
			sd := ev.SrcOut.frames(c) - ev.SrcIn.frames(c)
			rd := ev.RecOut.frames(c) - ev.RecIn.frames(c)
			if sd < 0 || rd < 0 {
				continue
			}
			if i == 0 {
				rg.durTotal++
				rec.durTotal++
				if mg != nil {
					mg.durTotal++
				}
			}
			if sd == rd {
				rg.durMatch[i]++
				rec.durMatch[i]++
				if mg != nil {
					mg.durMatch[i]++
				}
			}
		}
	}

	var results []GroupResult
	for _, g := range order {
		if g.nEvents > 0 {
			results = append(results, g.infer())
		}
	}
	return results
}

type scored struct {
	rate     Rate
	score    float64
	excluded bool
	why      string
}

func (g *group) infer() GroupResult {
	res := GroupResult{Kind: g.kind, Name: g.name, Events: g.nEvents, StartTC: g.startTC}
	cs := make([]scored, len(candidates))
	for i, r := range candidates {
		c := &cs[i]
		c.rate = r
		c.score = 1
		switch {
		case g.samples > 0 && g.maxFrame >= r.Nominal:
			c.excluded = true
			c.why = fmt.Sprintf("观测到帧号 %d，达到或超过标称 %d", g.maxFrame, r.Nominal)
		case r.Drop && g.dfImpossible > 0:
			c.excluded = true
			c.why = "在非整十分钟边界观测到帧号 00/01，与丢帧规则矛盾"
		}
		if c.excluded {
			continue
		}
		if g.samples > 0 {
			// 帧号近似均匀分布时，观测最大值不超过 maxFrame 的概率。
			p := float64(g.maxFrame+1) / float64(r.Nominal)
			c.score *= math.Pow(p, float64(g.samples))
		}
		if g.durTotal > 0 {
			c.score *= float64(g.durMatch[i]+1) / float64(g.durTotal+2)
		}
		if g.dfPositive > 0 && r.Drop {
			c.score *= 2
		}
		matched := false
		for _, d := range g.decls {
			if d.matches(r) {
				matched = true
			}
		}
		if matched {
			c.score *= 1000
		} else if len(g.decls) > 0 {
			c.score *= 0.001
		}
	}

	var sum float64
	best := -1
	for i := range cs {
		if cs[i].excluded {
			continue
		}
		sum += cs[i].score
		if best < 0 || cs[i].score > cs[best].score {
			best = i
		}
	}

	res.Rationale = append(res.Rationale,
		fmt.Sprintf("观测 %d 个时码样本，最大帧号 %d", g.samples, g.maxFrame))
	for _, c := range cs {
		if c.excluded {
			res.Rationale = append(res.Rationale, "排除 "+c.rate.Name+"："+c.why)
		}
	}
	if g.durTotal > 0 && best >= 0 {
		res.Rationale = append(res.Rationale, fmt.Sprintf(
			"时长一致性：候选 %s 下 %d/%d 条事件源/记录帧长吻合",
			cs[best].rate.Name, g.durMatch[best], g.durTotal))
	}
	if g.dfImpossible > 0 {
		res.Rationale = append(res.Rationale,
			fmt.Sprintf("丢帧反证：%d 处非整十分钟边界出现帧号 00/01", g.dfImpossible))
	}
	if g.dfPositive > 0 {
		res.Rationale = append(res.Rationale,
			fmt.Sprintf("丢帧支持：%d 处非整十分钟边界帧号从 02 起计", g.dfPositive))
	}
	for _, d := range g.decls {
		res.Rationale = append(res.Rationale, "显式声明："+d.String())
	}

	if best < 0 || sum == 0 {
		res.Decision = "全部候选均被硬证据排除，存在冲突，不自动修正"
		return res
	}
	conf := cs[best].score / sum
	res.Confidence = conf
	res.Best = cs[best].rate.Name

	declaredOK := false
	declaredConflict := false
	for _, d := range g.decls {
		any := false
		for i := range cs {
			if d.matches(cs[i].rate) {
				any = true
				if !cs[i].excluded {
					declaredOK = true
				}
			}
		}
		if !any || !declaredOK {
			declaredConflict = true
		}
	}

	switch {
	case len(g.decls) > 0 && declaredOK:
		res.Decision = fmt.Sprintf("采用声明帧率 %s（声明与证据一致）", res.Best)
	case declaredConflict:
		res.Decision = fmt.Sprintf("声明与硬证据冲突；按证据推断为 %s（置信度 %.0f%%），请人工核对",
			res.Best, conf*100)
	case conf >= confidenceThreshold:
		res.Decision = fmt.Sprintf("采用推断帧率 %s（置信度 %.0f%%）", res.Best, conf*100)
	default:
		res.Decision = fmt.Sprintf("置信度不足（%.0f%% < %.0f%%），不自动修正，保留原值",
			conf*100, confidenceThreshold*100)
		res.Best = ""
	}
	if cs[best].rate.Nominal == 24 && !declaredOK {
		res.Rationale = append(res.Rationale,
			"注意：23.976 与 24 仅凭时码无法区分，需媒体元数据佐证")
	}
	return res
}

// Report 渲染完整推断报告。
func Report(e *EDL) string {
	var b strings.Builder
	if e.Title != "" {
		fmt.Fprintf(&b, "剪辑表: %s\n", e.Title)
	}
	fmt.Fprintf(&b, "事件总数: %d\n\n", len(e.Events))
	for _, r := range Analyze(e) {
		fmt.Fprintf(&b, "[%s] %s — %d 条事件，起始时码 %s\n", r.Kind, r.Name, r.Events, r.StartTC)
		fmt.Fprintf(&b, "  决定: %s\n", r.Decision)
		for _, line := range r.Rationale {
			fmt.Fprintf(&b, "    - %s\n", line)
		}
		b.WriteByte('\n')
	}
	for _, w := range e.Warnings {
		fmt.Fprintf(&b, "警告: %s\n", w)
	}
	return b.String()
}
