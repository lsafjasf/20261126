package cea608

import "testing"

func buildXDS(t *testing.T, class, typ byte, content string, parity bool) []byte {
	t.Helper()
	if len(content)%2 != 0 {
		t.Fatalf("test content must have even length, got %d", len(content))
	}

	sum := int(class) + int(typ) + 0x0f
	for i := 0; i < len(content); i++ {
		sum += int(content[i])
	}
	checksum := byte((0x80 - sum%0x80) % 0x80)

	raw := []byte{class, typ}
	raw = append(raw, content...)
	raw = append(raw, 0x0f, checksum)
	if !parity {
		return raw
	}

	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = WithOddParity(b)
	}
	return out
}

func captionBytes(s string) []byte {
	body := []byte(s)
	if len(body)%2 == 1 {
		body = append(body, 0x00)
	}
	return catBytes([]byte{0x14, CmdRCL, 0x14, 0x40}, body, []byte{0x14, CmdEOC})
}

func wireCaptionBytes(s string) []byte {
	return catBytes(testRCL, testPACRow14, encodeText(s), testEOC)
}

func TestXDSPacketIsSeparateFromCaption(t *testing.T) {
	for _, parity := range []bool{false, true} {
		xds := buildXDS(t, 0x05, 0x01, "RATE", parity)
		caption := captionBytes("HELLO")
		if parity {
			caption = wireCaptionBytes("HELLO")
		}
		result := Parse(catBytes(xds, caption))

		if len(result.XDSPackets) != 1 {
			t.Fatalf("parity=%v: 应完成 1 个 XDS 包，got %d", parity, len(result.XDSPackets))
		}
		packet := result.XDSPackets[0]
		if packet.Class != 5 || packet.Type != 1 {
			t.Fatalf("parity=%v: class/type 应为 5/1，got %d/%d", parity, packet.Class, packet.Type)
		}
		if got := string(packet.Content); got != "RATE" {
			t.Fatalf("parity=%v: XDS content 应为 RATE，got %q", parity, got)
		}
		if !packet.ChecksumOK {
			t.Fatalf("parity=%v: XDS 校验和应正确，checksum=%02x raw=% X",
				parity, packet.Checksum, packet.Raw)
		}
		if got := result.Lines[13]; got != "HELLO" {
			t.Fatalf("parity=%v: XDS 不得进入字幕，line=%q text=%q", parity, got, result.Text)
		}
		if result.Text != "HELLO" {
			t.Fatalf("parity=%v: 干净字幕文本应为 HELLO，got %q", parity, result.Text)
		}
	}
}

func TestXDSStateRemainsIndependent(t *testing.T) {
	d := NewWithOptions(Options{Parity: ParityOff})
	d.Decode(buildXDS(t, 0x01, 0x02, "TIME", false))
	if d.XDS.Active {
		t.Fatal("完整 XDS 包后应退出 XDS active 状态")
	}
	if len(d.XDS.Packets) != 1 || len(d.Caption.Cues) != 0 {
		t.Fatalf("XDS 不应产生 caption cue: xds=%d cues=%d",
			len(d.XDS.Packets), len(d.Caption.Cues))
	}

	d.Decode(captionBytes("NEWS"))
	if len(d.XDS.Packets) != 1 {
		t.Fatal("字幕解码不得追加或清空 XDS 包")
	}
	if d.Lines()[13] != "NEWS" {
		t.Fatalf("字幕状态应独立更新，got %q", d.Lines()[13])
	}
}

func TestBadXDSChecksumIsRecorded(t *testing.T) {
	raw := buildXDS(t, 0x05, 0x01, "RATE", false)
	raw[len(raw)-1] ^= 0x01
	result := Parse(raw)
	if len(result.XDSPackets) != 1 {
		t.Fatalf("坏校验包仍应保留供诊断，got %d", len(result.XDSPackets))
	}
	if result.XDSPackets[0].ChecksumOK {
		t.Fatal("翻转校验位后 ChecksumOK 应为 false")
	}
	if result.XDS.BadChecksums != 1 {
		t.Fatalf("应记录一次 XDS 校验错误，got %d", result.XDS.BadChecksums)
	}
}

func TestTruncatedXDSRecoversAtCaptionControl(t *testing.T) {
	d := NewWithOptions(Options{Parity: ParityOff})
	full := buildXDS(t, 0x06, 0x01, "INFO", false)
	d.Decode(full[:len(full)-2]) // lose 0x0F/checksum ending.
	if !d.XDS.Active {
		t.Fatal("缺少结束对时 XDS 应保持 active")
	}

	d.Decode(captionBytes("BACK"))
	if d.XDS.Active {
		t.Fatal("caption 控制前导码应中止残缺 XDS 状态")
	}
	if d.XDS.Truncated != 1 {
		t.Fatalf("应记录一个截断 XDS 包，got %d", d.XDS.Truncated)
	}
	if got := d.Lines()[13]; got != "BACK" {
		t.Fatalf("XDS 截断后应恢复字幕，got %q", got)
	}
}
