package ttmlevents

import (
	"testing"
	"time"
)

func TestSequentialTimeContainer(t *testing.T) {
	doc := mustParse(t, `<tt><body>
      <div timeContainer="seq">
        <p xml:id="a" begin="0s" dur="1s">A</p>
        <p xml:id="b" begin="0.5s" dur="1s">B</p>
      </div>
    </body></tt>`)
	tl := ResolveTiming(doc)

	a := timingByID(t, tl, "a")
	b := timingByID(t, tl, "b")
	if a.Effective.Begin != 0 || endOrZero(a.Effective) != time.Second {
		t.Fatalf("a = [%v,%v), want [0s,1s)", a.Effective.Begin, endOrZero(a.Effective))
	}
	if b.Effective.Begin != 1500*time.Millisecond || endOrZero(b.Effective) != 2500*time.Millisecond {
		t.Fatalf("b = [%v,%v), want [1.5s,2.5s)", b.Effective.Begin, endOrZero(b.Effective))
	}
}
