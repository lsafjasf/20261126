package smptett

import (
	"os"
	"strings"
	"testing"
	"time"
)

func loadSample(t *testing.T) *Document {
	t.Helper()
	doc, err := ParseFile("testdata/sample.ttml", ParseOptions{})
	if err != nil {
		t.Fatalf("ParseFile sample: %v", err)
	}
	return doc
}

func TestParseSampleResourcesAndTiming(t *testing.T) {
	doc := loadSample(t)
	if doc.Width != 640 || doc.Height != 360 {
		t.Fatalf("canvas = %dx%d, want 640x360", doc.Width, doc.Height)
	}
	if doc.Duration != 6*time.Second {
		t.Fatalf("duration = %s, want 6s", doc.Duration)
	}
	if doc.FrameRate != 25 {
		t.Fatalf("frame rate = %v, want 25", doc.FrameRate)
	}

	byID := make(map[string]*Resource)
	for _, res := range doc.Resources {
		byID[res.ID] = res
	}
	if got := byID["bg-a-background"]; got == nil {
		t.Fatal("missing first background image")
	} else if got.Kind != KindBackgroundImage || got.Image == nil {
		t.Fatalf("first background not decoded: %+v", got)
	}
	if got := byID["bg-b-label-text"]; got == nil {
		t.Fatal("missing second foreground text")
	} else if got.Begin != 3*time.Second || got.End != 6*time.Second {
		t.Fatalf("nested timing = [%s,%s], want [3s,6s]", got.Begin, got.End)
	}
	if got := byID["note-hidden-annotation"]; got == nil {
		t.Fatal("missing hidden annotation")
	} else {
		if got.Kind != KindAnnotation || !got.intrinsicallyHidden() {
			t.Fatalf("hidden annotation misclassified: %+v", got)
		}
	}

	annotations := 0
	hiddenAnnotations := 0
	texts := 0
	images := 0
	for _, res := range doc.Resources {
		switch res.Kind {
		case KindAnnotation:
			annotations++
			if res.intrinsicallyHidden() {
				hiddenAnnotations++
			}
		case KindForegroundText:
			texts++
		case KindBackgroundImage:
			images++
		}
	}
	if images != 2 || texts != 2 || annotations != 2 || hiddenAnnotations != 2 {
		t.Fatalf("layer counts images=%d texts=%d annotations=%d hidden=%d", images, texts, annotations, hiddenAnnotations)
	}
}

func TestParseExternalMediaAndStyleVisibility(t *testing.T) {
	dir := t.TempDir()
	copyFile(t, "testdata/sample.ttml", dir+"/nested.ttml")
	doc, err := ParseFile(dir+"/nested.ttml", ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Resources) == 0 {
		t.Fatal("no resources parsed")
	}

	input := `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling" tts:extent="100px 50px">
	  <body><div begin="1s" end="2s"><p begin="0s" end="1s" tts:display="none">hidden text</p></div></body>
	</tt>`
	doc, err = Parse(strings.NewReader(input), ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Resources) != 1 || !doc.Resources[0].intrinsicallyHidden() {
		t.Fatalf("display:none was not retained as a hidden foreground resource: %+v", doc.Resources)
	}
}

func TestHiddenNestedAnnotationDoesNotLeakIntoText(t *testing.T) {
	input := `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling" tts:extent="200px 100px">
	  <body><p begin="0s" end="2s">VISIBLE <span role="annotation" tts:display="none">SECRET</span></p></body>
	</tt>`
	doc, err := Parse(strings.NewReader(input), ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var text, annotation *Resource
	for _, res := range doc.Resources {
		switch res.Kind {
		case KindForegroundText:
			text = res
		case KindAnnotation:
			annotation = res
		}
	}
	if text == nil || text.Text != "VISIBLE" {
		t.Fatalf("foreground text = %+v, want only VISIBLE", text)
	}
	if annotation == nil || !annotation.intrinsicallyHidden() || annotation.Text != "SECRET" {
		t.Fatalf("nested hidden annotation = %+v", annotation)
	}
}

func TestMixedTextPreservesDocumentOrder(t *testing.T) {
	input := `<tt xmlns="http://www.w3.org/ns/ttml" tts:extent="200px 100px" xmlns:tts="http://www.w3.org/ns/ttml#styling">
	  <body><p begin="0s" end="1s">Hello <span>wide </span>world!</p></body>
	</tt>`
	doc, err := Parse(strings.NewReader(input), ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Resources) != 1 || doc.Resources[0].Text != "Hello wide world!" {
		t.Fatalf("text = %+v", doc.Resources)
	}
}

func TestRootDurationCapsUnboundedResources(t *testing.T) {
	input := `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:tts="http://www.w3.org/ns/ttml#styling" tts:extent="200px 100px" dur="4s">
	  <body><p>no explicit end</p></body>
	</tt>`
	doc, err := Parse(strings.NewReader(input), ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Duration != 4*time.Second || len(doc.Resources) != 1 {
		t.Fatalf("duration=%s resources=%d", doc.Duration, len(doc.Resources))
	}
	if doc.Resources[0].End != 4*time.Second {
		t.Fatalf("resource end = %s", doc.Resources[0].End)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
