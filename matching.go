package channelmap

import "sort"

type matchProblem struct {
	roles    []string
	features []Features
	channels [][]float64
	costs    map[string]map[int]roleScore
}

func newMatchProblem(roles []string, set *SignatureSet, features []Features, channels [][]float64) matchProblem {
	p := matchProblem{roles: roles, features: features, channels: channels, costs: make(map[string]map[int]roleScore)}
	for _, role := range roles {
		sig, _, ok := lookupSignature(set, role)
		if !ok {
			continue
		}
		p.costs[role] = make(map[int]roleScore, len(features))
		for _, f := range features {
			var wireSamples []float64
			if f.WireIndex >= 0 && f.WireIndex < len(channels) {
				wireSamples = channels[f.WireIndex]
			}
			p.costs[role][f.WireIndex] = scoreRoleWire(role, sig, f, wireSamples)
		}
	}
	return p
}

func (p matchProblem) scoredRoles() []string {
	out := make([]string, 0, len(p.costs))
	for role := range p.costs {
		out = append(out, role)
	}
	sort.Strings(out)
	return out
}

// enumerate returns the lowest-cost injective assignments of roles to wires.
// WAV home-theater layouts are small; 8! is bounded and fully explainable.
func (p matchProblem) enumerate(limit int) []Candidate {
	roles := p.scoredRoles()
	wireCount := len(p.features)
	if len(roles) == 0 || wireCount == 0 || len(roles) > wireCount || len(roles) > 8 {
		return nil
	}

	var results []Candidate
	used := make([]bool, wireCount)
	current := make([]CandidateRole, 0, len(roles))

	var visit func(roleIndex int, total float64)
	visit = func(roleIndex int, total float64) {
		if roleIndex == len(roles) {
			rolesByWire := make([]string, wireCount)
			assignments := append([]CandidateRole(nil), current...)
			for _, a := range assignments {
				rolesByWire[a.WireIndex] = a.Role
			}
			results = append(results, Candidate{
				RolesByWire: rolesByWire,
				TotalCost:   round6(total),
				AverageCost: round6(total / float64(len(roles))),
				Assignments: assignments,
			})
			return
		}
		role := roles[roleIndex]
		for wire := 0; wire < wireCount; wire++ {
			if used[wire] {
				continue
			}
			score := p.costs[role][wire]
			used[wire] = true
			current = append(current, CandidateRole{
				WireIndex: wire, Role: role, Cost: round6(score.Cost),
				Evidence: score.Evidence, Violations: score.Violations,
			})
			visit(roleIndex+1, total+score.Cost)
			current = current[:len(current)-1]
			used[wire] = false
		}
	}
	visit(0, 0)

	sort.Slice(results, func(i, j int) bool {
		if results[i].TotalCost != results[j].TotalCost {
			return results[i].TotalCost < results[j].TotalCost
		}
		return mappingKey(results[i].RolesByWire) < mappingKey(results[j].RolesByWire)
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	annotateCandidateConfidence(results, len(roles))
	return results
}

func annotateCandidateConfidence(candidates []Candidate, roleCount int) {
	if len(candidates) == 0 {
		return
	}
	best := candidates[0].AverageCost
	gap := 100.0
	if len(candidates) > 1 {
		gap = candidates[1].AverageCost - best
	}
	conf := ConfidenceNone
	switch {
	case best <= 8 && gap >= 18:
		conf = ConfidenceHigh
	case best <= 18 && gap >= 7:
		conf = ConfidenceMedium
	case best <= 35 && gap >= 2:
		conf = ConfidenceLow
	default:
		conf = ConfidenceNone
	}
	candidates[0].Confidence = conf

	// Near-tied mappings are all ambiguous, not just the first one.
	for i := 1; i < len(candidates); i++ {
		if candidates[i].AverageCost-best <= 3 {
			candidates[i].Confidence = ConfidenceLow
		}
	}
}

func mappingKey(m []string) string {
	out := ""
	for i, role := range m {
		if i > 0 {
			out += "|"
		}
		out += role
	}
	return out
}

func perWireAlternatives(p matchProblem, wire int, limit int) []RoleCandidate {
	var all []RoleCandidate
	for role, wires := range p.costs {
		if rs, ok := wires[wire]; ok {
			all = append(all, RoleCandidate{Role: role, Cost: round6(rs.Cost), Score: round6(100 - rs.Cost)})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Cost != all[j].Cost {
			return all[i].Cost < all[j].Cost
		}
		return all[i].Role < all[j].Role
	})
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}
