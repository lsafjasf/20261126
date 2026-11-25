package vttlayout

import "sort"

// Stage is a maximal time interval during which the active cue set is
// constant. Layout conflicts are evaluated per stage.
type Stage struct {
	StartMS Milliseconds
	EndMS   Milliseconds
	Cues    []*Cue
}

// Key returns a stable identity for an active set.
func (s Stage) Key() string {
	orders := make([]int, len(s.Cues))
	for i, c := range s.Cues {
		orders[i] = c.Order
	}
	sort.Ints(orders)
	out := ""
	for i, o := range orders {
		if i > 0 {
			out += ","
		}
		out += itoa(o)
	}
	return out
}

// BuildStages sweeps all cue endpoints and reports non empty active sets.
func BuildStages(cues []*Cue) []Stage {
	points := map[Milliseconds]struct{}{}
	for _, c := range cues {
		if c.EndMS <= c.StartMS {
			continue
		}
		points[c.StartMS] = struct{}{}
		points[c.EndMS] = struct{}{}
	}
	times := make([]Milliseconds, 0, len(points))
	for t := range points {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

	var stages []Stage
	for i := 0; i+1 < len(times); i++ {
		a, b := times[i], times[i+1]
		if b <= a {
			continue
		}
		var active []*Cue
		for _, c := range cues {
			if c.StartMS <= a && c.EndMS > a {
				active = append(active, c)
			}
		}
		if len(active) == 0 {
			continue
		}
		sort.Slice(active, func(i, j int) bool { return active[i].Order < active[j].Order })
		stages = append(stages, Stage{StartMS: a, EndMS: b, Cues: active})
	}
	return stages
}

// Cluster is a connected component of the interval overlap graph.
type Cluster struct {
	Cues    []*Cue
	StartMS Milliseconds
	EndMS   Milliseconds
}

// BuildClusters groups cues that transitively overlap in time.
func BuildClusters(cues []*Cue) []Cluster {
	sorted := make([]*Cue, len(cues))
	copy(sorted, cues)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].StartMS != sorted[j].StartMS {
			return sorted[i].StartMS < sorted[j].StartMS
		}
		return sorted[i].Order < sorted[j].Order
	})

	var clusters []Cluster
	for _, c := range sorted {
		if c.EndMS <= c.StartMS {
			continue
		}
		placed := false
		for i := range clusters {
			if c.StartMS < clusters[i].EndMS {
				clusters[i].Cues = append(clusters[i].Cues, c)
				if c.EndMS > clusters[i].EndMS {
					clusters[i].EndMS = c.EndMS
				}
				placed = true
				break
			}
		}
		if !placed {
			clusters = append(clusters, Cluster{
				Cues: []*Cue{c}, StartMS: c.StartMS, EndMS: c.EndMS,
			})
		}
	}
	return clusters
}

// OverlapTime reports whether two cue intervals share positive time.
func OverlapTime(a, b *Cue) bool {
	return a.StartMS < b.EndMS && b.StartMS < a.EndMS
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
