package smptett

import (
	"image"
	"image/color"
	"time"
)

// ResourceKind is the presentation layer a timed resource belongs to.
type ResourceKind string

const (
	KindBackgroundImage ResourceKind = "background_image"
	KindForegroundText  ResourceKind = "foreground_text"
	KindAnnotation      ResourceKind = "annotation"
)

// ResourceKinds is in back-to-front compositing order.
var ResourceKinds = []ResourceKind{
	KindBackgroundImage,
	KindForegroundText,
	KindAnnotation,
}

// Rect is a pixel rectangle in the root canvas coordinate system.
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

func (r Rect) Empty() bool {
	return r.W <= 0 || r.H <= 0
}

// Region is a TTML layout region, resolved to pixels.
type Region struct {
	ID    string
	Rect  Rect
	Style Style
}

// Style contains the presentation properties implemented by this renderer.
type Style struct {
	Color          color.NRGBA
	Background     color.NRGBA
	FontSize       float64
	LineHeight     float64
	TextAlign      string
	DisplayAlign   string
	WritingMode    string
	Padding        [4]float64 // top, right, bottom, left
	Display        string
	Visibility     string
	ShowBackground string
	Opacity        float64
	OriginX        float64
	OriginY        float64
	ExtentW        float64
	ExtentH        float64
	HasOrigin      bool
	HasExtent      bool
	OriginXUnit    string
	OriginYUnit    string
	ExtentXUnit    string
	ExtentYUnit    string
}

func defaultStyle() Style {
	return Style{
		Color:          color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		Background:     color.NRGBA{},
		FontSize:       32,
		TextAlign:      "center",
		DisplayAlign:   "after",
		WritingMode:    "lrtb",
		Display:        "auto",
		Visibility:     "visible",
		ShowBackground: "always",
		Opacity:        1,
	}
}

// Resource is one timed background image, text block, or annotation.
type Resource struct {
	ID            string
	Kind          ResourceKind
	Begin         time.Duration
	End           time.Duration
	RegionID      string
	Rect          Rect
	Text          string
	MediaType     string
	Source        string
	SourceBytes   int
	Image         image.Image
	Style         Style
	HiddenReasons []string
	ParseWarning  string
}

// Document is a parsed, resolved SMPTE-TT presentation.
type Document struct {
	Title       string
	Language    string
	Width       int
	Height      int
	FrameRate   float64
	TickRate    float64
	RootStyle   Style
	Regions     map[string]*Region
	RegionOrder []string
	Resources   []*Resource
	Duration    time.Duration
	Warnings    []string
}

// Visibility describes whether a resource contributes pixels at a time.
type Visibility struct {
	Active  bool
	Visible bool
	Reasons []string
}

// VisibleAt applies both the document timeline and intrinsic hiding rules.
func (r *Resource) VisibleAt(t time.Duration) Visibility {
	active := t >= r.Begin && t < r.End
	reasons := append([]string(nil), r.HiddenReasons...)
	if !active {
		reasons = append(reasons, "inactive")
	}
	if r.Rect.Empty() {
		reasons = append(reasons, "empty-region")
	}
	if r.Style.Opacity <= 0 {
		reasons = append(reasons, "zero-opacity")
	}
	return Visibility{
		Active:  active,
		Visible: active && len(reasons) == 0,
		Reasons: uniqueStrings(reasons),
	}
}

func (r *Resource) intrinsicallyHidden() bool {
	return len(r.HiddenReasons) > 0
}

func uniqueStrings(in []string) []string {
	if len(in) < 2 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
