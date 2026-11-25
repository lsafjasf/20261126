package channelmap

import (
	"fmt"
	"sort"
	"strings"
)

// Microsoft/WAVEFORMATEXTENSIBLE speaker-position bits.
const (
	SpeakerFrontLeft          uint32 = 0x00000001
	SpeakerFrontRight         uint32 = 0x00000002
	SpeakerFrontCenter        uint32 = 0x00000004
	SpeakerLowFrequency       uint32 = 0x00000008
	SpeakerBackLeft           uint32 = 0x00000010
	SpeakerBackRight          uint32 = 0x00000020
	SpeakerFrontLeftOfCenter  uint32 = 0x00000040
	SpeakerFrontRightOfCenter uint32 = 0x00000080
	SpeakerBackCenter         uint32 = 0x00000100
	SpeakerSideLeft           uint32 = 0x00000200
	SpeakerSideRight          uint32 = 0x00000400
	SpeakerTopCenter          uint32 = 0x00000800
	SpeakerTopFrontLeft       uint32 = 0x00001000
	SpeakerTopFrontCenter     uint32 = 0x00002000
	SpeakerTopFrontRight      uint32 = 0x00004000
	SpeakerTopBackLeft        uint32 = 0x00008000
	SpeakerTopBackCenter      uint32 = 0x00010000
	SpeakerTopBackRight       uint32 = 0x00020000
)

const (
	Mask5Point1     uint32 = 0x0000003F // FL, FR, FC, LFE, BL, BR
	Mask5Point1Side uint32 = 0x0000060F // FL, FR, FC, LFE, SL, SR
	MaskStereo      uint32 = 0x00000003
	MaskMono        uint32 = 0x00000004
	Mask7Point1     uint32 = 0x0000063F // FL, FR, FC, LFE, BL, BR, SL, SR
	Mask7Point1Wide uint32 = 0x000000FF // includes FLC/FRC instead of sides
)

// Speaker describes one bit in dwChannelMask and its stable report label.
type Speaker struct {
	Bit         uint32 `json:"bit"`
	BitIndex    int    `json:"bit_index"`
	Role        string `json:"role"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

var speakerTable = []Speaker{
	{SpeakerFrontLeft, 0, "FL", "front-left", "左前置"},
	{SpeakerFrontRight, 1, "FR", "front-right", "右前置"},
	{SpeakerFrontCenter, 2, "FC", "front-center", "中置"},
	{SpeakerLowFrequency, 3, "LFE", "low-frequency", "低频效果"},
	{SpeakerBackLeft, 4, "BL", "back-left", "左环绕/后左"},
	{SpeakerBackRight, 5, "BR", "back-right", "右环绕/后右"},
	{SpeakerFrontLeftOfCenter, 6, "FLC", "front-left-of-center", "左中前置"},
	{SpeakerFrontRightOfCenter, 7, "FRC", "front-right-of-center", "右中前置"},
	{SpeakerBackCenter, 8, "BC", "back-center", "后中置"},
	{SpeakerSideLeft, 9, "SL", "side-left", "左侧环绕"},
	{SpeakerSideRight, 10, "SR", "side-right", "右侧环绕"},
	{SpeakerTopCenter, 11, "TC", "top-center", "顶置中"},
	{SpeakerTopFrontLeft, 12, "TFL", "top-front-left", "顶置左前"},
	{SpeakerTopFrontCenter, 13, "TFC", "top-front-center", "顶置中前"},
	{SpeakerTopFrontRight, 14, "TFR", "top-front-right", "顶置右前"},
	{SpeakerTopBackLeft, 15, "TBL", "top-back-left", "顶置左后"},
	{SpeakerTopBackCenter, 16, "TBC", "top-back-center", "顶置后中"},
	{SpeakerTopBackRight, 17, "TBR", "top-back-right", "顶置右后"},
}

var speakerByBit = func() map[uint32]Speaker {
	out := make(map[uint32]Speaker, len(speakerTable))
	for _, sp := range speakerTable {
		out[sp.Bit] = sp
	}
	return out
}()

// MaskRoles returns roles in WAV interleaved sample order. WAV orders
// channels by increasing dwChannelMask bit, not by the numerical mask value.
func MaskRoles(mask uint32) []Speaker {
	var out []Speaker
	for bit := 0; bit < 32; bit++ {
		b := uint32(1) << bit
		if mask&b == 0 {
			continue
		}
		if sp, ok := speakerByBit[b]; ok {
			out = append(out, sp)
		} else {
			out = append(out, Speaker{Bit: b, BitIndex: bit, Role: fmt.Sprintf("UNKNOWN_BIT_%d", bit), Label: "unknown", Description: "未定义声道掩码位"})
		}
	}
	return out
}

// MaskPopCount counts set bits without relying on bits.OnesCount32 at call
// sites, keeping mask semantics explicit.
func MaskPopCount(mask uint32) int {
	n := 0
	for bit := 0; bit < 32; bit++ {
		if mask&(uint32(1)<<bit) != 0 {
			n++
		}
	}
	return n
}

// DefaultMaskForChannels is only a low-confidence fallback for plain PCM WAV
// files that cannot carry dwChannelMask.
func DefaultMaskForChannels(channels int) (uint32, bool) {
	switch channels {
	case 1:
		return MaskMono, true
	case 2:
		return MaskStereo, true
	case 6:
		return Mask5Point1, true
	case 8:
		return Mask7Point1, true
	default:
		return 0, false
	}
}

// IsSurroundRole reports whether a role is commonly used as a surround channel.
func IsSurroundRole(role string) bool {
	switch strings.ToUpper(role) {
	case "BL", "BR", "SL", "SR", "BC", "TBL", "TBR":
		return true
	default:
		return false
	}
}

// IsCenterRole reports center-like roles.
func IsCenterRole(role string) bool {
	switch strings.ToUpper(role) {
	case "FC", "BC", "TFC", "TC", "TBC":
		return true
	default:
		return false
	}
}

func sortedRoles(roles []string) []string {
	out := append([]string(nil), roles...)
	sort.Strings(out)
	return out
}
