package main

import (
	"fmt"
	"strings"

	"cafmarker"
)

type textOptions struct {
	showPackets bool
	allPackets  bool
}

func printText(r *cafmarker.Report, opts textOptions) {
	fmt.Printf("CAF: %s (%d bytes)\n", orDash(r.Path), r.FileSize)
	if r.Format != nil {
		f := r.Format
		fmt.Printf("format: %s %.1f Hz, %d ch, %d bits; bytes/packet=%d frames/packet=%d\n",
			f.FormatID, f.SampleRate, f.ChannelsPerFrame, f.BitsPerChannel, f.BytesPerPacket, f.FramesPerPacket)
	}
	if r.PacketTable != nil {
		p := r.PacketTable
		fmt.Printf("pakt: packets=%d valid_frames=%d priming=%d remainder=%d\n",
			p.NumberPackets, p.NumberValidFrames, p.PrimingFrames, p.RemainderFrames)
	}
	if r.Map != nil {
		m := r.Map
		fmt.Printf("map: mapped_packets=%d declared=%d coded_frames=%d audible_frames=%d audio_bytes=%d available=%d\n",
			m.PacketCountMapped, m.PacketCountDeclared, m.CodedFrameCount, m.AudibleFrameCount,
			m.AudioByteSize, m.AvailableAudioBytes)
	}

	if opts.showPackets && r.Map != nil {
		printPackets(r.Map, opts.allPackets)
	}

	fmt.Println("\nmarkers:")
	if len(r.Markers) == 0 {
		fmt.Println("  (none)")
	}
	for _, d := range r.Markers {
		m := d.Marker
		label := m.Name
		if label == "" {
			label = m.TypeName
		}
		frameText := "invalid"
		if m.FrameValid {
			frameText = fmt.Sprintf("%d", m.Frame)
		}
		fmt.Printf("  #%d id=%d %q frame=%s (raw=%.3f) -> packet=%d offset=%d classification=%s confidence=%s\n",
			d.Index, m.MarkerID, label, frameText, m.FramePosition, d.Location.PacketIndex,
			d.Location.FrameOffsetInPacket, d.Classification, d.Confidence)
		if d.Location.FileByteOffset > 0 || d.Location.AudioByteOffset > 0 {
			fmt.Printf("      bytes: audio=0x%x file=0x%x inner=%d\n",
				d.Location.AudioByteOffset, d.Location.FileByteOffset, d.Location.ByteOffsetInPacket)
		}
		for _, ev := range d.Evidence {
			fmt.Printf("      evidence: %s\n", ev)
		}
		if d.CorrectedFrame != nil {
			pkt := -1
			if d.CorrectedPacket != nil {
				pkt = *d.CorrectedPacket
			}
			fmt.Printf("      correction: frame=%d packet=%d\n", *d.CorrectedFrame, pkt)
		}
		if d.ErrorSource != "" {
			fmt.Printf("      source: %s\n", d.ErrorSource)
		}
	}

	if len(r.Issues) > 0 || len(r.Findings) > 0 {
		fmt.Println("\nissues:")
		for _, issue := range r.Issues {
			fmt.Printf("  [%s] %s\n", issue.Code, issue.Message)
		}
		for _, issue := range r.Findings {
			fmt.Printf("  [%s] %s\n", issue.Code, issue.Message)
		}
	}

	fmt.Printf("\nverdict: %s\n", r.Verdict)
	if r.RootCause != "" {
		fmt.Printf("root cause: %s\n", wrap(r.RootCause, "  "))
	}
}

func printPackets(m *cafmarker.PacketMap, all bool) {
	fmt.Println("\npackets:")
	total := int(m.PacketCountMapped)
	if !all && total > 12 {
		for i := 0; i < 6; i++ {
			if p, ok := m.PacketAt(i); ok {
				printPacket(p)
			}
		}
		fmt.Println("  ...")
		for i := total - 6; i < total; i++ {
			if p, ok := m.PacketAt(i); ok {
				printPacket(p)
			}
		}
		fmt.Printf("  (%d packets total; detailed JSON includes up to %d materialized entries; use -all-packets for text)\n", total, len(m.Packets))
		return
	}
	for i := 0; i < total; i++ {
		if p, ok := m.PacketAt(i); ok {
			printPacket(p)
		}
	}
}

func printPacket(p cafmarker.Packet) {
	flags := ""
	if len(p.Flags) > 0 {
		flags = " [" + strings.Join(p.Flags, ",") + "]"
	}
	fmt.Printf("  p=%-4d frames=[%d,%d) n=%d bytes=[%d,+%d) available=%d missing=%d%s\n",
		p.Index, p.FrameStart, p.FrameEnd, p.FrameCount, p.ByteOffset, p.ByteSize,
		p.AvailableBytes, p.MissingBytes, flags)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func wrap(s, indent string) string {
	words := strings.Fields(s)
	var b strings.Builder
	line := indent
	for _, w := range words {
		if len(line)+len(w)+1 > 100 {
			b.WriteString(strings.TrimRight(line, " "))
			b.WriteByte('\n')
			line = indent
		}
		line += w + " "
	}
	b.WriteString(strings.TrimRight(line, " "))
	return b.String()
}
