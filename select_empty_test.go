package subselect

import "testing"

func TestSelect_EmptyCriteriaUsesDefaultNotFallbackLanguage(t *testing.T) {
	_, sel := fixtureSelector("en")
	d := sel.Select(Criteria{})
	if d.Reason != ReasonDefault || d.Track == nil || d.Track.ID != "zh-main" {
		t.Fatalf("empty criteria = %s/%v, want default zh-main", d.Reason, d.Track)
	}
}
