package main

import (
	"os"
	"testing"
)

func loadFixture(t *testing.T, name string) *Project {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	p, err := LoadProject(data)
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	return p
}

func hasCode(rep *Report, code string) bool {
	for _, d := range rep.Diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestRecoverableFixtureEmitsMinimal(t *testing.T) {
	p := loadFixture(t, "broken_recoverable.json")
	g := BuildGraph(p)
	rep := Diagnose(p, g, "test")
	if !rep.OK {
		t.Fatalf("expected recoverable project, got fatals: %+v", rep.Diags)
	}
	for _, code := range []string{
		"MISSING_SOURCE", "MISSING_METADATA", "INEXPRESSIBLE_PROP",
		"INEXPRESSIBLE_VALUE", "DANGLING_WEAKREF", "ORPHAN_PRUNED",
	} {
		if !hasCode(rep, code) {
			t.Errorf("expected diagnostic code %s", code)
		}
	}

	SubstituteFillers(p, g)
	g = BuildGraph(p)
	minimal, err := EmitMinimal(p, g)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if err := selfCheck(minimal); err != nil {
		t.Fatalf("self check: %v", err)
	}

	filler := minimal.lookup("clip-missing")
	if filler == nil || filler.Class != "Filler" {
		t.Fatalf("clip-missing should become a Filler, got %+v", filler)
	}
	if _, ok := filler.Properties["Length"]; !ok {
		t.Error("substituted Filler must keep the original Length")
	}
	if minimal.lookup("sm-orphan") != nil {
		t.Error("orphan source mob must be pruned")
	}
	comp := minimal.lookup("comp-1")
	if _, ok := comp.Properties["DescriptiveMetadata"]; ok {
		t.Error("dangling descriptive metadata attachment must be detached")
	}
	if _, ok := comp.Properties["UserComments"]; ok {
		t.Error("inexpressible property must be dropped")
	}
}

func TestMissingEffectIsFatal(t *testing.T) {
	p := loadFixture(t, "broken_fatal.json")
	g := BuildGraph(p)
	rep := Diagnose(p, g, "test")
	if rep.OK {
		t.Fatal("missing effect definition must be fatal, never silent no-effect")
	}
	if !hasCode(rep, "MISSING_EFFECT") {
		t.Errorf("expected MISSING_EFFECT diagnostic, got %+v", rep.Diags)
	}
}

func TestMinimalProjectRoundTrips(t *testing.T) {
	p := loadFixture(t, "broken_recoverable.json")
	g := BuildGraph(p)
	if rep := Diagnose(p, g, "test"); !rep.OK {
		t.Fatalf("fatals: %+v", rep.Diags)
	}
	SubstituteFillers(p, g)
	minimal, err := EmitMinimal(p, BuildGraph(p))
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	encoded, err := MarshalProject(minimal)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	reloaded, err := LoadProject(encoded)
	if err != nil {
		t.Fatalf("reimport: %v", err)
	}
	rep := Diagnose(reloaded, BuildGraph(reloaded), "roundtrip")
	if !rep.OK || rep.Warnings != 0 {
		t.Fatalf("reimported project must be clean, got %+v", rep.Diags)
	}
}

func TestDuplicateIDsRejected(t *testing.T) {
	data := []byte(`{"objects":[
		{"id":"a","class":"Timecode","properties":{"Start":0,"FPS":25}},
		{"id":"a","class":"Timecode","properties":{"Start":1,"FPS":25}}]}`)
	if _, err := LoadProject(data); err == nil {
		t.Fatal("duplicate object IDs must be rejected")
	}
}

func TestHeaderlessDumpRootsAtMobs(t *testing.T) {
	data := []byte(`{"objects":[
		{"id":"m1","class":"SourceMob","properties":{
			"MobID":"urn:x:1","Slots":[{"ref":"s1"}]}},
		{"id":"s1","class":"TimelineMobSlot","properties":{
			"SlotID":1,"EditRate":"25/1","Segment":{"ref":"tc1"}}},
		{"id":"tc1","class":"Timecode","properties":{"Start":0,"FPS":25}}]}`)
	p, err := LoadProject(data)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g := BuildGraph(p)
	if !g.Reachable["tc1"] {
		t.Error("timecode must be reachable from the mob root")
	}
	rep := Diagnose(p, g, "test")
	if !rep.OK {
		t.Fatalf("header-less dump should diagnose clean, got %+v", rep.Diags)
	}
}
