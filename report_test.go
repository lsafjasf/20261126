package vttlayout

import (
	"encoding/json"
	"testing"
)

func TestReportJSONShape(t *testing.T) {
	doc, plan := analyzeExample(t, ExampleInfeasibleVTT)
	rep := NewReport("mem", doc, plan)
	b, err := rep.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(b, &generic); err != nil {
		t.Fatalf("report JSON invalid: %v", err)
	}
	if generic["feasible"].(bool) {
		t.Fatal("infeasible report must say feasible=false")
	}
	if int(generic["infeasible_combo_count"].(float64)) != 1 {
		t.Fatalf("combo count wrong: %v", generic["infeasible_combo_count"])
	}
	cands := generic["candidates"].([]any)
	if len(cands) < 5 {
		t.Fatalf("expected >=5 candidates, got %d", len(cands))
	}
}

func TestCustomCanvasScales(t *testing.T) {
	cv := CustomCanvas(720, 1280)
	if cv.FontPx <= 0 || cv.SafeRect().H <= 0 {
		t.Fatalf("bad custom canvas %+v", cv)
	}
	doc, err := Parse(ExampleOverlapVTT)
	if err != nil {
		t.Fatal(err)
	}
	plan := Analyze(doc, cv, Options{})
	if len(plan.Boxes) != 5 {
		t.Fatal("expected 5 boxes on custom canvas")
	}
}
