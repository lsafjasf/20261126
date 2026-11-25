package cuemap

import (
	"fmt"
	"math"
)

func placeHorizontalLine(index int, mapping Mapping, spans []RubySpan, metrics Metrics) LineLayout {
	pitch := metrics.BaseSize + metrics.CharSpacing
	upper := pitch + maxRubyLength(spans)*metrics.RubySize

	var line LineLayout
	for candidate := pitch; candidate <= upper; candidate++ {
		line = buildHorizontalLine(index, mapping, spans, metrics, candidate)
		if lineHasNoCellCollision(line) {
			line.BasePitch = candidate
			return line
		}
		pitch = candidate
	}
	line.BasePitch = upper
	return line
}

func buildHorizontalLine(index int, mapping Mapping, spans []RubySpan, metrics Metrics, pitch int) LineLayout {
	line := LineLayout{Index: index, Mapping: mapping}
	hasRuby := len(spans) > 0
	topInset := 0
	if hasRuby {
		topInset = metrics.RubySize + metrics.RubyGap
	}

	for slot := 0; slot < len(mapping.ScreenOrder); slot++ {
		logicalCluster := mapping.ScreenOrder[slot]
		cell := Cell{
			Kind:           CellBase,
			Text:           mapping.Clusters[logicalCluster].Text,
			ScreenSlot:     slot,
			LogicalCluster: logicalCluster,
			Rect: Rect{
				X:      slot * pitch,
				Y:      topInset,
				Width:  metrics.BaseSize,
				Height: metrics.BaseSize,
			},
		}
		line.BaseCells = append(line.BaseCells, cell)
		line.Cells = append(line.Cells, cell)
	}

	for rubyIndex, span := range spans {
		minSlot, maxSlot := rubySlotInterval(mapping, span)
		anchorCenterX := (minSlot+maxSlot)*pitch/2 + metrics.BaseSize/2
		rubyRunes := []rune(span.Text)
		totalWidth := len(rubyRunes) * metrics.RubySize
		startX := anchorCenterX - totalWidth/2

		for charIndex, r := range rubyRunes {
			cell := Cell{
				Kind:      CellRuby,
				Text:      string(r),
				RubyIndex: rubyIndex,
				Rect: Rect{
					X:      startX + charIndex*metrics.RubySize,
					Y:      0,
					Width:  metrics.RubySize,
					Height: metrics.RubySize,
				},
			}
			line.RubyCells = append(line.RubyCells, cell)
			line.Cells = append(line.Cells, cell)
		}
	}

	line.Bounds = unionCellBounds(line.Cells)
	return line
}

func placeVerticalLine(index int, mapping Mapping, spans []RubySpan, metrics Metrics) LineLayout {
	pitch := metrics.BaseSize + metrics.CharSpacing
	upper := pitch + maxRubyLength(spans)*metrics.RubySize

	var line LineLayout
	for candidate := pitch; candidate <= upper; candidate++ {
		line = buildVerticalLine(index, mapping, spans, metrics, candidate)
		normalizeLineOrigin(&line)
		if lineHasNoCellCollision(line) {
			line.BasePitch = candidate
			return line
		}
		pitch = candidate
	}
	line.BasePitch = upper
	return line
}

func buildVerticalLine(index int, mapping Mapping, spans []RubySpan, metrics Metrics, pitch int) LineLayout {
	line := LineLayout{Index: index, Mapping: mapping}

	for slot := 0; slot < len(mapping.ScreenOrder); slot++ {
		logicalCluster := mapping.ScreenOrder[slot]
		cell := Cell{
			Kind:           CellBase,
			Text:           mapping.Clusters[logicalCluster].Text,
			ScreenSlot:     slot,
			LogicalCluster: logicalCluster,
			Rect: Rect{
				X:      0,
				Y:      slot * pitch,
				Width:  metrics.BaseSize,
				Height: metrics.BaseSize,
			},
		}
		line.BaseCells = append(line.BaseCells, cell)
		line.Cells = append(line.Cells, cell)
	}

	rubyX := metrics.BaseSize + metrics.RubyGap
	for rubyIndex, span := range spans {
		minSlot, maxSlot := rubySlotInterval(mapping, span)
		anchorCenterY := (minSlot+maxSlot)*pitch/2 + metrics.BaseSize/2
		rubyRunes := []rune(span.Text)
		totalHeight := len(rubyRunes) * metrics.RubySize
		startY := anchorCenterY - totalHeight/2

		for charIndex, r := range rubyRunes {
			cell := Cell{
				Kind:      CellRuby,
				Text:      string(r),
				RubyIndex: rubyIndex,
				Rect: Rect{
					X:      rubyX,
					Y:      startY + charIndex*metrics.RubySize,
					Width:  metrics.RubySize,
					Height: metrics.RubySize,
				},
			}
			line.RubyCells = append(line.RubyCells, cell)
			line.Cells = append(line.Cells, cell)
		}
	}

	line.Bounds = unionCellBounds(line.Cells)
	return line
}

func rubySlotInterval(mapping Mapping, span RubySpan) (int, int) {
	startCluster := mapping.Runes[span.StartRune].Cluster
	endCluster := mapping.Runes[span.EndRune-1].Cluster + 1
	minSlot := math.MaxInt
	maxSlot := math.MinInt
	for cluster := startCluster; cluster < endCluster; cluster++ {
		slot := mapping.LogicalToScreen[cluster]
		if slot < minSlot {
			minSlot = slot
		}
		if slot > maxSlot {
			maxSlot = slot
		}
	}
	return minSlot, maxSlot
}

func maxRubyLength(spans []RubySpan) int {
	max := 1
	for _, span := range spans {
		if length := len([]rune(span.Text)); length > max {
			max = length
		}
	}
	return max
}

func unionCellBounds(cells []Cell) Rect {
	if len(cells) == 0 {
		return Rect{}
	}
	minX, minY := math.MaxInt, math.MaxInt
	maxX, maxY := math.MinInt, math.MinInt
	for _, cell := range cells {
		minX = minInt(minX, cell.Rect.X)
		minY = minInt(minY, cell.Rect.Y)
		maxX = maxInt(maxX, cell.Rect.Right())
		maxY = maxInt(maxY, cell.Rect.Bottom())
	}
	return Rect{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
}

func normalizeLineOrigin(line *LineLayout) {
	if line.Bounds.X == 0 && line.Bounds.Y == 0 {
		return
	}
	dx, dy := -line.Bounds.X, -line.Bounds.Y
	translateLine(line, dx, dy)
	line.Bounds.X = 0
	line.Bounds.Y = 0
}

func translateLine(line *LineLayout, dx, dy int) {
	for i := range line.Cells {
		line.Cells[i].Rect.X += dx
		line.Cells[i].Rect.Y += dy
	}
	for i := range line.BaseCells {
		line.BaseCells[i].Rect.X += dx
		line.BaseCells[i].Rect.Y += dy
	}
	for i := range line.RubyCells {
		line.RubyCells[i].Rect.X += dx
		line.RubyCells[i].Rect.Y += dy
	}
	line.Bounds.X += dx
	line.Bounds.Y += dy
}

func lineHasNoCellCollision(line LineLayout) bool {
	for i := 0; i < len(line.Cells); i++ {
		for j := i + 1; j < len(line.Cells); j++ {
			if line.Cells[i].Rect.Intersects(line.Cells[j].Rect) {
				return false
			}
		}
	}
	return true
}

// Collision identifies two positioned cells that overlap.
type Collision struct {
	LineA int
	CellA Cell
	LineB int
	CellB Cell
}

func (c *Collision) Error() string {
	return fmt.Sprintf("line %d %s cell %q overlaps line %d %s cell %q",
		c.LineA, c.CellA.Kind, c.CellA.Text,
		c.LineB, c.CellB.Kind, c.CellB.Text)
}

// FindCollision checks all final cells, including across hard lines.
func FindCollision(result Result) *Collision {
	for lineA := range result.Lines {
		cellsA := result.Lines[lineA].Cells
		for i := 0; i < len(cellsA); i++ {
			for lineB := lineA; lineB < len(result.Lines); lineB++ {
				cellsB := result.Lines[lineB].Cells
				startJ := 0
				if lineA == lineB {
					startJ = i + 1
				}
				for j := startJ; j < len(cellsB); j++ {
					if cellsA[i].Rect.Intersects(cellsB[j].Rect) {
						return &Collision{
							LineA: lineA, CellA: cellsA[i],
							LineB: lineB, CellB: cellsB[j],
						}
					}
				}
			}
		}
	}
	return nil
}

func stackLineBounds(lines []LineLayout, mode WritingMode, metrics Metrics) Rect {
	if len(lines) == 0 {
		return Rect{Width: metrics.Padding * 2, Height: metrics.Padding * 2}
	}

	if mode == ModeVertical {
		maxWidth, maxHeight := 0, 0
		for _, line := range lines {
			maxWidth = maxInt(maxWidth, line.Bounds.Width)
			maxHeight = maxInt(maxHeight, line.Bounds.Height)
		}
		columnStep := maxWidth + metrics.LineSpacing
		for i := range lines {
			x := metrics.Padding + (len(lines)-1-i)*columnStep
			y := metrics.Padding
			translateLine(&lines[i], x-lines[i].Bounds.X, y-lines[i].Bounds.Y)
			lines[i].ColumnPitch = columnStep
			lines[i].Bounds.X = x
			lines[i].Bounds.Y = y
		}
		return Rect{
			Width:  metrics.Padding*2 + (len(lines)-1)*columnStep + maxWidth,
			Height: metrics.Padding*2 + maxHeight,
		}
	}

	y := metrics.Padding
	maxWidth := 0
	for i := range lines {
		x := metrics.Padding
		translateLine(&lines[i], x-lines[i].Bounds.X, y-lines[i].Bounds.Y)
		lines[i].Bounds.X = x
		lines[i].Bounds.Y = y
		y += lines[i].Bounds.Height + metrics.LineSpacing
		maxWidth = maxInt(maxWidth, lines[i].Bounds.Width)
	}
	return Rect{
		Width:  metrics.Padding*2 + maxWidth,
		Height: y - metrics.LineSpacing + metrics.Padding,
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
