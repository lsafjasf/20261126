package safereflow

import (
	"os"
	"strings"
	"testing"
)

func TestLoadSafeCSV(t *testing.T) {
	csv := "start,end,x,y,width,height,confidence\n" +
		"0,2,10,20,100,200,0.9\n" +
		"2,4,30,40,110,210,\n"
	track, err := LoadSafeCSV(strings.NewReader(csv), Canvas{Width: 1920, Height: 1080})
	if err != nil {
		t.Fatal(err)
	}
	spans := track.Spans()
	if len(spans) != 2 {
		t.Fatalf("spans=%d", len(spans))
	}
	if spans[0].Rect.X != 10 || spans[0].Conf() != 0.9 {
		t.Fatalf("span0 = %+v", spans[0])
	}
	if spans[1].Confidence != nil {
		t.Fatal("empty confidence must be unknown")
	}
}

func TestLoadSafeCSVFixture(t *testing.T) {
	if _, err := os.Stat("testdata/safe_areas.csv"); err != nil {
		t.Skip("fixture missing")
	}
	track, err := LoadSafeCSVFile("testdata/safe_areas.csv", Canvas{Width: 1920, Height: 1080})
	if err != nil {
		t.Fatal(err)
	}
	if len(track.Spans()) != 3 {
		t.Fatalf("spans=%d", len(track.Spans()))
	}
}
