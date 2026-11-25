// Package sdh splits Subtitles for the Deaf and Hard-of-hearing (SDH) into
// semantic layers (speaker labels, environmental sound descriptions, music
// cues, sung lyrics, voice-over and ordinary dialogue), resolves temporal
// overlaps into visual lanes, and projects the result back to a complete
// ordinary subtitle track when SDH is switched off.
package sdh

// Role is the semantic role of a piece of subtitle text.
type Role string

const (
	// RoleDialogue is ordinary spoken dialogue (普通对白).
	RoleDialogue Role = "dialogue"
	// RoleSpeaker is a speaker label such as "JOHN:" or "张三：" (说话人标签).
	RoleSpeaker Role = "speaker"
	// RoleSound is an environmental sound/action description such as
	// "[thunder rumbles]" or "【敲门声】" (环境声描述).
	RoleSound Role = "sound"
	// RoleMusic is an instrumental/background music description such as
	// "[upbeat jazz plays]" or "（背景音乐响起）".
	RoleMusic Role = "music"
	// RoleLyric is sung song lyrics, normally delimited by ♪/♫ (音乐歌词).
	RoleLyric Role = "lyric"
	// RoleVoiceOver is off-screen speech / narration such as "(V.O.)" or
	// "（画外音）" (画外音/旁白). The words are still spoken content.
	RoleVoiceOver Role = "voiceover"
)

// Anchor is the screen edge a layer is anchored to when laying out lanes.
type Anchor int

const (
	// AnchorBottom stacks lanes upward from the bottom edge.
	AnchorBottom Anchor = iota
	// AnchorTop stacks lanes downward from the top edge.
	AnchorTop
)

// roleMeta carries editor-facing defaults for a semantic role.
type roleMeta struct {
	order    int
	anchor   Anchor
	assLayer int
	// vocal reports whether the role is spoken/sung content that must
	// survive switching SDH off.
	vocal bool
	// zh is the Chinese display name used in reports.
	zh string
}

var roleMetadata = map[Role]roleMeta{
	RoleDialogue:  {order: 10, anchor: AnchorBottom, assLayer: 2, vocal: true, zh: "对白"},
	RoleVoiceOver: {order: 20, anchor: AnchorBottom, assLayer: 3, vocal: true, zh: "画外音"},
	RoleLyric:     {order: 30, anchor: AnchorBottom, assLayer: 2, vocal: true, zh: "歌词"},
	RoleSpeaker:   {order: 40, anchor: AnchorBottom, assLayer: 4, vocal: false, zh: "说话人"},
	RoleSound:     {order: 50, anchor: AnchorTop, assLayer: 1, vocal: false, zh: "环境声"},
	RoleMusic:     {order: 60, anchor: AnchorTop, assLayer: 1, vocal: false, zh: "音乐描述"},
}

// Roles returns the known semantic roles in stable editorial order.
func Roles() []Role {
	return []Role{
		RoleDialogue,
		RoleVoiceOver,
		RoleLyric,
		RoleSpeaker,
		RoleSound,
		RoleMusic,
	}
}

// Valid reports whether r is one of the known roles.
func (r Role) Valid() bool {
	_, ok := roleMetadata[r]
	return ok
}

// String returns the machine-readable role name.
func (r Role) String() string { return string(r) }

// Label returns the Chinese editorial display name for the role.
func (r Role) Label() string {
	if m, ok := roleMetadata[r]; ok {
		return m.zh
	}
	return string(r)
}

// Vocal reports whether text with this role is spoken/sung content that
// belongs to the ordinary (SDH-off) subtitle track.
func (r Role) Vocal() bool {
	if m, ok := roleMetadata[r]; ok {
		return m.vocal
	}
	return false
}

// Anchor returns the default screen anchor for the role.
func (r Role) Anchor() Anchor {
	if m, ok := roleMetadata[r]; ok {
		return m.anchor
	}
	return AnchorBottom
}
