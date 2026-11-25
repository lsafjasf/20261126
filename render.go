package smptett

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"
	"strings"
	"time"
)

// RenderedFrame holds independent layers plus their back-to-front composite.
type RenderedFrame struct {
	Time            time.Duration
	Layers          map[ResourceKind]*image.NRGBA
	Composite       *image.NRGBA
	Visibility      map[string]Visibility
	VisibleByLayer  map[ResourceKind][]string
	LayerPixelCount map[ResourceKind]int
}

// RenderFrame rasterizes all currently visible resources into three layers.
func (d *Document) RenderFrame(t time.Duration) *RenderedFrame {
	frame := &RenderedFrame{
		Time:            t,
		Layers:          make(map[ResourceKind]*image.NRGBA, len(ResourceKinds)),
		Visibility:      make(map[string]Visibility, len(d.Resources)),
		VisibleByLayer:  make(map[ResourceKind][]string, len(ResourceKinds)),
		LayerPixelCount: make(map[ResourceKind]int, len(ResourceKinds)),
	}
	for _, kind := range ResourceKinds {
		frame.Layers[kind] = image.NewNRGBA(image.Rect(0, 0, d.Width, d.Height))
	}

	bg := frame.Layers[KindBackgroundImage]
	if d.RootStyle.Background.A > 0 {
		fillRect(bg, Rect{W: d.Width, H: d.Height}, d.RootStyle.Background)
	}
	d.renderRegionBackdrops(bg, t)

	for _, res := range d.Resources {
		vis := res.VisibleAt(t)
		frame.Visibility[res.ID] = vis
		if !vis.Visible {
			continue
		}
		frame.VisibleByLayer[res.Kind] = append(frame.VisibleByLayer[res.Kind], res.ID)
		layer := frame.Layers[res.Kind]
		switch {
		case res.Kind == KindBackgroundImage && res.Image != nil:
			drawScaledImage(layer, res.Image, res.Rect, res.Style.Opacity)
		case res.Kind == KindBackgroundImage:
			continue
		default:
			drawTextBlock(layer, res)
		}
	}

	for _, kind := range ResourceKinds {
		sort.Strings(frame.VisibleByLayer[kind])
		frame.LayerPixelCount[kind] = nonTransparentPixels(frame.Layers[kind])
	}

	frame.Composite = image.NewNRGBA(image.Rect(0, 0, d.Width, d.Height))
	for _, kind := range ResourceKinds {
		layer := frame.Layers[kind]
		draw.Draw(frame.Composite, frame.Composite.Bounds(), layer, image.Point{}, draw.Over)
	}
	return frame
}

func (d *Document) renderRegionBackdrops(dst *image.NRGBA, t time.Duration) {
	for _, id := range d.RegionOrder {
		region := d.Regions[id]
		if region == nil || region.Style.Background.A == 0 {
			continue
		}
		if region.Style.ShowBackground == "whenActive" && !d.regionActive(id, t) {
			continue
		}
		background := region.Style.Background
		background.A = clampByte(float64(background.A) * math.Max(0, math.Min(1, region.Style.Opacity)))
		fillRect(dst, region.Rect, background)
	}
}

func (d *Document) regionActive(id string, t time.Duration) bool {
	for _, res := range d.Resources {
		if res.RegionID == id && res.VisibleAt(t).Visible {
			return true
		}
	}
	return false
}

func drawScaledImage(dst *image.NRGBA, src image.Image, rect Rect, opacity float64) {
	rect = clipRect(rect, dst.Bounds())
	if rect.Empty() {
		return
	}
	b := src.Bounds()
	if b.Empty() {
		return
	}
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		sy := (float64(y-rect.Y)+0.5)/float64(rect.H)*float64(b.Dy()) - 0.5
		for x := rect.X; x < rect.X+rect.W; x++ {
			sx := (float64(x-rect.X)+0.5)/float64(rect.W)*float64(b.Dx()) - 0.5
			c := sampleBilinear(src, sx+float64(b.Min.X), sy+float64(b.Min.Y))
			blendPixel(dst, x, y, c, opacity)
		}
	}
}

func sampleBilinear(src image.Image, x, y float64) color.NRGBA {
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := x0+1, y0+1
	fx, fy := x-float64(x0), y-float64(y0)
	b := src.Bounds()
	clampX := func(v int) int { return max(b.Min.X, min(b.Max.X-1, v)) }
	clampY := func(v int) int { return max(b.Min.Y, min(b.Max.Y-1, v)) }
	c00 := color.NRGBAModel.Convert(src.At(clampX(x0), clampY(y0))).(color.NRGBA)
	c10 := color.NRGBAModel.Convert(src.At(clampX(x1), clampY(y0))).(color.NRGBA)
	c01 := color.NRGBAModel.Convert(src.At(clampX(x0), clampY(y1))).(color.NRGBA)
	c11 := color.NRGBAModel.Convert(src.At(clampX(x1), clampY(y1))).(color.NRGBA)
	mix := func(a, b uint8, f float64) uint8 {
		return clampByte(float64(a)*(1-f) + float64(b)*f)
	}
	return color.NRGBA{
		R: mix(mix(c00.R, c10.R, fx), mix(c01.R, c11.R, fx), fy),
		G: mix(mix(c00.G, c10.G, fx), mix(c01.G, c11.G, fx), fy),
		B: mix(mix(c00.B, c10.B, fx), mix(c01.B, c11.B, fx), fy),
		A: mix(mix(c00.A, c10.A, fx), mix(c01.A, c11.A, fx), fy),
	}
}

func drawTextBlock(dst *image.NRGBA, res *Resource) {
	rect := res.Rect
	if rect.Empty() || strings.TrimSpace(res.Text) == "" {
		return
	}
	style := res.Style
	if style.Background.A > 0 {
		background := style.Background
		background.A = clampByte(float64(background.A) * math.Max(0, math.Min(1, style.Opacity)))
		fillRect(dst, rect, background)
	}
	scale := max(1, int(math.Round(style.FontSize/9)))
	advance := 6 * scale
	lineHeight := style.LineHeight
	if lineHeight <= 0 {
		lineHeight = float64(10 * scale)
	}
	padTop := style.Padding[0]
	padRight := style.Padding[1]
	padBottom := style.Padding[2]
	padLeft := style.Padding[3]
	inner := Rect{
		X: rect.X + int(math.Round(padLeft)),
		Y: rect.Y + int(math.Round(padTop)),
		W: rect.W - int(math.Round(padLeft+padRight)),
		H: rect.H - int(math.Round(padTop+padBottom)),
	}
	inner = clipRect(inner, dst.Bounds())
	if inner.Empty() {
		return
	}
	lines := wrapText(res.Text, inner.W, advance)
	totalHeight := int(math.Round(float64(len(lines)) * lineHeight))
	startY := inner.Y
	if totalHeight < inner.H {
		switch style.DisplayAlign {
		case "center":
			startY += (inner.H - totalHeight) / 2
		case "after":
			startY += inner.H - totalHeight
		}
	}
	for i, line := range lines {
		lineWidth := len([]rune(line))*advance - scale
		if lineWidth < 0 {
			lineWidth = 0
		}
		x := inner.X
		switch style.TextAlign {
		case "center":
			x += (inner.W - lineWidth) / 2
		case "right":
			x += inner.W - lineWidth
		}
		y := startY + int(math.Round(float64(i)*lineHeight))
		drawTextLine(dst, line, x, y, scale, inner, style.Color, style.Opacity)
		if y > inner.Y+inner.H {
			break
		}
	}
}

func drawTextLine(dst *image.NRGBA, line string, x, y, scale int, clip Rect, c color.NRGBA, opacity float64) {
	cursor := x
	for _, r := range line {
		glyph := glyphFor(r)
		for gy := 0; gy < 7; gy++ {
			for gx := 0; gx < 5; gx++ {
				if !glyph[gy][gx] {
					continue
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						px := cursor + gx*scale + dx
						py := y + gy*scale + dy
						if px >= clip.X && px < clip.X+clip.W && py >= clip.Y && py < clip.Y+clip.H {
							blendPixel(dst, px, py, c, opacity)
						}
					}
				}
			}
		}
		cursor += 6 * scale
	}
}

func wrapText(text string, maxWidth, advance int) []string {
	if maxWidth <= 0 || advance <= 0 {
		return nil
	}
	var out []string
	for _, paragraph := range strings.Split(text, "\n") {
		runes := []rune(paragraph)
		if len(runes) == 0 {
			out = append(out, "")
			continue
		}
		start, lastSpace := 0, -1
		for i := 0; i < len(runes); i++ {
			if runes[i] == ' ' || runes[i] == '\t' {
				lastSpace = i
			}
			if (i-start+1)*advance <= maxWidth+advance/2 || i == start {
				continue
			}
			breakAt := i
			if lastSpace >= start {
				breakAt = lastSpace
			}
			out = append(out, strings.TrimSpace(string(runes[start:breakAt])))
			start = breakAt
			for start < len(runes) && (runes[start] == ' ' || runes[start] == '\t') {
				start++
			}
			i = start - 1
			lastSpace = -1
		}
		if start < len(runes) {
			out = append(out, strings.TrimSpace(string(runes[start:])))
		}
	}
	return out
}

func blendPixel(dst *image.NRGBA, x, y int, src color.NRGBA, opacity float64) {
	if !image.Pt(x, y).In(dst.Bounds()) {
		return
	}
	sa := float64(src.A) / 255 * math.Max(0, math.Min(1, opacity))
	if sa <= 0 {
		return
	}
	i := dst.PixOffset(x, y)
	da := float64(dst.Pix[i+3]) / 255
	outA := sa + da*(1-sa)
	if outA <= 0 {
		return
	}
	blend := func(s, d uint8) uint8 {
		value := (float64(s)/255*sa + float64(d)/255*da*(1-sa)) / outA
		return clampByte(value * 255)
	}
	dst.Pix[i] = blend(src.R, dst.Pix[i])
	dst.Pix[i+1] = blend(src.G, dst.Pix[i+1])
	dst.Pix[i+2] = blend(src.B, dst.Pix[i+2])
	dst.Pix[i+3] = clampByte(outA * 255)
}

func fillRect(dst *image.NRGBA, rect Rect, c color.NRGBA) {
	rect = clipRect(rect, dst.Bounds())
	if rect.Empty() {
		return
	}
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		for x := 0; x < rect.W; x++ {
			blendPixel(dst, rect.X+x, y, c, 1)
		}
	}
}

func clipRect(rect Rect, bounds image.Rectangle) Rect {
	x0 := max(rect.X, bounds.Min.X)
	y0 := max(rect.Y, bounds.Min.Y)
	x1 := min(rect.X+rect.W, bounds.Max.X)
	y1 := min(rect.Y+rect.H, bounds.Max.Y)
	return Rect{X: x0, Y: y0, W: max(0, x1-x0), H: max(0, y1-y0)}
}

func nonTransparentPixels(img *image.NRGBA) int {
	count := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			count++
		}
	}
	return count
}
