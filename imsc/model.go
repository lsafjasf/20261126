package imsc

import "time"

// Image is one decoded smpte:image resource from the document metadata.
type Image struct {
	// ID is the xml:id referenced by smpte:backgroundImage fragments.
	ID string
	// Type is the smpte:imageType value, for example "png" or "jpeg".
	Type string
	// Data is the decoded raw image payload. It is never nil for a successful
	// Image; failed resources stay in Document.ElementErrors instead.
	Data []byte
	// Line is the 1-based source line of the smpte:image element.
	Line int
}

// Region mirrors a ttm:region-style style carrying a background image.
type Region struct {
	// ID is the xml:id of the tt:region element.
	ID string
	// ImageRefs are the ordered smpte:backgroundImage fragment references
	// (xml:id values, without the leading '#').
	ImageRefs []string
	// Line is the 1-based source line of the tt:region element.
	Line int
}

// Placeholder records why an image cue could not be rendered and what the
// text track should show instead.
type Placeholder struct {
	// Reason identifies the failure class. It is never empty.
	Reason Reason
	// Detail is a human-readable, non-empty explanation.
	Detail string
	// FailedRefs lists image references that were tried and could not be
	// resolved or decoded, in resolution order.
	FailedRefs []string
}

// Cue is one timed text-image entry derived from a tt:p element.
type Cue struct {
	// Begin is the resolved start time relative to the document timeline.
	Begin time.Duration
	// End is the resolved end time. Zero means an open-ended cue.
	End time.Duration
	// HasEnd reports whether an explicit end time was provided.
	HasEnd bool
	// RegionID is the region referenced by the paragraph, if any.
	RegionID string
	// Images are the resolved, decodable background images in priority order:
	// inline tt:p references first, then span, then region references.
	Images []*Image
	// FallbackText is the concatenated textual content of the paragraph and
	// its spans, used when no image can be displayed. It is always a
	// non-empty, trimmed string when HasText is true.
	FallbackText string
	// HasText reports whether FallbackText carries content.
	HasText bool
	// Placeholder is set when not every requested reference resolved; the
	// text track must then show FallbackText instead of the missing image.
	Placeholder *Placeholder
	// TimingValid is false when begin/end could not be parsed; such cues are
	// still emitted with a Placeholder so the failure is never silent.
	TimingValid bool
	// Line is the 1-based source line of the tt:p element.
	Line int
	// Path identifies the element within the document, for example
	// tt[1]/body[1]/div[1]/p[2].
	Path string
}

// Document is the parsed and validated IMSC image subtitle document.
type Document struct {
	Images  []*Image
	Regions []*Region
	Cues    []*Cue

	// ElementErrors holds every failure attributed to a concrete element. It
	// is never populated with blank messages.
	ElementErrors ErrorList

	// imageIndex maps xml:id to a decoded image.
	imageIndex map[string]*Image
}

// HasImages reports whether the document contains at least one decodable
// image resource.
func (d *Document) HasImages() bool {
	return len(d.Images) > 0
}

// ImageByID resolves a background-image fragment to its decoded image.
func (d *Document) ImageByID(id string) (*Image, bool) {
	img, ok := d.imageIndex[id]
	return img, ok
}

// PlaceholderCues returns only cues that currently require text-track
// placeholder behaviour.
func (d *Document) PlaceholderCues() []*Cue {
	var out []*Cue
	for _, cue := range d.Cues {
		if cue.Placeholder != nil {
			out = append(out, cue)
		}
	}
	return out
}
