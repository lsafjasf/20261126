package safereflow

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// LoadSafeCSV reads interval safe areas from CSV.
//
// The first row must be a header containing at least:
//
//	start,end,x,y,width,height
//
// Optional columns: id,confidence,source. Column order is flexible.
// CSV rectangles are always in pixels; use JSON for normalized coordinates.
func LoadSafeCSV(r io.Reader, canvas Canvas) (*SafeTrack, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("safereflow: read safe-area csv: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("safereflow: csv needs a header and at least one row")
	}

	idx := map[string]int{}
	for i, name := range rows[0] {
		idx[strings.ToLower(strings.TrimSpace(name))] = i
	}
	need := []string{"start", "end", "x", "y"}
	for _, col := range need {
		if _, ok := idx[col]; !ok {
			return nil, fmt.Errorf("safereflow: csv missing required column %q", col)
		}
	}
	widthCol := firstCol(idx, "width", "w")
	heightCol := firstCol(idx, "height", "h")
	if widthCol < 0 || heightCol < 0 {
		return nil, fmt.Errorf("safereflow: csv missing width/height columns")
	}

	doc := SafeDocument{Canvas: canvas, Unit: "pixel"}
	for ri, row := range rows[1:] {
		if len(row) == 1 && strings.TrimSpace(row[0]) == "" {
			continue
		}
		get := func(col string) string {
			if i, ok := idx[col]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		start, err1 := strconv.ParseFloat(get("start"), 64)
		end, err2 := strconv.ParseFloat(get("end"), 64)
		x, err3 := strconv.ParseFloat(get("x"), 64)
		y, err4 := strconv.ParseFloat(get("y"), 64)
		w, err5 := strconv.ParseFloat(row[widthCol], 64)
		h, err6 := strconv.ParseFloat(row[heightCol], 64)
		if err := firstErr(err1, err2, err3, err4, err5, err6); err != nil {
			return nil, fmt.Errorf("safereflow: csv row %d: %w", ri+2, err)
		}
		sp := SafeSpan{
			TimeRange: TimeRange{Start: start, End: end},
			Rect:      Rect{X: x, Y: y, W: w, H: h},
			ID:        get("id"),
			Source:    get("source"),
		}
		if c := get("confidence"); c != "" {
			v, err := strconv.ParseFloat(c, 64)
			if err != nil {
				return nil, fmt.Errorf("safereflow: csv row %d confidence: %w", ri+2, err)
			}
			sp.Confidence = &v
		}
		doc.SafeAreas = append(doc.SafeAreas, sp)
	}
	return NewSafeTrack(doc)
}

// LoadSafeCSVFile opens a CSV safe-area file.
func LoadSafeCSVFile(path string, canvas Canvas) (*SafeTrack, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LoadSafeCSV(f, canvas)
}

func firstCol(idx map[string]int, names ...string) int {
	for _, n := range names {
		if i, ok := idx[n]; ok {
			return i
		}
	}
	return -1
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
