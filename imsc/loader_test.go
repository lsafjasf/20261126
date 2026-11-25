package imsc

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"
)

func pngBase64(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

const docShell = `<tt xml:lang="en"
	xmlns="http://www.w3.org/ns/ttml"
	xmlns:tt="http://www.w3.org/ns/ttml"
	xmlns:smpte="http://www.smpte-ra.org/schemas/2052-1/2010/smpte-tt"
	xmlns:ttp="http://www.w3.org/ns/ttml#parameter">
	<head>
		<layout>
			<region xml:id="rBottom" tts:extent="100% 20%"
				smpte:backgroundImage="%s"
				xmlns:tts="http://www.w3.org/ns/ttml#styling"/>
			<region xml:id="rNoImage"/>
			<region xml:id="rExternal" smpte:backgroundImage="https://example.test/x.png"/>
		</layout>
		<smpte:image xml:id="imgGood" smpte:type="png" smpte:encoding="Base64">%s</smpte:image>
		<smpte:image xml:id="imgBroken" smpte:type="png" smpte:encoding="Base64">%%%not-base64</smpte:image>
	</head>
	<body>
		<div>
			<p region="rBottom" begin="00:00:01" end="00:00:03">Hello</p>
			<p region="rBottom" begin="1s" end="2s">
				<span smpte:backgroundImage="#imgGood">Inline span</span>
			</p>
			<p region="rNoImage" begin="0s" end="1s">Text only</p>
			<p region="rMissing" begin="0s" end="1s">Orphan region</p>
			<p region="rBottom" begin="nonsense" end="1s">Bad timing</p>
			<p smpte:backgroundImage="#missing #imgGood" begin="0s" end="1s">Fallback chain</p>
			<p region="rExternal" begin="0s" end="1s">External ref</p>
			<p region="rBottom" begin="2s">Open ended</p>
		</div>
	</body>
</tt>`

func loadTestDoc(t *testing.T) (*Document, error) {
	t.Helper()
	doc := bytes.NewBufferString(
		"<?xml version=\"1.0\"?>\n" +
			sprintfShell(docShell, "#imgGood", pngBase64(t)))
	return Load(doc)
}

func sprintfShell(format, ref, payload string) string {
	out := format
	out = strings.Replace(out, "%s", ref, 1)
	out = strings.Replace(out, "%s", payload, 1)
	out = strings.Replace(out, "%%%not-base64", "%not-base64", 1)
	return out
}

func TestLoadHappyPathAndCues(t *testing.T) {
	doc, err := loadTestDoc(t)
	if err == nil {
		t.Fatalf("expected element errors from the fixture, got nil")
	}
	if doc == nil {
		t.Fatalf("document should still be returned with element errors")
	}
	if !doc.HasImages() {
		t.Fatalf("expected one decodable image")
	}
	if len(doc.Images) != 1 || doc.Images[0].ID != "imgGood" {
		t.Fatalf("unexpected images: %+v", doc.Images)
	}

	first := doc.Cues[0]
	if first.Begin != time.Second || !first.HasEnd || first.End != 3*time.Second {
		t.Fatalf("unexpected timing on first cue: %+v", first)
	}
	if len(first.Images) != 1 || first.Images[0].ID != "imgGood" {
		t.Fatalf("region image not resolved: %+v", first.Images)
	}
	if first.Placeholder != nil {
		t.Fatalf("first cue should render an image, got placeholder")
	}

	open := doc.Cues[len(doc.Cues)-1]
	if open.HasEnd {
		t.Fatalf("last cue should be open-ended, got end %v", open.End)
	}
}

func TestFallbackChainUsesSecondReference(t *testing.T) {
	doc, _ := loadTestDoc(t)
	var chain *Cue
	for _, cue := range doc.Cues {
		if cue.FallbackText == "Fallback chain" {
			chain = cue
		}
	}
	if chain == nil {
		t.Fatalf("fallback chain cue not found")
	}
	if len(chain.Images) != 1 || chain.Images[0].ID != "imgGood" {
		t.Fatalf("expected fallback to imgGood, got %+v", chain.Images)
	}
	if chain.Placeholder != nil {
		t.Fatalf("a cue whose later fallback resolves must not be a placeholder")
	}
}

func TestPlaceholderOnMissingImageWithText(t *testing.T) {
	doc, _ := loadTestDoc(t)
	var textOnly *Cue
	for _, cue := range doc.Cues {
		if cue.FallbackText == "Text only" {
			textOnly = cue
		}
	}
	if textOnly == nil || textOnly.Placeholder == nil {
		t.Fatalf("region without image must produce a placeholder: %+v", textOnly)
	}
	if textOnly.Placeholder.Reason == "" || textOnly.Placeholder.Detail == "" {
		t.Fatalf("placeholder must carry non-empty reason and detail")
	}
	if !textOnly.HasText || textOnly.FallbackText != "Text only" {
		t.Fatalf("fallback text must be preserved")
	}
}

func TestBadTimingStillEmitsCue(t *testing.T) {
	doc, _ := loadTestDoc(t)
	var bad *Cue
	for _, cue := range doc.Cues {
		if cue.FallbackText == "Bad timing" {
			bad = cue
		}
	}
	if bad == nil {
		t.Fatalf("bad-timing cue must still be emitted")
	}
	if bad.TimingValid {
		t.Fatalf("TimingValid must be false")
	}
	if !hasReason(doc.ElementErrors, ReasonBadClock) {
		t.Fatalf("bad clock must be an element error")
	}
}

func TestUnknownRegionAndExternalReference(t *testing.T) {
	doc, _ := loadTestDoc(t)
	if !hasReason(doc.ElementErrors, ReasonUnknownRegion) {
		t.Fatalf("expected unknown_region error")
	}
	if !hasReason(doc.ElementErrors, ReasonExternalImage) {
		t.Fatalf("expected external_image error")
	}
	for _, cue := range doc.Cues {
		if cue.FallbackText == "External ref" && cue.Placeholder == nil {
			t.Fatalf("external reference cue must be a placeholder")
		}
	}
}

func TestBrokenImageIsElementErrorNotSuccess(t *testing.T) {
	doc, _ := loadTestDoc(t)
	for _, img := range doc.Images {
		if img.ID == "imgBroken" {
			t.Fatalf("broken image must not appear as a usable image")
		}
	}
	if !hasReason(doc.ElementErrors, ReasonBadBase64) {
		t.Fatalf("broken Base64 must produce an element error")
	}
	for _, elemErr := range doc.ElementErrors.ElementErrors() {
		if strings.TrimSpace(elemErr.Message) == "" || strings.TrimSpace(elemErr.Element) == "" {
			t.Fatalf("element error must never have blank fields: %+v", elemErr)
		}
	}
}

func TestNoImagesReturnsSentinelAndPlaceholder(t *testing.T) {
	xml := `<?xml version="1.0"?><tt xmlns="http://www.w3.org/ns/ttml"
		xmlns:smpte="http://www.smpte-ra.org/schemas/2052-1/2010/smpte-tt">
		<body><div><p begin="0s" end="1s">Only text, no images</p></div></body></tt>`
	doc, err := Load(strings.NewReader(xml))
	if doc == nil {
		t.Fatalf("document must be returned even without images")
	}
	if !errors.Is(err, ErrNoImages) {
		t.Fatalf("expected ErrNoImages, got %v", err)
	}
	if doc.HasImages() {
		t.Fatalf("HasImages must be false")
	}
	cue := doc.Cues[0]
	if cue.Placeholder == nil {
		t.Fatalf("text-only document cue must carry a placeholder")
	}
	if cue.HasText && cue.FallbackText == "" {
		t.Fatalf("fallback text must not be an empty string")
	}
}

func TestMalformedXMLIsElementError(t *testing.T) {
	_, err := Load(strings.NewReader(`<tt><body><p></div>`))
	var elemErr *ElementError
	if !errors.As(err, &elemErr) {
		t.Fatalf("expected ElementError for malformed XML, got %T %v", err, err)
	}
	if elemErr.Reason != ReasonMalformedXML || elemErr.Line <= 0 {
		t.Fatalf("malformed XML error missing reason/line: %+v", elemErr)
	}
}

func TestNewElementErrorRejectsBlankFields(t *testing.T) {
	for _, tc := range []struct {
		element string
		reason  Reason
		message string
	}{
		{"", ReasonBadClock, "x"},
		{"p[1]", "", "x"},
		{"p[1]", ReasonBadClock, "  "},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected panic for blank error fields: %+v", tc)
				}
			}()
			NewElementError(tc.element, 1, tc.reason, tc.message)
		}()
	}
}

func TestElementErrorReportsLineAndPath(t *testing.T) {
	doc, _ := loadTestDoc(t)
	for _, elemErr := range doc.ElementErrors.ElementErrors() {
		if elemErr.Line <= 0 {
			t.Fatalf("element error should carry a source line: %+v", elemErr)
		}
		if !strings.Contains(elemErr.Element, "tt[1]") {
			t.Fatalf("element path should start at the root: %q", elemErr.Element)
		}
	}
}

func hasReason(list ErrorList, reason Reason) bool {
	for _, err := range list.ElementErrors() {
		if err.Reason == reason {
			return true
		}
	}
	return false
}

func TestLoadWithOptionsAndMultiRefFallback(t *testing.T) {
	payload := pngBase64(t)
	xml := `<?xml version="1.0"?>
<tt xmlns="http://www.w3.org/ns/ttml"
    xmlns:smpte="http://www.smpte-ra.org/schemas/2052-1/2010/smpte-tt">
  <head>
    <smpte:image xml:id="a" smpte:imageType="png" smpte:encoding="Base64">` + payload + `</smpte:image>
  </head>
  <body><div>
    <p smpte:backgroundImage="#missing #a" begin="00:00:00:12" end="1s">Cue</p>
  </div></body>
</tt>`
	doc, err := Load(strings.NewReader(xml), WithFrameRate(24))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cue := doc.Cues[0]
	if len(cue.Images) != 1 || cue.Images[0].ID != "a" {
		t.Fatalf("multi-reference fallback did not resolve image a: %+v", cue.Images)
	}
	if cue.Placeholder != nil {
		t.Fatalf("resolved cue must not be a placeholder")
	}
	if cue.Begin != 500*time.Millisecond {
		t.Fatalf("12/24 frames = 500ms, got %v", cue.Begin)
	}
}
