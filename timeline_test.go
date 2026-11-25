package vttlayout

import "testing"

func TestBuildStages(t *testing.T) {
	cues := []*Cue{
		{Order: 0, StartMS: 0, EndMS: 4000},
		{Order: 1, StartMS: 1000, EndMS: 3000},
		{Order: 2, StartMS: 2000, EndMS: 5000},
	}
	stages := BuildStages(cues)
	if len(stages) != 5 {
		t.Fatalf("want 5 stages, got %d: %+v", len(stages), stages)
	}
	// [1000,2000): cues 0,1
	if len(stages[1].Cues) != 2 || stages[1].StartMS != 1000 || stages[1].EndMS != 2000 {
		t.Fatalf("bad stage 1: %+v", stages[1])
	}
	// [2000,3000): all three
	if len(stages[2].Cues) != 3 {
		t.Fatalf("bad stage 2: %+v", stages[2])
	}
}

func TestOverlapTime(t *testing.T) {
	a := &Cue{StartMS: 0, EndMS: 1000}
	b := &Cue{StartMS: 1000, EndMS: 2000}
	if OverlapTime(a, b) {
		t.Fatal("touching intervals must not overlap")
	}
	b.StartMS = 999
	if !OverlapTime(a, b) {
		t.Fatal("expected overlap")
	}
}

func TestBuildClusters(t *testing.T) {
	cues := []*Cue{
		{Order: 0, StartMS: 0, EndMS: 1000},
		{Order: 1, StartMS: 900, EndMS: 2000},
		{Order: 2, StartMS: 5000, EndMS: 6000},
	}
	cl := BuildClusters(cues)
	if len(cl) != 2 {
		t.Fatalf("want 2 clusters, got %d", len(cl))
	}
}
