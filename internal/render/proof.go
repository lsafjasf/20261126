// Package render draws subtitle lines with the exact primary font so the
// report contains visual proof of tofu. Missing runes are drawn as an empty
// .notdef box; this mirrors what ffmpeg/libass produce when the configured
// font lacks the glyph and no fallback chain is available.
package render

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"unicode"

	"fontaudit/internal/fontscan"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	dpi      = 96
	fontSize = 40
	padX     = 24
	padY     = 20
)

// LineProof is one rendered subtitle line plus the runes that became boxes.
type LineProof struct {
	CueIndex  int
	StartText string
	Line      string
	Boxes     []rune
	PNGPath   string
}

// Renderer draws with one fixed face. It never consults a fallback list:
// silent substitution is exactly what the audit is meant to detect.
type Renderer struct {
	face font.Face
	src  *fontscan.Face
}

// NewRenderer opens the audit face at the proof size.
func NewRenderer(f *fontscan.Face) (*Renderer, error) {
	face, err := opentype.NewFace(f.Font(), &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}
	return &Renderer{face: face, src: f}, nil
}

// DrawLine renders one line and writes a PNG under outDir.
func (r *Renderer) DrawLine(cueIndex int, startText, line string, outDir string) (LineProof, error) {
	p := LineProof{CueIndex: cueIndex, StartText: startText, Line: line}

	width := r.measure(line)
	imgW := width + padX*2
	imgH := r.face.Metrics().Height.Ceil() + padY*2
	if imgH < fontSize+padY*2 {
		imgH = fontSize + padY*2
	}
	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{A: 255}}, image.Point{}, draw.Src)

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{R: 30, G: 30, B: 30, A: 255}),
		Face: r.face,
	}
	baseline := padY + r.face.Metrics().Ascent.Ceil()
	d.Dot = fixed.Point26_6{X: fixed.I(padX), Y: fixed.I(baseline)}

	boxColor := color.RGBA{R: 200, G: 40, B: 40, A: 255}
	for _, c := range line {
		if c == '\n' || c == '\r' || unicode.IsControl(c) {
			continue
		}
		adv, ok := r.face.GlyphAdvance(c)
		if !ok {
			p.Boxes = append(p.Boxes, c)
			r.drawBox(d, adv, baseline, boxColor)
			if adv == 0 {
				adv = r.face.Metrics().Height / 2
				d.Dot.X += adv
			}
			continue
		}
		d.DrawString(string(c))
	}

	name := fmt.Sprintf("cue%03d.png", cueIndex)
	p.PNGPath = filepath.Join(outDir, name)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return p, err
	}
	f, err := os.Create(p.PNGPath)
	if err != nil {
		return p, err
	}
	defer f.Close()
	return p, png.Encode(f, img)
}

func (r *Renderer) measure(line string) int {
	var w fixed.Int26_6
	for _, c := range line {
		if c == '\n' || unicode.IsControl(c) {
			continue
		}
		adv, ok := r.face.GlyphAdvance(c)
		if !ok {
			adv = r.face.Metrics().Height / 2
		}
		w += adv
	}
	return w.Ceil()
}

// drawBox paints the .notdef rectangle: outline only, matching the hollow
// tofu square shown by typical renderers.
func (r *Renderer) drawBox(d *font.Drawer, adv fixed.Int26_6, baseline int, col color.Color) {
	if adv == 0 {
		adv = r.face.Metrics().Height / 2
	}
	x0 := d.Dot.X
	x1 := x0 + adv
	top := fixed.I(baseline - r.face.Metrics().Ascent.Ceil())
	bottom := fixed.I(baseline + r.face.Metrics().Descent.Ceil())
	rect := image.Rect(x0.Ceil()+1, top.Ceil()+1, x1.Ceil()-1, bottom.Ceil()-1)

	for x := rect.Min.X; x <= rect.Max.X; x++ {
		d.Dst.Set(x, rect.Min.Y, col)
		d.Dst.Set(x, rect.Max.Y, col)
	}
	for y := rect.Min.Y; y <= rect.Max.Y; y++ {
		d.Dst.Set(rect.Min.X, y, col)
		d.Dst.Set(rect.Max.X, y, col)
	}
	d.Dot.X = x1
}
