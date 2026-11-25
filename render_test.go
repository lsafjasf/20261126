package smptett

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRenderFrameProducesIndependentLayers(t *testing.T) {
	doc := loadSample(t)
	frame := doc.RenderFrame(5 * time.Second)

	if frame.LayerPixelCount[KindBackgroundImage] == 0 {
		t.Fatal("background layer is empty")
	}
	if frame.LayerPixelCount[KindForegroundText] == 0 {
		t.Fatal("foreground text layer is empty")
	}
	if frame.LayerPixelCount[KindAnnotation] != 0 {
		t.Fatalf("hidden annotation leaked %d pixels", frame.LayerPixelCount[KindAnnotation])
	}
	if got := frame.Visibility["note-hidden-annotation"]; got.Visible {
		t.Fatalf("hidden annotation is visible: %+v", got)
	}
	if got := frame.Visibility["note-hidden-annotation"]; !got.Active {
		t.Fatalf("hidden annotation should still be active at 5s: %+v", got)
	}
	if got := frame.VisibleByLayer[KindForegroundText]; len(got) != 1 || got[0] != "bg-b-label-text" {
		t.Fatalf("visible foreground resources = %v", got)
	}

	bg := frame.Layers[KindBackgroundImage]
	text := frame.Layers[KindForegroundText]
	if bg.NRGBAAt(10, 10).A != 255 {
		t.Fatal("background image was not rasterized")
	}
	if text.NRGBAAt(10, 10).A != 0 {
		t.Fatal("text layer should not contain background pixels")
	}
}

func TestBackgroundAndTextChangeOverTime(t *testing.T) {
	doc := loadSample(t)
	first := doc.RenderFrame(0)
	second := doc.RenderFrame(3500 * time.Millisecond)

	if layerDigest(t, first.Layers[KindBackgroundImage]) == layerDigest(t, second.Layers[KindBackgroundImage]) {
		t.Fatal("background layer did not change when the second image became active")
	}
	if layerDigest(t, first.Layers[KindForegroundText]) == layerDigest(t, second.Layers[KindForegroundText]) {
		t.Fatal("text layer did not change between captions")
	}
	if first.VisibleByLayer[KindBackgroundImage][0] != "bg-a-background" {
		t.Fatalf("first background = %v", first.VisibleByLayer[KindBackgroundImage])
	}
	if second.VisibleByLayer[KindBackgroundImage][0] != "bg-b-background" {
		t.Fatalf("second background = %v", second.VisibleByLayer[KindBackgroundImage])
	}
}

func TestGeneratePreviewWritesFramesAndManifest(t *testing.T) {
	doc := loadSample(t)
	out := t.TempDir()
	report, err := GeneratePreview(doc, out, PreviewOptions{
		Times:  []time.Duration{0, 1 * time.Second, 3500 * time.Millisecond, 5500 * time.Millisecond},
		Strict: true,
		Input:  "testdata/sample.ttml",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Frames) != 4 {
		t.Fatalf("frames = %d, want 4", len(report.Frames))
	}
	if !report.Visibility.AllChecksPassed {
		t.Fatal("visibility checks failed")
	}
	if report.Visibility.HiddenAnnotations.Total != 2 || len(report.Visibility.HiddenAnnotations.EverVisible) != 0 {
		t.Fatalf("hidden annotation report = %+v", report.Visibility.HiddenAnnotations)
	}
	if len(report.Visibility.BackgroundImages.EverVisible) != 2 {
		t.Fatalf("background visibility = %+v", report.Visibility.BackgroundImages)
	}
	if len(report.Visibility.ForegroundText.EverVisible) != 2 {
		t.Fatalf("text visibility = %+v", report.Visibility.ForegroundText)
	}

	data, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded PreviewReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Input != "testdata/sample.ttml" || decoded.SchemaVersion != 1 {
		t.Fatalf("manifest header = %+v", decoded)
	}

	for _, frame := range report.Frames {
		for _, layer := range frame.Layers {
			img := readPNG(t, filepath.Join(out, filepath.FromSlash(layer.File)))
			if countAlpha(img) != layer.NonTransparentPixels {
				t.Fatalf("%s pixel count does not match manifest", layer.File)
			}
			if digestImage(t, img) != layer.SHA256 {
				t.Fatalf("%s digest does not match manifest", layer.File)
			}
		}
		readPNG(t, filepath.Join(out, filepath.FromSlash(frame.File)))
	}
}

func TestAnnotationLayerStaysEmptyAcrossDefaultTransitions(t *testing.T) {
	doc := loadSample(t)
	times, err := PreviewTimes(doc, PreviewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range times {
		frame := doc.RenderFrame(at)
		if frame.LayerPixelCount[KindAnnotation] != 0 {
			t.Fatalf("hidden annotation pixels at %s: %d", at, frame.LayerPixelCount[KindAnnotation])
		}
	}
}

func layerDigest(t *testing.T, img *image.NRGBA) string {
	t.Helper()
	var data []byte
	for _, value := range img.Pix {
		data = append(data, value)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readPNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func countAlpha(img image.Image) int {
	count := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0 {
				count++
			}
		}
	}
	return count
}

func digestImage(t *testing.T, img image.Image) string {
	t.Helper()
	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:])
}
