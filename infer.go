package channelmap

import (
	"fmt"
	"sort"
	"strings"
)

func inferMapping(r *Report, f *File, sig *SignatureSet) {
	if len(f.Samples) == 0 || len(r.Features) == 0 {
		r.Undecidable = append(r.Undecidable, UndecidableItem{
			Code: "NO_DECODED_SAMPLES", Message: "没有可用于特征比对的解码样本",
			Reason: "codec 不受内置 PCM/float 解码器支持，或 data chunk 为空",
		})
		return
	}

	if sig == nil {
		inferGeneric(r)
		return
	}

	declaredRoles := make([]string, 0, len(r.DeclaredMapping))
	for _, d := range r.DeclaredMapping {
		declaredRoles = append(declaredRoles, d.Role)
	}
	problem := newMatchProblem(declaredRoles, sig, r.Features, f.Samples)
	if len(problem.costs) == 0 {
		inferGeneric(r)
		r.Undecidable = append(r.Undecidable, UndecidableItem{
			Code: "NO_MATCHING_SIGNATURE_ROLES", Message: "特征规则没有覆盖掩码声明的任何角色",
			Reason: "signature roles must use labels such as FC, BL, BR, LFE",
		})
		return
	}
	if len(problem.scoredRoles()) > len(r.Features) || len(problem.scoredRoles()) > 8 {
		r.Undecidable = append(r.Undecidable, UndecidableItem{Code: "PERMUTATION_SEARCH_SKIPPED", Message: "声道数或待匹配角色数超过可解释穷举范围", Reason: "当前实现对至多 8 个角色做全排列"})
		inferGeneric(r)
		return
	}

	candidates := problem.enumerate(8)
	if len(candidates) == 0 {
		r.Undecidable = append(r.Undecidable, UndecidableItem{Code: "NO_MAPPING_CANDIDATE", Message: "无法构造特征映射候选", Reason: "signature/role count or channel count is invalid"})
		return
	}
	r.Candidates = candidates
	best := candidates[0]
	r.Confidence = best.Confidence

	bestByWire := make(map[int]CandidateRole)
	for _, a := range best.Assignments {
		bestByWire[a.WireIndex] = a
	}
	for i := range r.InferredMapping {
		entry := &r.InferredMapping[i]
		a, ok := bestByWire[i]
		entry.Alternatives = perWireAlternatives(problem, i, 4)
		if !ok {
			entry.Status = "undecidable"
			entry.Confidence = ConfidenceNone
			continue
		}
		entry.InferredRole = a.Role
		entry.Score = round6(100 - a.Cost)
		entry.Evidence = append(append([]string{}, a.Evidence...), a.Violations...)
		entry.Confidence = best.Confidence
		switch {
		case a.Role == entry.DeclaredRole:
			entry.Status = "match"
		case best.Confidence == ConfidenceNone:
			entry.Status = "ambiguous"
		default:
			entry.Status = "mismatch"
		}
	}

	r.Evidence = append(r.Evidence, fmt.Sprintf("best sample-to-role permutation is %s (average cost %.2f, confidence %s)", strings.Join(best.RolesByWire, ","), best.AverageCost, best.Confidence))
	detectMappingConflicts(r, best)
	detectFamilyConsensus(r, candidates)
	detectAmbiguity(r, candidates)
	buildRepair(r, best)
}

func detectFamilyConsensus(r *Report, candidates []Candidate) {
	if hasConflictCode(r, "CENTER_SURROUND_SWAP") || len(candidates) < 2 {
		return
	}
	bestCost := candidates[0].AverageCost
	var viable []Candidate
	for _, c := range candidates {
		if c.AverageCost <= bestCost+3 && c.AverageCost <= 15 {
			viable = append(viable, c)
		}
	}
	if len(viable) < 2 {
		return
	}

	groupsByWire := map[int]map[string]bool{}
	for _, c := range viable {
		for wire, role := range c.RolesByWire {
			if role == "" {
				continue
			}
			if groupsByWire[wire] == nil {
				groupsByWire[wire] = map[string]bool{}
			}
			groupsByWire[wire][RoleClass(role)] = true
		}
	}
	consensus := map[int]string{}
	for wire, groups := range groupsByWire {
		if len(groups) == 1 {
			for group := range groups {
				consensus[wire] = group
			}
		}
	}

	for i := range r.DeclaredMapping {
		declared := r.DeclaredMapping[i].Role
		observedGroup, ok := consensus[i]
		if !ok || observedGroup == RoleClass(declared) {
			continue
		}
		if !(IsCenterRole(declared) && strings.HasPrefix(observedGroup, "SURROUND")) &&
			!(IsSurroundRole(declared) && observedGroup == "CENTER") {
			continue
		}
		for j := range r.DeclaredMapping {
			if i == j {
				continue
			}
			otherDeclared := r.DeclaredMapping[j].Role
			otherGroup := consensus[j]
			if IsCenterRole(declared) && IsSurroundRole(otherDeclared) && otherGroup == "CENTER" {
				// i=declared center but observed surround; j=declared surround but observed center.
			} else if IsSurroundRole(declared) && IsCenterRole(otherDeclared) && observedGroup == "CENTER" && otherGroup == "SURROUND" {
				// The loop can encounter the same pair from the opposite direction.
			} else {
				continue
			}
			r.Conflicts = append(r.Conflicts, Conflict{
				Code: "CENTER_SURROUND_SWAP", Severity: "error", Confidence: ConfidenceMedium,
				Message: fmt.Sprintf("中置/环绕声道族接反：线序 %d 声明 %s 但样本一致属于 %s；线序 %d 声明 %s 但样本一致属于 CENTER。BL/SL 与 BR/SR 的具体左右身份不可判定。",
					i, declared, observedGroup, j, otherDeclared),
				WireIndices: []int{i, j}, DeclaredRoles: []string{declared, otherDeclared},
				ObservedRoles: observedRolesForPair(declared, observedGroup, otherDeclared, otherGroup),
				Evidence:      []string{fmt.Sprintf("%d 个近似最优排列对这两个声道族给出一致结论", len(viable))},
			})
			r.Confidence = maxConfidence(r.Confidence, ConfidenceMedium)
			for wire, group := range map[int]string{i: observedGroup, j: otherGroup} {
				entry := &r.InferredMapping[wire]
				if group == "CENTER" {
					entry.InferredRole = "FC"
				} else {
					entry.InferredRole = group + "?"
				}
				entry.Status = "family_mismatch"
				entry.Confidence = ConfidenceMedium
			}
			return
		}
	}
}

func observedRolesForPair(declaredA, observedA, declaredB, observedB string) []string {
	if IsCenterRole(declaredA) {
		return []string{"SURROUND", "FC"}
	}
	return []string{"FC", "SURROUND"}
}

func hasConflictCode(r *Report, code string) bool {
	for _, c := range r.Conflicts {
		if c.Code == code {
			return true
		}
	}
	return false
}

func inferGeneric(r *Report) {
	var lfeWires []int
	for _, f := range r.Features {
		if f.Active && f.LowBandEnergyRatio >= 0.70 && f.DominantFrequencyHz > 0 && f.DominantFrequencyHz <= 120 {
			lfeWires = append(lfeWires, f.WireIndex)
		}
		if !f.Active {
			r.Undecidable = append(r.Undecidable, UndecidableItem{
				Code: "SILENT_CHANNEL", WireIndices: []int{f.WireIndex},
				Message: fmt.Sprintf("线序 %d 为静音/近静音，无法凭样本确定角色", f.WireIndex),
				Reason:  "silence is compatible with an unused channel, missing feed, or gated material",
			})
		}
	}
	if len(lfeWires) == 1 {
		wire := lfeWires[0]
		for i := range r.InferredMapping {
			if r.InferredMapping[i].WireIndex != wire {
				continue
			}
			r.InferredMapping[i].InferredRole = "LFE?"
			r.InferredMapping[i].Status = "heuristic"
			r.InferredMapping[i].Confidence = ConfidenceMedium
			r.InferredMapping[i].Evidence = []string{fmt.Sprintf("dominant=%.2f Hz low-band ratio=%.2f", r.Features[wire].DominantFrequencyHz, r.Features[wire].LowBandEnergyRatio)}
			if r.InferredMapping[i].DeclaredRole != "LFE" {
				r.Conflicts = append(r.Conflicts, Conflict{
					Code: "POSSIBLE_LFE_MISMATCH", Severity: "warning", Confidence: ConfidenceLow,
					Message:     fmt.Sprintf("低频特征位于线序 %d，但掩码在该位置声明 %s", wire, r.InferredMapping[i].DeclaredRole),
					WireIndices: []int{wire}, ObservedRoles: []string{"LFE?"},
					Evidence: r.InferredMapping[i].Evidence,
				})
			}
		}
	}
	r.Undecidable = append(r.Undecidable, UndecidableItem{
		Code:       "SPEAKER_IDENTITY_NOT_ENCODED",
		Message:    "普通音频样本不携带通用的“中置/环绕”身份标签",
		Reason:     "频率、相关性和声像特征只有在已知测试信号、参考文件或角色特征规则时才能唯一映射到扬声器名称",
		Candidates: []string{"provide Options.Signatures", "use -signatures roles.json", "use -calibration for bundled test tones"},
	})
}

func detectMappingConflicts(r *Report, best Candidate) {
	if best.Confidence != ConfidenceHigh && best.Confidence != ConfidenceMedium {
		return
	}
	var mismatches []InferredEntry
	for _, e := range r.InferredMapping {
		if e.Status == "mismatch" {
			mismatches = append(mismatches, e)
		}
	}
	used := map[int]bool{}
	for _, e := range mismatches {
		if used[e.WireIndex] {
			continue
		}
		partner := -1
		for _, other := range mismatches {
			if other.WireIndex == e.WireIndex || used[other.WireIndex] {
				continue
			}
			if other.InferredRole == e.DeclaredRole && e.InferredRole == other.DeclaredRole {
				partner = other.WireIndex
				break
			}
		}
		if partner >= 0 {
			used[e.WireIndex] = true
			used[partner] = true
			other := r.InferredMapping[partner]
			code := "CHANNEL_SWAP"
			message := fmt.Sprintf("线序 %d 与 %d 发生交换：掩码顺序要求 %s/%s，样本实际显示 %s/%s",
				e.WireIndex, partner, e.DeclaredRole, other.DeclaredRole, e.InferredRole, other.InferredRole)
			if centerSurroundPair(e.DeclaredRole, other.DeclaredRole) || centerSurroundPair(e.InferredRole, other.InferredRole) {
				code = "CENTER_SURROUND_SWAP"
				message = fmt.Sprintf("中置与环绕声道接反：线序 %d（掩码声明 %s，样本为 %s）和线序 %d（掩码声明 %s，样本为 %s）",
					e.WireIndex, e.DeclaredRole, e.InferredRole, partner, other.DeclaredRole, other.InferredRole)
			}
			r.Conflicts = append(r.Conflicts, Conflict{
				Code: code, Severity: "error", Confidence: best.Confidence, Message: message,
				WireIndices:   []int{e.WireIndex, partner},
				DeclaredRoles: []string{e.DeclaredRole, other.DeclaredRole},
				ObservedRoles: []string{e.InferredRole, other.InferredRole},
				Evidence:      append(append([]string{}, e.Evidence...), other.Evidence...),
			})
			continue
		}
		r.Conflicts = append(r.Conflicts, Conflict{
			Code: "CHANNEL_MAPPING_CONFLICT", Severity: "error", Confidence: best.Confidence,
			Message:     fmt.Sprintf("线序 %d 掩码声明 %s，但特征指向 %s", e.WireIndex, e.DeclaredRole, e.InferredRole),
			WireIndices: []int{e.WireIndex}, DeclaredRoles: []string{e.DeclaredRole}, ObservedRoles: []string{e.InferredRole},
			Evidence: e.Evidence,
		})
	}
}

func centerSurroundPair(a, b string) bool {
	return (IsCenterRole(a) && IsSurroundRole(b)) || (IsSurroundRole(a) && IsCenterRole(b))
}

func detectAmbiguity(r *Report, candidates []Candidate) {
	if len(candidates) < 2 {
		return
	}
	best := candidates[0]
	var tied []Candidate
	for _, c := range candidates {
		if c.AverageCost-best.AverageCost <= 3 {
			tied = append(tied, c)
		}
	}
	if len(tied) <= 1 || best.Confidence == ConfidenceHigh {
		return
	}
	roles := map[string]bool{}
	wires := map[int]bool{}
	for _, c := range tied {
		for wire, role := range c.RolesByWire {
			if role != "" {
				roles[role] = true
				wires[wire] = true
			}
		}
	}
	roleList := sortedKeys(roles)
	wireList := sortedIntKeys(wires)
	code := "MAPPING_AMBIGUOUS"
	if containsAny(roleList, "BL", "BR", "SL", "SR") && containsAny(roleList, "FC") {
		code = "CENTER_OR_SURROUND_SIDE_AMBIGUOUS"
	} else if containsAny(roleList, "BL", "BR", "SL", "SR") {
		code = "SURROUND_PAIR_AMBIGUOUS"
	}
	r.Undecidable = append(r.Undecidable, UndecidableItem{
		Code: code, Message: "存在多个特征成本近似相同的映射，不能唯一指定扬声器角色",
		WireIndices: wireList, Roles: roleList,
		Reason:     "左右对称角色或相同特征规则会产生不可区分的最优排列；需要更独特的样本/参考文件",
		Candidates: []string{mappingKey(best.RolesByWire), mappingKey(tied[1].RolesByWire)},
	})
}

func buildRepair(r *Report, best Candidate) {
	if best.Confidence != ConfidenceHigh && best.Confidence != ConfidenceMedium {
		return
	}
	wireForRole := map[string]int{}
	for wire, role := range best.RolesByWire {
		if role != "" {
			if _, exists := wireForRole[role]; exists {
				return
			}
			wireForRole[role] = wire
		}
	}
	order := make([]int, 0, len(r.DeclaredMapping))
	routes := make([]string, 0, len(r.DeclaredMapping))
	for _, d := range r.DeclaredMapping {
		wire, ok := wireForRole[d.Role]
		if !ok {
			return
		}
		order = append(order, wire)
		routes = append(routes, fmt.Sprintf("wire %d -> %s", wire, d.Role))
	}
	r.Repair = &Repair{
		SourceIndicesForMaskOrder: order,
		WireToSpeaker:             routes,
		Description:               "按 source_indices_for_mask_order 从当前交错数据取声道，可重排为掩码声明顺序；该建议只在特征映射置信度足够时生成。",
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedIntKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func containsAny(items []string, wanted ...string) bool {
	for _, item := range items {
		for _, w := range wanted {
			if item == w {
				return true
			}
		}
	}
	return false
}
