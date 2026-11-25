package smptett

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ParseOptions controls document parsing and media resolution.
type ParseOptions struct {
	BaseDir     string
	Width       int
	Height      int
	CellColumns int
	CellRows    int
}

type parser struct {
	doc         *Document
	sheet       *styleSheet
	baseDir     string
	canvasW     int
	canvasH     int
	comments    []commentAnnotation
	resourceIDs map[string]bool
	warnings    []string
}

type commentAnnotation struct {
	Text  string
	Begin time.Duration
	End   time.Duration
}

type walkState struct {
	Begin       time.Duration
	End         time.Duration
	Style       Style
	RegionID    string
	InTextBlock bool
}

// ParseFile parses a local SMPTE-TT file.
func ParseFile(path string, opts ParseOptions) (*Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open input: %w", err)
	}
	defer f.Close()
	if opts.BaseDir == "" {
		opts.BaseDir = filepath.Dir(path)
	}
	doc, err := Parse(f, opts)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc, nil
}

// Parse parses and resolves a SMPTE-TT document. It decodes referenced raster
// media during parsing so the preview stage cannot silently ignore bad images.
func Parse(r io.Reader, opts ParseOptions) (*Document, error) {
	root, err := parseXMLTree(r)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "tt" {
		return nil, fmt.Errorf("root element is %q, want tt", root.Name.Local)
	}
	if opts.BaseDir == "" {
		opts.BaseDir = "."
	}
	p := &parser{
		doc:         &Document{Regions: make(map[string]*Region)},
		sheet:       newStyleSheet(),
		baseDir:     opts.BaseDir,
		resourceIDs: make(map[string]bool),
	}
	if err := p.parseHeader(root, opts); err != nil {
		return nil, err
	}

	body := firstChild(root, "body")
	if body == nil {
		return nil, fmt.Errorf("document has no body")
	}
	initial := walkState{
		Begin: 0,
		End:   p.rootEnd(root),
		Style: p.doc.RootStyle,
	}
	if err := p.walkBody(body, initial); err != nil {
		return nil, err
	}
	p.finishTimeline(root)
	p.doc.Warnings = uniqueStrings(append(p.doc.Warnings, p.warnings...))
	return p.doc, nil
}

func (p *parser) rootEnd(root *xmlNode) time.Duration {
	for _, name := range []string{"end", "dur"} {
		if value, ok := attrValue(root, name); ok && value != "" {
			if d, err := parseTTMLTime(value, p.doc.FrameRate, p.doc.TickRate); err == nil {
				return d
			}
		}
	}
	return -1
}

func (p *parser) parseHeader(root *xmlNode, opts ParseOptions) error {
	rootAttrs := attrLocalMap(root)
	p.doc.Title = firstNonEmpty(rootAttrs["title"], rootAttrs["id"])
	p.doc.Language = rootAttrs["lang"]

	frameRate := parseFrameRate(rootAttrs["frameRate"], rootAttrs["frameRateMultiplier"])
	tickRate := frameRate
	if n, err := strconv.ParseFloat(rootAttrs["tickRate"], 64); err == nil && n > 0 {
		tickRate = n
	}
	p.doc.FrameRate = frameRate
	p.doc.TickRate = tickRate

	p.doc.RootStyle = defaultStyle()
	applyRawStyle(&p.doc.RootStyle, rootAttrs, 640, 360)

	head := firstChild(root, "head")
	if head != nil {
		if styling := firstChild(head, "styling"); styling != nil {
			for _, style := range childElements(styling, "style") {
				p.sheet.addStyle(style)
			}
		}
		for _, style := range childElements(head, "style") {
			p.sheet.addStyle(style)
		}
	}

	p.canvasW, p.canvasH = opts.Width, opts.Height
	if p.canvasW <= 0 || p.canvasH <= 0 {
		w, h := p.parseRootExtent(rootAttrs["extent"])
		if p.canvasW <= 0 {
			p.canvasW = w
		}
		if p.canvasH <= 0 {
			p.canvasH = h
		}
	}
	if head != nil {
		p.inferCanvasFromRegions(head)
	}
	if p.canvasW <= 0 {
		p.canvasW = 640
	}
	if p.canvasH <= 0 {
		p.canvasH = int(math.Round(float64(p.canvasW) * 9 / 16))
	}
	if p.canvasW > 8192 || p.canvasH > 8192 {
		return fmt.Errorf("canvas %dx%d exceeds 8192x8192 safety limit", p.canvasW, p.canvasH)
	}
	p.doc.Width = p.canvasW
	p.doc.Height = p.canvasH

	p.doc.RootStyle = defaultStyle()
	applyRawStyle(&p.doc.RootStyle, rootAttrs, p.canvasW, p.canvasH)
	p.doc.RootStyle.HasOrigin = false
	p.doc.RootStyle.HasExtent = false

	if head != nil {
		layout := firstChild(head, "layout")
		if layout != nil {
			for _, regionNode := range childElements(layout, "region") {
				p.addRegion(regionNode)
			}
		}
		for _, regionNode := range childElements(head, "region") {
			p.addRegion(regionNode)
		}
	}
	return nil
}

func (p *parser) parseRootExtent(value string) (int, int) {
	a, b, unit, ok := parsePair(value)
	if !ok {
		return 0, 0
	}
	switch unit {
	case "", "px":
		return int(math.Round(a)), int(math.Round(b))
	default:
		p.warnings = append(p.warnings, fmt.Sprintf("root extent unit %q is not absolute; using 640x360", unit))
		return 0, 0
	}
}

func (p *parser) inferCanvasFromRegions(head *xmlNode) {
	if p.canvasW > 0 && p.canvasH > 0 {
		return
	}
	maxW, maxH := 0, 0
	walkNodes(head, func(n *xmlNode) {
		if n.Name.Local != "region" {
			return
		}
		raw := p.sheet.rawForNode(n)
		ox, _, oy, _, okO := parsePairMixed(raw["origin"])
		ew, _, eh, _, okE := parsePairMixed(raw["extent"])
		if okO && okE {
			maxW = max(maxW, int(math.Round(ox+ew)))
			maxH = max(maxH, int(math.Round(oy+eh)))
		}
	})
	if p.canvasW <= 0 {
		p.canvasW = maxW
	}
	if p.canvasH <= 0 {
		p.canvasH = maxH
	}
}

func (p *parser) addRegion(n *xmlNode) {
	raw := attrLocalMap(n)
	id := raw["id"]
	if id == "" {
		p.warnings = append(p.warnings, "ignoring region without xml:id")
		return
	}
	merged := p.sheet.rawForNode(n)
	style := defaultStyle()
	style.TextAlign = "center"
	applyRawStyle(&style, merged, p.canvasW, p.canvasH)

	rect := Rect{X: 0, Y: 0, W: p.canvasW, H: p.canvasH}
	if style.HasOrigin {
		rect.X = int(math.Round(resolveLength(style.OriginX, style.OriginXUnit, p.canvasW, p.canvasW)))
		rect.Y = int(math.Round(resolveLength(style.OriginY, style.OriginYUnit, p.canvasH, p.canvasH)))
	}
	if style.HasExtent {
		rect.W = int(math.Round(resolveLength(style.ExtentW, style.ExtentXUnit, p.canvasW, p.canvasW)))
		rect.H = int(math.Round(resolveLength(style.ExtentH, style.ExtentYUnit, p.canvasH, p.canvasH)))
	}
	rect = clampRect(rect, p.canvasW, p.canvasH)
	style.HasOrigin = false
	style.HasExtent = false
	p.doc.Regions[id] = &Region{ID: id, Rect: rect, Style: style}
	p.doc.RegionOrder = append(p.doc.RegionOrder, id)
}

func (p *parser) walkBody(n *xmlNode, parent walkState) error {
	if n.Comment {
		text := normalizeText(n.Text)
		if text != "" {
			p.comments = append(p.comments, commentAnnotation{
				Text:  text,
				Begin: parent.Begin,
				End:   parent.End,
			})
		}
		return nil
	}

	begin, end, err := p.resolveTiming(n, parent)
	if err != nil {
		return fmt.Errorf("element %q: %w", n.Name.Local, err)
	}
	regionID := parent.RegionID
	if id, ok := attrValue(n, "region"); ok && id != "" {
		regionID = id
	}
	style := parent.Style
	if regionID != "" && regionID != parent.RegionID {
		if region, ok := p.doc.Regions[regionID]; ok {
			style = region.Style
		} else {
			p.warnings = append(p.warnings, fmt.Sprintf("unknown region %q", regionID))
			style = defaultStyle()
		}
	}
	raw := p.sheet.rawForNode(n)
	applyRawStyle(&style, raw, p.canvasW, p.canvasH)

	if isMetadataNode(n) {
		p.addAnnotation(n, begin, end, regionID, style, "metadata-only")
		return nil
	}

	if source := p.backgroundSource(n, raw); source != "" {
		p.addImageResource(n, source, begin, end, regionID, style)
	}

	emittedText := false
	if p.shouldEmitText(n, parent.InTextBlock, style) {
		text := p.textContent(n, style)
		if text != "" || isAnnotationNode(n) {
			p.addTextResource(n, text, begin, end, regionID, style)
			emittedText = true
		}
	}

	childState := walkState{
		Begin:       begin,
		End:         end,
		Style:       style,
		RegionID:    regionID,
		InTextBlock: parent.InTextBlock || emittedText,
	}
	for _, child := range n.Children {
		if child.Name.Local == "#text" && !child.Comment {
			continue
		}
		if err := p.walkBody(child, childState); err != nil {
			return err
		}
	}
	return nil
}

func (p *parser) resolveTiming(n *xmlNode, parent walkState) (time.Duration, time.Duration, error) {
	begin := parent.Begin
	if value, ok := attrValue(n, "begin"); ok && value != "" {
		d, err := parseTTMLTime(value, p.doc.FrameRate, p.doc.TickRate)
		if err != nil {
			return 0, 0, fmt.Errorf("begin: %w", err)
		}
		begin += d
	}
	end := parent.End
	if value, ok := attrValue(n, "end"); ok && value != "" {
		d, err := parseTTMLTime(value, p.doc.FrameRate, p.doc.TickRate)
		if err != nil {
			return 0, 0, fmt.Errorf("end: %w", err)
		}
		end = parent.Begin + d
	} else if value, ok := attrValue(n, "dur"); ok && value != "" {
		d, err := parseTTMLTime(value, p.doc.FrameRate, p.doc.TickRate)
		if err != nil {
			return 0, 0, fmt.Errorf("dur: %w", err)
		}
		end = begin + d
	}
	if parent.End >= 0 && end >= 0 && end > parent.End {
		end = parent.End
	}
	if end >= 0 && end < begin {
		return 0, 0, fmt.Errorf("end time %s precedes begin time %s", formatTimestamp(end), formatTimestamp(begin))
	}
	return begin, end, nil
}

func (p *parser) backgroundSource(n *xmlNode, raw map[string]string) string {
	if n.Name.Local == "image" {
		for _, key := range []string{"src", "href", "backgroundImage"} {
			if value := strings.TrimSpace(raw[key]); value != "" {
				return value
			}
		}
		return ""
	}
	return strings.TrimSpace(raw["backgroundImage"])
}

func (p *parser) shouldEmitText(n *xmlNode, inTextBlock bool, style Style) bool {
	if inTextBlock && !isAnnotationNode(n) {
		return false
	}
	text := p.textContent(n, style)
	if strings.TrimSpace(text) == "" && !isAnnotationNode(n) {
		return false
	}
	switch n.Name.Local {
	case "p", "span", "annotation", "comment", "note":
		return true
	case "div", "body":
		return hasDirectText(n) && !hasDescendant(n, "p")
	default:
		return isAnnotationNode(n) || hasDirectText(n)
	}
}

func (p *parser) addTextResource(n *xmlNode, text string, begin, end time.Duration, regionID string, style Style) {
	kind := KindForegroundText
	if isAnnotationNode(n) {
		kind = KindAnnotation
	}
	rect := p.resourceRect(regionID, style, kind)
	reasons := hiddenReasonsForStyle(style, text)
	res := &Resource{
		ID:            p.makeID(n, kind),
		Kind:          kind,
		Begin:         begin,
		End:           end,
		RegionID:      regionID,
		Rect:          rect,
		Text:          text,
		Style:         style,
		HiddenReasons: reasons,
	}
	p.doc.Resources = append(p.doc.Resources, res)
}

func (p *parser) addAnnotation(n *xmlNode, begin, end time.Duration, regionID string, style Style, extraReason string) {
	text := p.textContent(n, style)
	reasons := hiddenReasonsForStyle(style, text)
	if extraReason != "" {
		reasons = append(reasons, extraReason)
	}
	p.doc.Resources = append(p.doc.Resources, &Resource{
		ID:            p.makeID(n, KindAnnotation),
		Kind:          KindAnnotation,
		Begin:         begin,
		End:           end,
		RegionID:      regionID,
		Rect:          p.resourceRect(regionID, style, KindAnnotation),
		Text:          text,
		Style:         style,
		HiddenReasons: uniqueStrings(reasons),
	})
}

func (p *parser) textContent(root *xmlNode, base Style) string {
	var b strings.Builder
	var walk func(*xmlNode, Style, bool)
	walk = func(cur *xmlNode, inherited Style, isRoot bool) {
		if cur.Comment {
			return
		}
		if !isRoot && isAnnotationNode(cur) {
			return
		}
		style := inherited
		applyRawStyle(&style, p.sheet.rawForNode(cur), p.canvasW, p.canvasH)
		if !isRoot && (style.Display == "none" || style.Visibility == "hidden" || style.Visibility == "collapse" || style.Opacity <= 0) {
			return
		}
		if cur.Name.Local == "br" {
			b.WriteByte('\n')
			return
		}
		if isRoot {
			style.Display = "auto"
			style.Visibility = "visible"
			style.Opacity = 1
		}
		for _, part := range cur.Content {
			if part.Child == nil {
				b.WriteString(part.Text)
				continue
			}
			walk(part.Child, style, false)
		}
	}
	walk(root, base, true)
	return normalizeText(b.String())
}

func (p *parser) addImageResource(n *xmlNode, source string, begin, end time.Duration, regionID string, style Style) {
	id := p.makeID(n, KindBackgroundImage)
	media := loadMedia(source, p.baseDir)
	reasons := hiddenReasonsForImage(style)
	if media.warning != "" {
		reasons = append(reasons, "media-unavailable")
		p.warnings = append(p.warnings, fmt.Sprintf("%s: %s", id, media.warning))
	}
	mediaType := media.format
	if mediaType == "" {
		mediaType = mimeFromSource(source)
	}
	p.doc.Resources = append(p.doc.Resources, &Resource{
		ID:            id,
		Kind:          KindBackgroundImage,
		Begin:         begin,
		End:           end,
		RegionID:      regionID,
		Rect:          p.resourceRect(regionID, style, KindBackgroundImage),
		MediaType:     mediaType,
		Source:        media.display,
		SourceBytes:   media.size,
		Image:         media.image,
		Style:         style,
		HiddenReasons: uniqueStrings(reasons),
		ParseWarning:  media.warning,
	})
}

func hiddenReasonsForImage(style Style) []string {
	var reasons []string
	if style.Display == "none" {
		reasons = append(reasons, "display-none")
	}
	if style.Visibility == "hidden" || style.Visibility == "collapse" {
		reasons = append(reasons, "visibility-hidden")
	}
	if style.Opacity <= 0 {
		reasons = append(reasons, "zero-opacity")
	}
	return reasons
}

func mimeFromSource(source string) string {
	if strings.HasPrefix(source, "data:") {
		comma := strings.IndexByte(source, ',')
		if comma > 5 {
			meta := source[5:comma]
			if semi := strings.IndexByte(meta, ';'); semi >= 0 {
				meta = meta[:semi]
			}
			return meta
		}
	}
	ext := strings.ToLower(filepath.Ext(source))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	}
	return ""
}

func (p *parser) resourceRect(regionID string, style Style, kind ResourceKind) Rect {
	rect := Rect{}
	if region, ok := p.doc.Regions[regionID]; ok {
		rect = region.Rect
	} else if kind == KindBackgroundImage {
		rect = Rect{W: p.canvasW, H: p.canvasH}
	} else {
		rect = Rect{
			X: int(math.Round(float64(p.canvasW) * 0.05)),
			Y: int(math.Round(float64(p.canvasH) * 0.70)),
			W: int(math.Round(float64(p.canvasW) * 0.90)),
			H: int(math.Round(float64(p.canvasH) * 0.25)),
		}
	}
	if style.HasOrigin {
		rect.X = int(math.Round(resolveLength(style.OriginX, style.OriginXUnit, p.canvasW, p.canvasW)))
		rect.Y = int(math.Round(resolveLength(style.OriginY, style.OriginYUnit, p.canvasH, p.canvasH)))
	}
	if style.HasExtent {
		rect.W = int(math.Round(resolveLength(style.ExtentW, style.ExtentXUnit, p.canvasW, p.canvasW)))
		rect.H = int(math.Round(resolveLength(style.ExtentH, style.ExtentYUnit, p.canvasH, p.canvasH)))
	}
	return clampRect(rect, p.canvasW, p.canvasH)
}

func resolveLength(value float64, unit string, percentBase int, cellBase int) float64 {
	switch unit {
	case "%":
		return float64(percentBase) * value / 100
	case "c":
		return float64(cellBase) / 32 * value
	default:
		return value
	}
}

func clampRect(rect Rect, width, height int) Rect {
	if rect.W < 0 {
		rect.W = 0
	}
	if rect.H < 0 {
		rect.H = 0
	}
	if rect.X < 0 {
		rect.W += rect.X
		rect.X = 0
	}
	if rect.Y < 0 {
		rect.H += rect.Y
		rect.Y = 0
	}
	if rect.X+rect.W > width {
		rect.W = max(0, width-rect.X)
	}
	if rect.Y+rect.H > height {
		rect.H = max(0, height-rect.Y)
	}
	return rect
}

func (p *parser) makeID(n *xmlNode, kind ResourceKind) string {
	base := ""
	if n != nil {
		base, _ = attrValue(n, "id")
	}
	suffix := map[ResourceKind]string{
		KindBackgroundImage: "background",
		KindForegroundText:  "text",
		KindAnnotation:      "annotation",
	}[kind]
	if base != "" {
		base += "-" + suffix
	} else {
		base = fmt.Sprintf("%s-%03d", suffix, len(p.doc.Resources)+1)
	}
	if !p.resourceIDs[base] {
		p.resourceIDs[base] = true
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !p.resourceIDs[candidate] {
			p.resourceIDs[candidate] = true
			return candidate
		}
	}
}

func (p *parser) finishTimeline(root *xmlNode) {
	maxEnd := time.Duration(0)
	for _, res := range p.doc.Resources {
		if res.End > maxEnd {
			maxEnd = res.End
		}
	}
	if rootEnd := p.rootEnd(root); rootEnd > maxEnd {
		maxEnd = rootEnd
	}
	if maxEnd <= 0 {
		maxEnd = 10 * time.Second
		p.warnings = append(p.warnings, "no finite cue end found; using a 10s preview timeline")
	}
	for _, comment := range p.comments {
		end := comment.End
		if end < 0 {
			end = maxEnd
		}
		p.doc.Resources = append(p.doc.Resources, &Resource{
			ID:            p.makeID(nil, KindAnnotation),
			Kind:          KindAnnotation,
			Begin:         comment.Begin,
			End:           end,
			Rect:          p.resourceRect("", defaultStyle(), KindAnnotation),
			Text:          comment.Text,
			Style:         defaultStyle(),
			HiddenReasons: []string{"xml-comment", "metadata-only"},
		})
	}
	for _, res := range p.doc.Resources {
		if res.End < 0 {
			res.End = maxEnd
		}
		if res.End > maxEnd {
			maxEnd = res.End
		}
	}
	p.doc.Duration = maxEnd
}

func walkNodes(n *xmlNode, fn func(*xmlNode)) {
	if n == nil {
		return
	}
	fn(n)
	for _, child := range n.Children {
		walkNodes(child, fn)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
