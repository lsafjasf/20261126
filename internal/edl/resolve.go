package edl

import (
	"path/filepath"
	"sort"
)

// index is the online library projected onto the three identity dimensions.
// Every dimension is tracked separately so the resolver can show, per op,
// exactly which dimensions agreed or conflicted.
type index struct {
	byID   map[string][]Asset
	byHash map[string][]Asset
	byName map[string][]Asset
}

func buildIndex(lib Library) index {
	idx := index{
		byID:   map[string][]Asset{},
		byHash: map[string][]Asset{},
		byName: map[string][]Asset{},
	}
	for _, a := range lib.Assets {
		idx.byID[a.ClipID] = append(idx.byID[a.ClipID], a)
		idx.byHash[a.MediaHash] = append(idx.byHash[a.MediaHash], a)
		name := filepath.Base(filepath.FromSlash(a.Path))
		idx.byName[name] = append(idx.byName[name], a)
	}
	return idx
}

func assetKey(a Asset) string { return a.ClipID + "|" + a.MediaHash }

// toCandidate annotates an online asset with the dimensions it shares with
// the offline op.
func toCandidate(a Asset, op Op) Candidate {
	return Candidate{
		ClipID:    a.ClipID,
		Path:      a.Path,
		MediaHash: a.MediaHash,
		MatchID:   a.ClipID == op.OldClipID,
		MatchHash: a.MediaHash == op.MediaHash,
		MatchName: filepath.Base(filepath.FromSlash(a.Path)) ==
			filepath.Base(filepath.FromSlash(op.OldPath)),
	}
}

func uniqueCandidates(assets []Asset, op Op) []Candidate {
	seen := map[string]bool{}
	out := make([]Candidate, 0, len(assets))
	for _, a := range assets {
		if seen[assetKey(a)] {
			continue
		}
		seen[assetKey(a)] = true
		out = append(out, toCandidate(a, op))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ClipID != out[j].ClipID {
			return out[i].ClipID < out[j].ClipID
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// Resolve re-anchors every offline op against the online library and splits
// the results into applied, pending and dead resolutions.
func Resolve(offline, online Library, cl ChangeList) Report {
	idx := buildIndex(online)
	offIdx := buildIndex(offline)

	rep := Report{
		ProjectID:     cl.ProjectID,
		OnlineVersion: online.SchemaVersion,
	}

	for _, op := range cl.Ops {
		r := Resolution{Seq: op.Seq, Action: op.Action, Offline: op}

		// Sanity check the offline manifest itself: the recorded (ID, hash)
		// pair must describe the same offline asset.
		if same := offIdx.byID[op.OldClipID]; len(same) > 0 {
			consistent := false
			for _, a := range same {
				if a.MediaHash == op.MediaHash {
					consistent = true
				}
			}
			if !consistent {
				r.Status = StatusPending
				r.Reason = ReasonOfflineMismatch
				r.Warnings = append(r.Warnings,
					"offline change list pairs clip ID "+op.OldClipID+
						" with a media hash that does not match the offline library")
				collectCandidates(&r, idx, op)
				rep.Pending = append(rep.Pending, r)
				continue
			}
		}

		hashHits := idx.byHash[op.MediaHash]
		idHits := idx.byID[op.OldClipID]

		switch {
		case len(hashHits) == 0:
			// The content the offline edit pointed at is gone for good.
			r.Status = StatusDead
			r.Reason = ReasonHashMissing
			collectCandidates(&r, idx, op)
			if len(r.Candidates) > 0 {
				r.Warnings = append(r.Warnings,
					"only file-name or clip-ID lookalikes remain; the content hash is absent, so no automatic match is allowed")
			}
			rep.Dead = append(rep.Dead, r)

		case len(hashHits) > 1:
			// The same content exists in more than one online place; a human
			// must pick which instance the edit targets.
			r.Status = StatusPending
			r.Reason = ReasonHashDuplicate
			r.Candidates = uniqueCandidates(hashHits, op)
			rep.Pending = append(rep.Pending, r)

		default:
			target := hashHits[0]
			r.Candidates = uniqueCandidates(append(append([]Asset{},
				hashHits...), idHits...), op)
			conflict := false

			if target.ClipID == op.OldClipID {
				// Strongest case: both identity dimensions agree.
				r.Reason = ReasonIDHashMatch
			} else {
				// Content survived but its clip ID changed.
				r.Reason = ReasonHashRelocated
				r.Warnings = append(r.Warnings,
					"content hash matched but the clip ID changed: "+
						op.OldClipID+" -> "+target.ClipID)
				if len(idHits) > 0 {
					conflict = true
					r.Warnings = append(r.Warnings,
						"old clip ID "+op.OldClipID+
							" is now used by different content; ID reuse confirmed")
				}
			}

			// A same-name/different-hash asset is a deliberate trap. If one
			// exists, require human confirmation even though the hash is
			// unique, because a name-based tool would silently pick it.
			name := filepath.Base(filepath.FromSlash(op.OldPath))
			for _, a := range idx.byName[name] {
				if a.MediaHash != op.MediaHash {
					conflict = true
					if r.Reason != ReasonFileNameCollision {
						r.Warnings = append(r.Warnings,
							"file name "+name+" is also used by unrelated asset "+
								a.ClipID+" with hash "+shortHash(a.MediaHash)+
								"; matching by name would be wrong")
					}
				}
			}

			t := target
			r.ResolvedTo = &t
			if conflict {
				r.Status = StatusPending
				if r.Reason == ReasonIDHashMatch {
					r.Reason = ReasonFileNameCollision
				}
			} else {
				r.Status = StatusApplied
			}
			if r.Status == StatusApplied {
				rep.Applied = append(rep.Applied, r)
			} else {
				rep.Pending = append(rep.Pending, r)
			}
		}
	}

	rep.AppliedCount = len(rep.Applied)
	rep.PendingCount = len(rep.Pending)
	rep.DeadCount = len(rep.Dead)
	return rep
}

// collectCandidates gathers ID/name lookalikes for dead or inconsistent ops,
// purely as reviewer evidence. None of them match on content hash.
func collectCandidates(r *Resolution, idx index, op Op) {
	merged := append([]Asset{}, idx.byID[op.OldClipID]...)
	name := filepath.Base(filepath.FromSlash(op.OldPath))
	merged = append(merged, idx.byName[name]...)
	r.Candidates = uniqueCandidates(merged, op)
}

func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12] + "..."
}
