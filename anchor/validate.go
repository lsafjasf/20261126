package anchor

import (
	"errors"
	"fmt"
	"strings"
)

func validID(id string) error {
	if strings.ContainsAny(id, "#:") {
		return fmt.Errorf("id %q must not contain '#' or ':'", id)
	}
	return nil
}

// Validate checks structural invariants of a project. It does not attempt
// mapping; it only guarantees that mapping can run on well-formed data.
func (p *Project) Validate() error {
	if p == nil {
		return errors.New("project is nil")
	}
	if p.Version != 1 {
		return fmt.Errorf("unsupported project version %d (want 1)", p.Version)
	}
	if p.TicksPerSecond <= 0 {
		return errors.New("ticks_per_second must be positive")
	}

	media := make(map[string]Media, len(p.Media))
	for _, m := range p.Media {
		if m.ID == "" {
			return errors.New("media with empty id")
		}
		if err := validID(m.ID); err != nil {
			return err
		}
		if _, dup := media[m.ID]; dup {
			return fmt.Errorf("duplicate media id %q", m.ID)
		}
		if m.Duration <= 0 {
			return fmt.Errorf("media %q has non-positive duration", m.ID)
		}
		media[m.ID] = m
	}

	seenEvent := make(map[string]bool, len(p.Events))
	for _, ev := range p.Events {
		if ev.ID == "" {
			return errors.New("event with empty id")
		}
		if err := validID(ev.ID); err != nil {
			return err
		}
		if seenEvent[ev.ID] {
			return fmt.Errorf("duplicate event id %q", ev.ID)
		}
		seenEvent[ev.ID] = true
		switch ev.Kind {
		case KindChapter, KindForced, KindSubtitle:
		default:
			return fmt.Errorf("event %q has unknown kind %q", ev.ID, ev.Kind)
		}
		if ev.Anchor != nil {
			if err := validateAnchor(media, *ev.Anchor); err != nil {
				return fmt.Errorf("event %q: %w", ev.ID, err)
			}
		}
	}

	seenSeg := make(map[string]bool, len(p.Edl))
	for _, seg := range p.Edl {
		if seg.ID == "" {
			return errors.New("segment with empty id")
		}
		if err := validID(seg.ID); err != nil {
			return err
		}
		if seenSeg[seg.ID] {
			return fmt.Errorf("duplicate segment id %q", seg.ID)
		}
		seenSeg[seg.ID] = true
		m, ok := media[seg.MediaID]
		if !ok {
			return fmt.Errorf("segment %q references unknown media %q", seg.ID, seg.MediaID)
		}
		if seg.SrcStart < 0 || seg.SrcEnd <= seg.SrcStart {
			return fmt.Errorf("segment %q has invalid source interval", seg.ID)
		}
		if seg.SrcEnd > m.Duration {
			return fmt.Errorf("segment %q source end %d exceeds media %q duration %d",
				seg.ID, seg.SrcEnd, m.ID, m.Duration)
		}
		if seg.DstStart < 0 {
			return fmt.Errorf("segment %q has negative destination start", seg.ID)
		}
	}

	for key, d := range p.Decisions {
		switch d.Action {
		case ActionSegment:
			if d.SegmentID == "" {
				return fmt.Errorf("decision %q: segment action requires segment_id", key)
			}
		case ActionCustom:
			if d.DstEnd <= d.DstStart {
				return fmt.Errorf("decision %q: custom interval must be non-empty", key)
			}
		case ActionDrop:
		default:
			return fmt.Errorf("decision %q has unknown action %q", key, d.Action)
		}
	}

	for eventID, a := range p.AnchorDecisions {
		m, ok := media[a.MediaID]
		if !ok {
			return fmt.Errorf("anchor decision for %q references unknown media %q", eventID, a.MediaID)
		}
		if a.Start < 0 || a.End <= a.Start || a.End > m.Duration {
			return fmt.Errorf("anchor decision for %q has invalid interval", eventID)
		}
	}
	return nil
}

func validateAnchor(media map[string]Media, a Anchor) error {
	m, ok := media[a.MediaID]
	if !ok {
		return fmt.Errorf("anchor references unknown media %q", a.MediaID)
	}
	if a.Start < 0 || a.End <= a.Start {
		return errors.New("anchor interval must be non-empty and start at zero or later")
	}
	if a.End > m.Duration {
		return fmt.Errorf("anchor end %d exceeds media %q duration %d", a.End, m.ID, m.Duration)
	}
	return nil
}
