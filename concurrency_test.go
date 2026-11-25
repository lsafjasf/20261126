package captionhub

import (
	"fmt"
	"sync"
	"testing"
)

func TestHubConcurrentConnectionsAndSnapshots(t *testing.T) {
	h := NewHub()
	mustSwitch(t, h, "LIVE", "concurrent")

	const connections = 32
	for i := 0; i < connections; i++ {
		id := fmt.Sprintf("conn-%02d", i)
		if _, err := h.ConnectAt(id, Millis(i%10)*500); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < connections; i++ {
		i := i
		id := fmt.Sprintf("conn-%02d", i)
		wg.Add(3)

		go func() {
			defer wg.Done()
			_, _ = h.ReportPosition(id, Millis((i*37)%12_000))
		}()
		go func() {
			defer wg.Done()
			_ = h.States()
		}()
		go func() {
			defer wg.Done()
			_ = h.Snapshot()
		}()
	}

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = h.AddCue(Cue{
				ID:      fmt.Sprintf("cue-%03d", i),
				StartMS: Millis(i % 10_000),
				EndMS:   Millis(i%10_000) + 1_000,
				Text:    "concurrent",
			})
		}(i)
	}

	wg.Wait()
	if states := h.States(); len(states) != connections {
		t.Fatalf("states = %d, want %d", len(states), connections)
	}
}
