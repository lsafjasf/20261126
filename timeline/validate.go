package timeline

import (
	"errors"
	"fmt"
)

// maxResolveDepth 限制复合片段嵌套深度，循环引用由 CheckCycles 单独报告，
// 这里仅作为防御性兜底。
const maxResolveDepth = 128

// Validate 做结构性校验。循环引用不在此处处理，见 CheckCycles。
func (p *Project) Validate() error {
	var errs []error
	if p == nil {
		return errors.New("timeline: nil project")
	}
	if p.Root == nil {
		return errors.New("timeline: project has no root timeline")
	}
	for id, src := range p.Sources {
		if id == "" {
			errs = append(errs, errors.New("timeline: source with empty id"))
			continue
		}
		if src == nil || src.Frames <= 0 {
			errs = append(errs, fmt.Errorf("timeline: source %q has invalid frame count", id))
		}
	}
	seenCompound := map[string]bool{}
	for id, c := range p.Compounds {
		if id == "" {
			errs = append(errs, errors.New("timeline: compound with empty id"))
			continue
		}
		seenCompound[id] = true
		if c == nil {
			errs = append(errs, fmt.Errorf("timeline: compound %q is nil", id))
			continue
		}
		if c.ID != id {
			errs = append(errs, fmt.Errorf("timeline: compound map key %q != id %q", id, c.ID))
		}
		errs = append(errs, p.validateCompound(id, c)...)
	}
	errs = append(errs, p.validateCompound("root", p.Root)...)
	return errors.Join(errs...)
}

func (p *Project) validateCompound(name string, c *Compound) []error {
	var errs []error
	if c.Dur <= 0 {
		errs = append(errs, fmt.Errorf("timeline: compound %q has non-positive duration", name))
		return errs
	}
	itemIDs := map[string]bool{}
	var lastEnd int64
	for idx, it := range c.Items {
		if it == nil {
			errs = append(errs, fmt.Errorf("timeline: compound %q item %d is nil", name, idx))
			continue
		}
		if it.ID == "" {
			errs = append(errs, fmt.Errorf("timeline: compound %q item %d has empty id", name, idx))
		} else if itemIDs[it.ID] {
			errs = append(errs, fmt.Errorf("timeline: compound %q duplicate item id %q", name, it.ID))
		} else {
			itemIDs[it.ID] = true
		}
		if it.Start < 0 || it.Dur <= 0 || it.Start+it.Dur > c.Dur {
			errs = append(errs, fmt.Errorf("timeline: item %q/%q exceeds timeline bounds", name, it.ID))
		}
		if idx > 0 && it.Start < lastEnd {
			errs = append(errs, fmt.Errorf("timeline: compound %q items overlap at item %q", name, it.ID))
		}
		if it.Start+it.Dur > lastEnd {
			lastEnd = it.Start + it.Dur
		}
		errs = append(errs, p.validateItem(name, it)...)
		for _, m := range it.Markers {
			if m.Offset < 0 || m.Offset >= it.Dur {
				errs = append(errs, fmt.Errorf("timeline: marker %q out of item %q bounds", m.ID, it.ID))
			}
		}
	}
	for _, m := range c.Markers {
		if m.Offset < 0 || m.Offset >= c.Dur {
			errs = append(errs, fmt.Errorf("timeline: compound %q marker %q out of bounds", name, m.ID))
		}
	}
	return errs
}

func (p *Project) validateItem(compoundName string, it *Item) []error {
	var errs []error
	speed := it.Speed.Normalize()
	if !speed.Valid() {
		errs = append(errs, fmt.Errorf("timeline: item %q/%q has invalid speed %v", compoundName, it.ID, it.Speed))
	}
	switch it.Kind {
	case KindLeaf:
		src := p.Sources[it.SourceID]
		if src == nil {
			errs = append(errs, fmt.Errorf("timeline: leaf %q references missing source %q", it.ID, it.SourceID))
			return errs
		}
		if it.SrcIn < 0 || it.SrcOut <= it.SrcIn || it.SrcOut > src.Frames {
			errs = append(errs, fmt.Errorf("timeline: leaf %q source range [%d,%d) invalid for source %q (%d frames)",
				it.ID, it.SrcIn, it.SrcOut, it.SourceID, src.Frames))
			return errs
		}
		if it.Freeze {
			if it.Speed.Normalize() != SpeedOne {
				errs = append(errs, fmt.Errorf("timeline: freeze leaf %q must not carry speed", it.ID))
			}
			return errs
		}
		lastMapped, err := speed.Map(it.Dur - 1)
		if err != nil {
			errs = append(errs, fmt.Errorf("timeline: leaf %q: %w", it.ID, err))
			return errs
		}
		if it.SrcIn+lastMapped >= it.SrcOut {
			errs = append(errs, fmt.Errorf("timeline: leaf %q speed %v overflows source range", it.ID, speed))
		}
	case KindRef:
		if it.Freeze {
			errs = append(errs, fmt.Errorf("timeline: ref %q cannot be a freeze segment", it.ID))
		}
		if it.SourceID != "" || it.SrcIn != 0 || it.SrcOut != 0 {
			errs = append(errs, fmt.Errorf("timeline: ref %q must not carry leaf source fields", it.ID))
		}
		child := p.Compounds[it.CompoundID]
		if child == nil {
			errs = append(errs, fmt.Errorf("timeline: ref %q references missing compound %q", it.ID, it.CompoundID))
			return errs
		}
		if it.Dur != child.Dur {
			errs = append(errs, fmt.Errorf("timeline: ref %q dur %d != compound %q dur %d (1:1 nesting required)",
				it.ID, it.Dur, it.CompoundID, child.Dur))
		}
	default:
		errs = append(errs, fmt.Errorf("timeline: item %q has unknown kind %q", it.ID, it.Kind))
	}
	return errs
}
