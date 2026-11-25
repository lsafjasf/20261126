package trackselect

// MediaKind 区分字幕轨与音频轨。
type MediaKind int

const (
	KindSubtitle MediaKind = iota
	KindAudio
)

func (k MediaKind) String() string {
	if k == KindAudio {
		return "audio"
	}
	return "subtitle"
}

// TrackType 描述轨道的呈现类型。
type TrackType int

const (
	TypeRegular    TrackType = iota // 普通字幕 / 主音轨
	TypeSDH                         // 听障字幕（含音效描述）
	TypeForced                      // 强制叙述字幕（外语对白必显）
	TypeCommentary                  // 解说轨 / 口述影像
)

func (t TrackType) String() string {
	switch t {
	case TypeSDH:
		return "sdh"
	case TypeForced:
		return "forced"
	case TypeCommentary:
		return "commentary"
	default:
		return "regular"
	}
}

// Track 表示一条字幕轨或音频轨。
type Track struct {
	ID       string
	Kind     MediaKind
	Type     TrackType
	Language string // BCP-47，如 "zh-CN"、"en"
	Label    string
	Default  bool   // 容器标记的默认轨
	Fallback string // 本轨不可用时回退到的轨道 ID，可为空
}

// Scenario 是用户当前观看场景，决定偏好如何加权。
type Scenario string

const (
	ScenarioDefault  Scenario = "default"  // 常规观影
	ScenarioNoisy    Scenario = "noisy"    // 嘈杂环境，优先 SDH
	ScenarioImpaired Scenario = "impaired" // 视障辅助，优先解说轨
	ScenarioLanguage Scenario = "language" // 语言学习，优先原声+普通字幕
)

// Preferences 描述某一场景下的用户偏好。
type Preferences struct {
	Scenario Scenario
	// Languages 按优先级排列的字幕语言，如 ["zh-CN", "en"]。
	Languages []string
	// AudioLanguages 按优先级排列的音轨语言；为空时复用 Languages。
	AudioLanguages []string
	// PreferSDH 为真时同语言下 SDH 优先于普通字幕。
	PreferSDH bool
	// WantCommentary 为真时优先选择解说轨。
	WantCommentary bool
	// AllowForced 为真时允许自动叠加强制叙述轨。
	AllowForced bool
}

// DefaultPreferences 返回各场景的内置偏好，可被调用方覆盖。
func DefaultPreferences(s Scenario) Preferences {
	p := Preferences{Scenario: s, Languages: []string{"zh-CN", "en"}, AllowForced: true}
	switch s {
	case ScenarioNoisy:
		p.PreferSDH = true
	case ScenarioImpaired:
		p.WantCommentary = true
		p.PreferSDH = true
	case ScenarioLanguage:
		p.Languages = []string{"en", "zh-CN"}
	}
	return p
}
