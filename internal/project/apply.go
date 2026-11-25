package project

import (
	"fmt"

	"mediarelink/internal/match"
	"mediarelink/internal/media"
)

// RelinkError reports that one or more assets are blocked, so nothing
// was written. Blocking is the default safety behaviour.
type RelinkError struct {
	Blocked []string
}

func (e *RelinkError) Error() string {
	return fmt.Sprintf("relink blocked for %d asset(s): %v", len(e.Blocked), e.Blocked)
}

// Apply rewrites Asset.Path for every confidently relinked asset. Media
// IDs and effect references are never touched, which keeps the effect ->
// media chain traceable by construction. If any asset is blocked and
// allowPartial is false, Apply changes nothing and returns *RelinkError.
func (p *Project) Apply(plans []match.AssetPlan, allowPartial bool) (*RelinkError, error) {
	byID := map[string]match.AssetPlan{}
	var blocked []string
	for _, pl := range plans {
		byID[pl.MediaID] = pl
		if pl.Status == match.StatusBlocked {
			blocked = append(blocked, pl.MediaID)
		}
	}
	if len(blocked) > 0 && !allowPartial {
		return &RelinkError{Blocked: blocked}, nil
	}
	for i := range p.Media {
		pl, ok := byID[p.Media[i].ID]
		if ok && pl.Status == match.StatusRelinked {
			p.Media[i].Path = pl.NewPath
		}
	}
	return nil, nil
}

// Verification summarises the post-write traceability audit.
type Verification struct {
	MediaIDsIntact   bool     `json:"media_ids_intact"`
	EffectsResolve   bool     `json:"effects_resolve"`
	RelinkedHashOK   bool     `json:"relinked_hash_ok"`
	EffectMediaPairs []string `json:"effect_media_pairs"`
	Problems         []string `json:"problems,omitempty"`
}

// OK reports whether every audit check passed.
func (v Verification) OK() bool {
	return v.MediaIDsIntact && v.EffectsResolve && v.RelinkedHashOK
}

// Verify re-checks a written project against the original ID set, the
// effect references and the on-disk content of every relinked path.
func Verify(originalIDs []string, relinked *Project, plans []match.AssetPlan) Verification {
	v := Verification{MediaIDsIntact: true, EffectsResolve: true, RelinkedHashOK: true}

	now := map[string]Asset{}
	for _, a := range relinked.Media {
		now[a.ID] = a
	}
	for _, id := range originalIDs {
		if _, ok := now[id]; !ok {
			v.MediaIDsIntact = false
			v.Problems = append(v.Problems, "media id lost: "+id)
		}
	}
	for _, fx := range relinked.Effects {
		a, ok := now[fx.MediaID]
		if !ok {
			v.EffectsResolve = false
			v.Problems = append(v.Problems,
				fmt.Sprintf("effect %s references missing media %s", fx.ID, fx.MediaID))
			continue
		}
		v.EffectMediaPairs = append(v.EffectMediaPairs, fmt.Sprintf("%s -> %s (%s)", fx.ID, a.ID, a.Path))
	}

	expected := map[string]string{} // media id -> expected frame hash
	for _, a := range relinked.Media {
		expected[a.ID] = a.FrameHash
	}
	for _, pl := range plans {
		if pl.Status != match.StatusRelinked {
			continue
		}
		probe, err := media.ProbeFile(pl.NewPath)
		if err != nil {
			v.RelinkedHashOK = false
			v.Problems = append(v.Problems, fmt.Sprintf("%s: cannot probe %s: %v", pl.MediaID, pl.NewPath, err))
			continue
		}
		if probe.FrameHash != expected[pl.MediaID] {
			v.RelinkedHashOK = false
			v.Problems = append(v.Problems,
				fmt.Sprintf("%s: frame hash mismatch at %s", pl.MediaID, pl.NewPath))
		}
	}
	return v
}
