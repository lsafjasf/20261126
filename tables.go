package ac3select

import "fmt"

var sampleRateTable = [4]int{48000, 44100, 32000, 0}

// frameSizeWords is A/52 frame size table indexed by frmsizecod and fscod.
// Values are 16-bit words; multiply by two to obtain bytes.
var frameSizeWords = [38][3]int{
	{64, 69, 96}, {64, 70, 96}, {80, 87, 120}, {80, 88, 120},
	{96, 104, 144}, {96, 105, 144}, {112, 121, 168}, {112, 122, 168},
	{128, 139, 192}, {128, 140, 192}, {160, 174, 240}, {160, 175, 240},
	{192, 208, 288}, {192, 209, 288}, {224, 243, 336}, {224, 244, 336},
	{256, 278, 384}, {256, 279, 384}, {320, 348, 480}, {320, 349, 480},
	{384, 417, 576}, {384, 418, 576}, {448, 487, 672}, {448, 488, 672},
	{512, 557, 768}, {512, 558, 768}, {640, 696, 960}, {640, 697, 960},
	{768, 835, 1152}, {768, 836, 1152}, {896, 975, 1344}, {896, 976, 1344},
	{1024, 1114, 1536}, {1024, 1115, 1536}, {1152, 1253, 1728}, {1152, 1254, 1728},
	{1280, 1393, 1920}, {1280, 1394, 1920},
}

var fullBandChannelCount = [8]int{2, 1, 2, 3, 3, 4, 4, 5}

var acmodNames = [8]string{
	"1+1", "1/0", "2/0", "3/0", "2/1", "3/1", "2/2", "3/2",
}

var acmodDescriptions = [8]string{
	"Dual mono (Ch1, Ch2)",
	"Mono (C)",
	"Stereo (L, R)",
	"3/0 (L, C, R)",
	"2/1 (L, R, S)",
	"3/1 (L, C, R, S)",
	"2/2 (L, R, Ls, Rs)",
	"3/2 (L, C, R, Ls, Rs)",
}

// ac3ChannelLabels gives full-band channel labels in chincpl bit order.
var ac3ChannelLabels = [8][]string{
	{"Ch1", "Ch2"},
	{"C"},
	{"L", "R"},
	{"L", "C", "R"},
	{"L", "R", "S"},
	{"L", "C", "R", "S"},
	{"L", "R", "Ls", "Rs"},
	{"L", "C", "R", "Ls", "Rs"},
}

var bsmodNames = [8]string{
	"complete_main",
	"music_and_effects",
	"visually_impaired",
	"hearing_impaired",
	"dialogue",
	"commentary",
	"emergency",
	"voice_over_or_karaoke",
}

var bsmodChineseLabels = [8]string{
	"主节目（完整主混音 CM）",
	"主节目（音乐与效果 ME）",
	"视障辅助解说（VI）",
	"听障辅助声道（HI）",
	"对白/注释轨（D）",
	"评论声（C）",
	"紧急广播（E）",
	"画外音/卡拉OK（VO/Karaoke）",
}

func (m BitstreamMode) String() string {
	if m <= 7 {
		return bsmodNames[m]
	}
	return fmt.Sprintf("reserved_%d", int(m))
}

// ChineseLabel returns a user-facing Chinese service description.
func (m BitstreamMode) ChineseLabel() string {
	if m <= 7 {
		return bsmodChineseLabels[m]
	}
	return fmt.Sprintf("保留 bsmod=%d", int(m))
}

func (m BitstreamMode) trackCategory(acmod AudioCodingMode, channelCount int) TrackSelection {
	selection := TrackSelection{Role: m.String(), Label: m.ChineseLabel()}
	switch m {
	case BSModeCompleteMain:
		selection.Category = "main"
		selection.IsMain = true
		selection.SelectedAs = "main"
		selection.Reason = "bsmod=0 explicitly identifies complete main audio"
	case BSModeMusicAndEffects:
		selection.Category = "main"
		selection.IsMain = true
		selection.SelectedAs = "main"
		selection.Reason = "bsmod=1 identifies a main music-and-effects service"
	case BSModeCommentary:
		selection.Category = "commentary"
		selection.IsCommentary = true
		selection.IsAnnotation = true
		selection.SelectedAs = "commentary"
		selection.Reason = "bsmod=5 explicitly identifies commentary"
	case BSModeVisuallyImpaired, BSModeHearingImpaired, BSModeDialogue:
		selection.Category = "annotation"
		selection.IsAnnotation = true
		selection.SelectedAs = "annotation"
		selection.Reason = "bsmod identifies an associated accessibility/dialogue service"
	case BSModeEmergency:
		selection.Category = "emergency"
		selection.IsAnnotation = true
		selection.Reason = "bsmod=6 identifies emergency audio"
	case BSModeVoiceOverOrKaraoke:
		if acmod >= ACMod2_0 && channelCount > 1 {
			selection.Category = "karaoke"
			selection.Role = "karaoke"
			selection.Label = bsmodChineseLabels[7]
			selection.Reason = "bsmod=7 with multiple channels is interpreted as karaoke mode"
		} else {
			selection.Category = "voice_over"
			selection.IsAnnotation = true
			selection.SelectedAs = "voice_over"
			selection.Reason = "bsmod=7 with mono service is interpreted as voice-over"
		}
	default:
		selection.Category = "unknown"
		selection.Reason = "reserved bitstream mode"
	}
	if acmod == ACModDualMono {
		selection.Reason += "; acmod=0 is dual mono: Ch1/Ch2 are independent programs and channel-level commentary assignment needs language/routing metadata"
	}
	return selection
}

func (m AudioCodingMode) String() string {
	if m <= 7 {
		return acmodNames[m]
	}
	return fmt.Sprintf("reserved_%d", int(m))
}

func channelLayout(acmod AudioCodingMode, lfe bool) string {
	if acmod > 7 {
		return "unknown"
	}
	layout := acmodDescriptions[acmod]
	if lfe {
		layout += " + LFE"
	}
	return layout
}

func layoutShort(acmod AudioCodingMode, fullBand int, lfe bool) string {
	if acmod == ACModDualMono {
		if lfe {
			return "1+1.1"
		}
		return "1+1"
	}
	lowFrequency := 0
	if lfe {
		lowFrequency = 1
	}
	return fmt.Sprintf("%d.%d", fullBand, lowFrequency)
}

func makeChannels(acmod AudioCodingMode, lfe bool, coupling Coupling) []Channel {
	if acmod > 7 {
		return nil
	}
	labels := ac3ChannelLabels[acmod]
	channels := make([]Channel, 0, len(labels)+1)
	coupled := make(map[string]int, len(coupling.CoupledChannels))
	for i, label := range coupling.CoupledChannels {
		coupled[label] = i + 1
	}
	for i, label := range labels {
		bit := i + 1
		_, inCpl := coupled[label]
		ch := Channel{Index: bit, Label: label, Name: label, Kind: "full_bandwidth", InCoupling: inCpl}
		if inCpl {
			b := bit
			ch.CouplingBit = &b
		}
		channels = append(channels, ch)
	}
	if lfe {
		channels = append(channels, Channel{
			Index: len(labels) + 1, Label: "LFE", Name: "Low-frequency effects", Kind: "lfe",
		})
	}
	return channels
}
