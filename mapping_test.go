package cuemap

import (
	"reflect"
	"testing"
)

func TestMapLineReversesArabicRun(t *testing.T) {
	logical := string([]rune{0x0645, 0x0631, 0x062D, 0x0628, 0x0627})
	mapping := MapLine(logical, DirectionAuto)

	if mapping.BaseDirection != DirectionRTL || mapping.BaseLevel != 1 {
		t.Fatalf("base = %v/%d, want RTL level 1", mapping.BaseDirection, mapping.BaseLevel)
	}
	want := []int{4, 3, 2, 1, 0}
	if !reflect.DeepEqual(mapping.ScreenOrder, want) {
		t.Fatalf("ScreenOrder = %v, want %v", mapping.ScreenOrder, want)
	}
	assertMappingRoundTrip(t, mapping)
}

func TestMapLineKeepsCombiningMarksOnBaseCluster(t *testing.T) {
	logical := string([]rune{
		0x0645, 0x064E,
		0x0631, 0x0652,
		0x062D, 0x064E,
		0x0628, 0x064B,
		0x0627,
	})
	mapping := MapLine(logical, DirectionAuto)

	if len(mapping.Clusters) != 5 {
		t.Fatalf("cluster count = %d, want 5: %#v", len(mapping.Clusters), mapping.Clusters)
	}
	for _, runeIndexes := range [][2]int{{0, 1}, {2, 3}, {4, 5}, {6, 7}} {
		base := mapping.RuneToScreen[runeIndexes[0]]
		mark := mapping.RuneToScreen[runeIndexes[1]]
		if base != mark {
			t.Fatalf("runes %d/%d screens = %d/%d, want same slot", runeIndexes[0], runeIndexes[1], base, mark)
		}
	}
	if got := mapping.ScreenToRunes[mapping.RuneToScreen[0]]; !reflect.DeepEqual(got, []int{0, 1}) {
		t.Fatalf("base screen runes = %v, want [0 1]", got)
	}
	assertMappingRoundTrip(t, mapping)
}

func TestMapLineMixedChineseArabicKeepsLogicalMapping(t *testing.T) {
	logical := "中文 " + string([]rune{0x0645, 0x0631, 0x062D, 0x0628, 0x0627})
	mapping := MapLine(logical, DirectionAuto)

	if mapping.BaseDirection != DirectionLTR {
		t.Fatalf("base direction = %v, want LTR", mapping.BaseDirection)
	}
	want := []int{0, 1, 2, 7, 6, 5, 4, 3}
	if !reflect.DeepEqual(mapping.ScreenOrder, want) {
		t.Fatalf("ScreenOrder = %v, want %v", mapping.ScreenOrder, want)
	}
	assertMappingRoundTrip(t, mapping)
}

func TestMapLineMixedArabicEnglishDoesNotReverseEnglish(t *testing.T) {
	logical := "مرحبا Hello عالم"
	mapping := MapLine(logical, DirectionAuto)

	assertMappingRoundTrip(t, mapping)
	english := []int{6, 7, 8, 9, 10}
	englishScreens := make([]int, len(english))
	for i, logicalCluster := range english {
		englishScreens[i] = mapping.LogicalToScreen[logicalCluster]
	}
	for i := 1; i < len(englishScreens); i++ {
		if englishScreens[i] != englishScreens[i-1]+1 {
			t.Fatalf("English screen slots not increasing: %v", englishScreens)
		}
	}
}

func TestMapTextResetsMappingPerHardLine(t *testing.T) {
	mappings := MapText("中文\nمرحبا", DirectionAuto)
	if len(mappings) != 2 {
		t.Fatalf("mapping count = %d, want 2", len(mappings))
	}
	if mappings[0].BaseDirection != DirectionLTR {
		t.Fatalf("first line direction = %v, want LTR", mappings[0].BaseDirection)
	}
	if mappings[1].BaseDirection != DirectionRTL {
		t.Fatalf("second line direction = %v, want RTL", mappings[1].BaseDirection)
	}
}

func TestResolveBaseDirectionFromBidiMarks(t *testing.T) {
	if direction, level := ResolveBaseDirection("\u200FHello", DirectionAuto); direction != DirectionRTL || level != 1 {
		t.Fatalf("RLM base = %v/%d, want RTL/1", direction, level)
	}
	if direction, _ := ResolveBaseDirection("\u200Eمرحبا", DirectionAuto); direction != DirectionLTR {
		t.Fatalf("LRM base = %v, want LTR", direction)
	}
}

func TestMapLineKeepsNumericSequencesInLogicalOrder(t *testing.T) {
	mapping := MapLine("12,345", DirectionLTR)
	if got := mapping.ScreenOrder; !reflect.DeepEqual(got, []int{0, 1, 2, 3, 4, 5}) {
		t.Fatalf("numeric ScreenOrder = %v, want identity", got)
	}
	assertMappingRoundTrip(t, mapping)
}

func TestForcedRTLKeepsPureLatinWordReadable(t *testing.T) {
	mapping := MapLine("Hello", DirectionRTL)
	if got := mapping.ScreenOrder; !reflect.DeepEqual(got, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("RTL Latin ScreenOrder = %v, want identity LTR run", got)
	}
}

func assertMappingRoundTrip(t *testing.T, mapping Mapping) {
	t.Helper()
	n := len(mapping.Clusters)
	if len(mapping.ScreenOrder) != n || len(mapping.LogicalToScreen) != n {
		t.Fatalf("mapping length mismatch: clusters=%d order=%d toScreen=%d", n, len(mapping.ScreenOrder), len(mapping.LogicalToScreen))
	}
	seen := make([]bool, n)
	for screen, logical := range mapping.ScreenOrder {
		if logical < 0 || logical >= n || seen[logical] {
			t.Fatalf("ScreenOrder is not a permutation: %v", mapping.ScreenOrder)
		}
		seen[logical] = true
		if mapping.LogicalToScreen[logical] != screen {
			t.Fatalf("LogicalToScreen[%d] = %d, want %d", logical, mapping.LogicalToScreen[logical], screen)
		}
	}
}
