package cea708_test

import (
	"fmt"

	cea708 "cea708"
)

// Example demonstrates the core guarantee: switching from service 1 to
// service 2 never composites service-1 glyphs onto service 2, while service 1
// stays latched and is restored when switching back.
func Example() {
	m := cea708.New(cea708.WithSelectedService(cea708.Service1))

	m.Update(cea708.Service1, func(d *cea708.Decoder) {
		d.DefineWindow(cea708.WindowConfig{
			ID: 0, RowCount: 1, ColCount: 16, AnchorRow: 1, AnchorCol: 1, Visible: true,
		})
		d.WriteText("NEWS")
	})

	m.SelectService(cea708.Service2) // boundary clear; service 2 is still empty
	m.Update(cea708.Service2, func(d *cea708.Decoder) {
		d.DefineWindow(cea708.WindowConfig{
			ID: 0, RowCount: 1, ColCount: 16, AnchorRow: 1, AnchorCol: 1, Visible: true,
		})
		d.WriteText("MATCH")
	})

	frame, _ := m.LastFrame()
	fmt.Printf("service=%d old-text=%v new-text=%v\n",
		frame.Service, frame.Contains("NEWS"), frame.Contains("MATCH"))

	back := m.SelectService(cea708.Service1)
	fmt.Printf("back-on-1 news=%v service=%d\n", back.Contains("NEWS"), back.Service)

	// Output:
	// service=2 old-text=false new-text=true
	// back-on-1 news=true service=1
}
