package imsc

import (
	"io"
	"strings"
	"time"
)

// Config controls Load behaviour.
type Config struct {
	// FrameRate is used for four-component HH:MM:SS:FF clock expressions.
	// Zero selects the package default (30).
	FrameRate int
}

// Option customises Load.
type Option func(*Config)

// WithFrameRate overrides the frame rate used for frame-based clocks.
func WithFrameRate(fps int) Option {
	return func(c *Config) {
		if fps > 0 {
			c.FrameRate = fps
		}
	}
}

// Load parses and validates an IMSC image subtitle document. The returned
// Document is always populated when the XML itself is well formed; element
// level problems are collected in Document.ElementErrors rather than dropped.
//
// When the document contains no decodable image, Load returns the document
// together with ErrNoImages wrapped in the error chain (use errors.Is). It
// never returns a nil document with a nil error for such input.
func Load(r io.Reader, opts ...Option) (*Document, error) {
	cfg := Config{FrameRate: defaultFrameRate}
	for _, opt := range opts {
		opt(&cfg)
	}

	root, parseErr := parseXML(r)
	if parseErr != nil {
		return nil, parseErr
	}

	doc := &Document{imageIndex: map[string]*Image{}}

	images, imageErrs := processImages(root)
	doc.Images = images
	for _, img := range images {
		doc.imageIndex[img.ID] = img
	}

	regions, regionErrs := processRegions(root)
	doc.Regions = regions

	cues, cueErrs := processCues(root, doc, cfg.FrameRate)
	doc.Cues = cues

	for _, sub := range []ErrorList{imageErrs, regionErrs, cueErrs} {
		for _, elemErr := range sub.All() {
			doc.ElementErrors.append(elemErr)
		}
	}

	var err error
	if doc.ElementErrors.Len() > 0 {
		err = doc.ElementErrors
	}
	if !doc.HasImages() {
		err = joinErrors(err, ErrNoImages)
	}
	return doc, err
}

func joinErrors(existing error, extra error) error {
	if existing == nil {
		return extra
	}
	if list, ok := existing.(ErrorList); ok {
		list.append(extra)
		return list
	}
	var list ErrorList
	list.append(existing)
	list.append(extra)
	return list
}

func processCues(root *node, doc *Document, frameRate int) ([]*Cue, ErrorList) {
	var cues []*Cue
	var errs ErrorList

	for _, p := range root.collect("p") {
		cue := &Cue{
			Line: p.Line,
			Path: p.path,
		}

		beginRaw, hasBegin := p.attr("begin")
		endRaw, hasEnd := p.attr("end")
		cue.Begin = parseTiming(p, "begin", beginRaw, hasBegin, frameRate, &errs, &cue.TimingValid)
		if hasEnd {
			end, ok := parseOneClock(p, "end", endRaw, frameRate, &errs)
			cue.TimingValid = cue.TimingValid && ok
			if ok {
				cue.End = end
				cue.HasEnd = true
				if end < cue.Begin {
					cue.TimingValid = false
					errs.append(NewElementError(p.path, p.Line, ReasonBadTiming,
						"end "+endRaw+" is earlier than begin "+beginRaw))
				}
			}
		}

		if regionRef, ok := p.attr("region"); ok {
			cue.RegionID = strings.TrimSpace(regionRef)
		}

		if text := strings.TrimSpace(p.fullText()); text != "" {
			cue.FallbackText = text
			cue.HasText = true
		}

		resolveCueImages(p, cue, doc, &errs)
		cues = append(cues, cue)
	}

	return cues, errs
}

func parseTiming(n *node, attrName, raw string, present bool, frameRate int, errs *ErrorList, valid *bool) time.Duration {
	if !present || strings.TrimSpace(raw) == "" {
		return 0
	}
	d, ok := parseOneClock(n, attrName, raw, frameRate, errs)
	*valid = ok
	return d
}

func parseOneClock(n *node, attrName, raw string, frameRate int, errs *ErrorList) (time.Duration, bool) {
	d, err := parseClockWithRate(raw, frameRate)
	if err != nil {
		errs.append(NewElementError(n.path, n.Line, ReasonBadClock,
			attrName+" "+raw+" is invalid: "+err.Error()))
		return 0, false
	}
	return d, true
}

// resolveCueImages applies the IMSC priority: inline tt:p background, then
// span background, then the referenced region's background. Each candidate
// reference list is tried in order until a decodable image is found.
func resolveCueImages(p *node, cue *Cue, doc *Document, errs *ErrorList) {
	var failed []string
	// softErrors hold per-reference failures that become element errors only
	// when the whole cue fails to resolve any image. A successful later
	// fallback reference means the earlier miss is expected, not an error.
	var softErrors ErrorList

	tryRefs := func(raw string, holder *node) bool {
		for _, ref := range parseBackgroundRefs(raw) {
			id, valid := fragmentID(ref)
			if !valid {
				failed = append(failed, ref)
				softErrors.append(NewElementError(holder.path, holder.Line, ReasonExternalImage,
					"external or malformed image reference "+ref+" cannot be loaded"))
				continue
			}
			img, ok := doc.ImageByID(id)
			if !ok {
				failed = append(failed, ref)
				softErrors.append(NewElementError(holder.path, holder.Line, ReasonUnresolvedRef,
					"image reference #"+id+" does not match any decodable smpte:image"))
				continue
			}
			cue.Images = append(cue.Images, img)
			return true
		}
		return false
	}

	resolved := false
	if raw, ok := p.attr("backgroundImage"); ok && strings.TrimSpace(raw) != "" {
		resolved = tryRefs(raw, p)
	}
	if !resolved {
		for _, span := range p.collect("span") {
			if raw, ok := span.attr("backgroundImage"); ok && strings.TrimSpace(raw) != "" {
				if tryRefs(raw, span) {
					resolved = true
					break
				}
			}
		}
	}
	if !resolved && cue.RegionID != "" {
		region := findRegion(doc, cue.RegionID)
		switch {
		case region == nil:
			errs.append(NewElementError(p.path, p.Line, ReasonUnknownRegion,
				"paragraph references unknown region "+cue.RegionID))
		case len(region.ImageRefs) == 0:
			errs.append(NewElementError(p.path, p.Line, ReasonRegionNoImage,
				"region "+cue.RegionID+" declares no smpte:backgroundImage"))
		default:
			for _, ref := range region.ImageRefs {
				id, valid := fragmentID(ref)
				if !valid {
					failed = append(failed, ref)
					softErrors.append(NewElementError(p.path, p.Line, ReasonExternalImage,
						"region "+cue.RegionID+" has external reference "+ref))
					continue
				}
				img, ok := doc.ImageByID(id)
				if !ok {
					failed = append(failed, ref)
					softErrors.append(NewElementError(p.path, p.Line, ReasonUnresolvedRef,
						"region "+cue.RegionID+" image #"+id+" cannot be resolved"))
					continue
				}
				cue.Images = append(cue.Images, img)
				resolved = true
				break
			}
		}
	}

	if !resolved {
		for _, softErr := range softErrors.All() {
			errs.append(softErr)
		}
		cue.Placeholder = buildPlaceholder(cue, failed)
	}
}

func findRegion(doc *Document, id string) *Region {
	for _, region := range doc.Regions {
		if region.ID == id {
			return region
		}
	}
	return nil
}

func buildPlaceholder(cue *Cue, failed []string) *Placeholder {
	detail := "no background image could be loaded"
	if cue.HasText {
		detail = "image unavailable; showing text fallback: " + cue.FallbackText
	} else {
		detail = "image unavailable and the element has no text fallback"
	}
	return &Placeholder{
		Reason:     ReasonUnresolvedRef,
		Detail:     detail,
		FailedRefs: failed,
	}
}
