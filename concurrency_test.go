package cea708

import "sync"
import "testing"

func TestManagerConcurrentUse(t *testing.T) {
	m := NewManager(WithSelectedService(1))
	var wg sync.WaitGroup

	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				id := ServiceID(1 + (g+i)%2)
				m.Update(id, func(d *Decoder) {
					d.DefineWindow(WindowConfig{ID: 0, RowCount: 1, ColCount: 12, Visible: true})
					d.WriteText("busy")
				})
				m.SelectService(ServiceID(1 + i%2))
				_ = m.Frames()
				_ = m.ClearEvents()
				_ = m.Snapshot()
				_, _ = m.LatchedScreen(id)
			}
		}(g)
	}
	wg.Wait()

	if _, ok := m.Selected(); !ok {
		t.Fatal("selection lost under concurrency")
	}
}
