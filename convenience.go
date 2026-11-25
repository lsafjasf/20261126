package subselect

import "context"

// SelectLanguage is shorthand for Select(Criteria{Language: lang}).
func (s *Selector) SelectLanguage(lang string) Decision {
	return s.Select(Criteria{Language: lang})
}

// SwitchLanguage switches this session to lang using the default version
// ranking. On success it returns the committed view. A superseded concurrent
// switch returns the current view together with ErrStaleSwitch.
func (p *Player) SwitchLanguage(ctx context.Context, lang string) (*View, error) {
	result, err := p.Switch(ctx, Criteria{Language: lang})
	return result.View, err
}

// SwitchTrack switches this session to one exact track/version id.
func (p *Player) SwitchTrack(ctx context.Context, trackID string) (*View, error) {
	result, err := p.Switch(ctx, Criteria{TrackID: trackID})
	return result.View, err
}
