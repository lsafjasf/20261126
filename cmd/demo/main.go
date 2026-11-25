// Command demo reproduces the reported bug scenario end to end: a device
// switches from 44.1 kHz to 48 kHz mid-playback, disappears, and recovers.
// The engine renegotiates each time and keeps a sample-accurate position.
package main

import (
	"fmt"
	"time"

	"audioneg/audio"
)

func main() {
	dev := audio.NewSimDevice(audio.Capabilities{
		SampleRates: []int{44100},
		Layouts:     []audio.ChannelLayout{audio.LayoutStereo},
		Formats:     []audio.SampleFormat{audio.FormatS16},
	})
	eng := audio.NewEngine(audio.StreamParams{
		SampleRate: 44100,
		Layout:     audio.LayoutStereo,
		Format:     audio.FormatS16,
	})
	eng.OnChange = func(s audio.State, n audio.Negotiation) {
		fmt.Printf("[event] state=%-11s selected=%v fallbacks=%v\n", s, n.Selected, n.Fallbacks)
	}
	if err := eng.Attach(dev); err != nil {
		panic(err)
	}
	defer eng.Close()
	time.Sleep(50 * time.Millisecond)
	render := func(frames int64, label string) {
		if err := eng.Render(frames); err != nil {
			fmt.Printf("[render] %s: %v\n", label, err)
			return
		}
		src, devFrames, total := eng.Position()
		fmt.Printf("[render] %-18s source=%d dev(cur)=%d dev(total)=%d\n", label, src, devFrames, total)
	}
	render(44100, "1s @ 44.1kHz")
	fmt.Println("--- device switches to 48 kHz ---")
	dev.SetCaps(audio.Capabilities{
		SampleRates: []int{48000},
		Layouts:     []audio.ChannelLayout{audio.LayoutStereo},
		Formats:     []audio.SampleFormat{audio.FormatS16},
	})
	time.Sleep(50 * time.Millisecond)
	render(48000, "1s @ 48kHz")
	fmt.Println("--- device unplugged ---")
	dev.Remove()
	time.Sleep(50 * time.Millisecond)
	render(4800, "while gone")
	fmt.Println("--- device replugged (48 kHz, 5.1 capable) ---")
	dev.Restore(audio.Capabilities{
		SampleRates: []int{48000},
		Layouts:     []audio.ChannelLayout{audio.LayoutStereo, audio.LayoutSurround5},
		Formats:     []audio.SampleFormat{audio.FormatS16},
	})
	time.Sleep(50 * time.Millisecond)
	render(48000, "1s after recovery")
	fmt.Println("--- recorded anchors ---")
	for _, a := range eng.Anchors() {
		fmt.Printf("seq=%d sourceFrame=%d deviceRate=%d layout=%v\n",
			a.Seq, a.SourceFrame, a.DeviceRate, a.DeviceLayout)
	}
}
