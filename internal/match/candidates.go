package match

import (
	"sort"

	"proxyrelink/internal/media"
)

// Candidate pairs one proxy with one possible original plus its score.
type Candidate struct {
	Result
	ProxyPath    string `json:"proxy_path"`
	OriginalPath string `json:"original_path"`
}

// RankedCandidates scores every proxy against every original and returns
// candidates sorted by score descending. Pairs that the user has locked
// or rejected are excluded (locked proxies/originals leave the pool).
func RankedCandidates(ix media.Index, dec *Decisions) []Candidate {
	lockedProxy := dec.LockedProxySet()
	lockedOrig := dec.LockedOriginalSet()
	var out []Candidate
	for _, p := range ix.Proxies() {
		if lockedProxy[p.ID] {
			continue
		}
		for _, o := range ix.Originals() {
			if lockedOrig[o.ID] || dec.IsRejected(p.ID, o.ID) {
				continue
			}
			r := ScorePair(p, o)
			out = append(out, Candidate{
				Result:       r,
				ProxyPath:    p.Path,
				OriginalPath: o.Path,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ProxyID < out[j].ProxyID
	})
	return out
}

// BestPerProxy returns the top candidate for each still-unmatched proxy.
func BestPerProxy(cands []Candidate) map[string]Candidate {
	best := map[string]Candidate{}
	for _, c := range cands {
		if cur, ok := best[c.ProxyID]; !ok || c.Score > cur.Score {
			best[c.ProxyID] = c
		}
	}
	return best
}
