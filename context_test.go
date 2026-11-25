package subselect

import (
	"context"
	"errors"
	"testing"
)

func TestSwitch_ContextCanceledReturnsCurrentView(t *testing.T) {
	cat, sel := fixtureSelector()
	p := NewPlayer("cancel", sel)
	if _, err := p.Switch(context.Background(), Criteria{Language: "en"}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := p.Switch(ctx, Criteria{Language: "zh"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if result.View == nil || result.View.Decision.Track == nil ||
		result.View.Decision.Track.ID != "en-main" {
		t.Fatalf("canceled switch must preserve current en-main view, got %+v", result.View)
	}
	if err := checkViewConsistency(cat, result.View); err != nil {
		t.Fatal(err)
	}
}
