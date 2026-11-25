package sdh

import (
	"strings"
	"unicode"
)

// descriptionWords are English word forms that reveal a bracket or label as
// a sound/action description rather than a person's name. Whole-word
// matching avoids false positives such as "Carl" containing "car".
var descriptionWords = map[string]bool{
	"thunder": true, "thunders": true, "thunderclap": true, "thunderclaps": true,
	"rain": true, "rains": true, "raining": true, "rainfall": true, "rainstorm": true,
	"wind": true, "winds": true, "windy": true, "howling": true, "howl": true, "howls": true,
	"door": true, "doors": true, "doorway": true, "doorbell": true,
	"footstep": true, "footsteps": true, "footfall": true, "footfalls": true,
	"step": true, "steps": true, "stepping": true,
	"knock": true, "knocks": true, "knocking": true, "knocked": true,
	"bell": true, "bells": true, "chime": true, "chimes": true,
	"ring": true, "rings": true, "ringing": true, "rang": true,
	"phone": true, "telephone": true,
	"gun": true, "guns": true, "gunshot": true, "gunshots": true,
	"shot": true, "shots": true, "firing": true,
	"explosion": true, "explosions": true, "blast": true, "blasts": true,
	"crash": true, "crashes": true, "crashing": true,
	"bang": true, "bangs": true, "banging": true,
	"slam": true, "slams": true, "slamming": true, "slammed": true,
	"creak": true, "creaks": true, "creaking": true, "creaked": true,
	"bark": true, "barks": true, "barking": true,
	"dog": true, "dogs": true, "puppy": true,
	"cat": true, "cats": true, "meow": true,
	"siren": true, "sirens": true,
	"horn": true, "horns": true, "honk": true, "honking": true,
	"engine": true, "engines": true, "motor": true,
	"car": true, "cars": true, "truck": true, "bus": true,
	"train": true, "trains": true, "rail": true,
	"plane": true, "planes": true, "airplane": true, "jet": true,
	"helicopter": true,
	"crowd":      true, "crowds": true,
	"cheer": true, "cheers": true, "cheering": true, "cheered": true,
	"applause": true, "clap": true, "claps": true, "clapping": true,
	"laugh": true, "laughs": true, "laughing": true, "laughter": true,
	"chuckle": true, "chuckles": true, "giggle": true, "giggles": true,
	"cough": true, "coughs": true, "coughing": true,
	"sigh": true, "sighs": true, "sighing": true,
	"gasp": true, "gasps": true, "gasping": true,
	"scream": true, "screams": true, "screaming": true, "screamed": true,
	"shout": true, "shouts": true, "shouting": true, "yell": true, "yells": true,
	"whisper": true, "whispers": true, "whispering": true,
	"cry": true, "cries": true, "crying": true,
	"weep": true, "weeps": true, "weeping": true,
	"sniff": true, "sniffs": true, "sniffling": true,
	"kiss": true, "kisses": true, "kissing": true,
	"punch": true, "punches": true, "punching": true,
	"break": true, "breaks": true, "breaking": true, "broken": true,
	"smash": true, "smashes": true, "shatter": true, "shatters": true,
	"glass": true,
	"fire":  true, "flames": true,
	"water": true, "splash": true, "splashes": true, "splashing": true,
	"clock": true, "tick": true, "ticks": true, "ticking": true, "tock": true,
	"keyboard": true, "typing": true, "types": true,
	"camera": true, "shutter": true, "flash": true,
	"noise": true, "noises": true, "sound": true, "sounds": true,
	"crackle": true, "crackles": true, "crackling": true,
	"static": true, "rustle": true, "rustles": true, "rustling": true,
	"roar": true, "roars": true, "roaring": true,
	"buzz": true, "buzzes": true, "buzzing": true,
	"hum": true, "hums": true, "humming": true,
	"beep": true, "beeps": true, "beeping": true,
	"echo": true, "echoes": true,
	"music": true, "song": true, "sings": true, "singing": true,
}

// lowercase particles allowed inside a Title-Case person/role name.
var nameParticles = map[string]bool{
	"de": true, "del": true, "der": true, "van": true, "von": true,
	"la": true, "le": true, "du": true, "di": true, "da": true,
	"the": true, "of": true, "on": true, "at": true, "and": true,
}

func latinWords(s string) []string {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			cur.WriteRune(unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()
	return words
}

// hasLatinDescriptionWord reports whether s contains a whole description
// word (inflected forms are enumerated in descriptionWords).
func hasLatinDescriptionWord(s string) bool {
	for _, w := range latinWords(s) {
		if descriptionWords[w] {
			return true
		}
	}
	return false
}

// hasCJKDescriptionMarker reports whether a normalized CJK string contains
// a sound/action marker.
func hasCJKDescriptionMarker(norm string) bool {
	for _, k := range soundKeywords {
		// soundKeywords mixes English and CJK entries; only use entries
		// containing Han runes here.
		if isCJKKeyword(k) && strings.Contains(norm, k) {
			return true
		}
	}
	return false
}

func isCJKKeyword(k string) bool {
	for _, r := range k {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

func countHan(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			n++
		}
	}
	return n
}

// looksLikeSpeakerName validates the text immediately before a colon as a
// speaker label.
func looksLikeSpeakerName(name string) bool {
	trimmed := strings.TrimSpace(name)
	runes := []rune(trimmed)
	if len(runes) == 0 || len(runes) > 40 {
		return false
	}
	if strings.ContainsAny(trimmed, "。！？!?；;…,，") {
		return false
	}

	han := countHan(trimmed)
	latin := len(latinWords(trimmed))

	if han > 0 && latin == 0 {
		if han < 2 || han > 12 {
			return false
		}
		for _, bad := range cjkLabelBlocklist {
			if strings.Contains(trimmed, bad) {
				return false
			}
		}
		for _, bad := range cjkNonSpeakerMarkers {
			if strings.Contains(trimmed, bad) {
				return false
			}
		}
		norm := normalizeLex(trimmed)
		if hasCJKDescriptionMarker(norm) {
			return false
		}
		return true
	}

	if latin == 0 {
		return false
	}

	// Uppercase SDH tag: "JOHN", "MARY ANN", "SPEAKER 1", "DR. BROWN".
	letters := 0
	lower := 0
	for _, r := range trimmed {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsLower(r) {
				lower++
			}
		}
	}
	if letters >= 2 && lower == 0 {
		if hasLatinDescriptionWord(trimmed) {
			return false
		}
		return true
	}

	// Title-Case name, allowing lowercase particles: "John Smith",
	// "Mary-Jane", "Dr. Brown", "Ann van Dijk".
	fields := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ' ' || r == '\t'
	})
	if len(fields) == 0 || len(fields) > 5 {
		return false
	}
	titleOK := true
	for _, f := range fields {
		core := strings.Trim(f, ".'’-")
		if core == "" {
			continue
		}
		if nameParticles[strings.ToLower(core)] {
			continue
		}
		first := []rune(core)[0]
		if !unicode.IsUpper(first) && !unicode.IsDigit(first) {
			titleOK = false
		}
	}
	if titleOK && !hasLatinDescriptionWord(trimmed) {
		return true
	}

	// Lowercase role/source noun phrase: "narrator", "radio", "the crowd".
	words := latinWords(trimmed)
	if len(words) >= 1 && len(words) <= 3 {
		allRole := true
		sawRole := false
		for _, w := range words {
			if w == "the" || w == "a" || w == "an" || nameParticles[w] {
				continue
			}
			if latinRoleNouns[w] {
				sawRole = true
				continue
			}
			allRole = false
		}
		if allRole && sawRole && !hasLatinDescriptionWord(trimmed) {
			return true
		}
	}
	return false
}

// looksLikeBracketSpeaker decides whether bracket content such as "[John]"
// or "（张三）" is a speaker label rather than a description.
func looksLikeBracketSpeaker(content string) bool {
	trimmed := strings.TrimSpace(content)
	runes := []rune(trimmed)
	if len(runes) == 0 || len(runes) > 24 {
		return false
	}
	if _, _, mod := parseModifier(trimmed); mod {
		return false
	}

	han := countHan(trimmed)
	latin := len(latinWords(trimmed))

	if han > 0 && latin == 0 {
		if han < 2 || han > 8 {
			return false
		}
		if strings.ContainsAny(trimmed, "，。！？、,.!?：:；;") {
			return false
		}
		norm := normalizeLex(trimmed)
		if hasCJKDescriptionMarker(norm) {
			return false
		}
		return true
	}

	if latin == 0 {
		return false
	}
	if latin > 4 {
		return false
	}
	if hasLatinDescriptionWord(trimmed) {
		return false
	}

	// Bare role nouns are accepted even in lowercase: "[narrator]".
	if latin == 1 && latinRoleNouns[latinWords(trimmed)[0]] {
		return true
	}

	// Otherwise every word must be name-shaped (initial uppercase,
	// uppercase acronym, digit) or a lowercase particle/source word.
	for _, w := range strings.Fields(trimmed) {
		core := strings.Trim(w, ".'’-,")
		if core == "" {
			continue
		}
		lower := strings.ToLower(core)
		if nameParticles[lower] || latinRoleNouns[lower] || sourceWords[lower] {
			continue
		}
		first := []rune(core)[0]
		if !unicode.IsUpper(first) && !unicode.IsDigit(first) {
			return false
		}
	}
	return true
}

// sourceWords are lowercase media/source nouns allowed inside a bracket
// source label such as "[TV news anchor]" or "[PA system]".
var sourceWords = map[string]bool{
	"news": true, "anchor": true, "newscaster": true, "pa": true,
	"system": true, "computer": true, "voice": true, "broadcast": true,
	"broadcasting": true, "station": true, "channel": true, "live": true,
	"recording": true, "recorded": true, "message": true, "announcement": true,
}

// narratorNames marks speaker names that denote off-screen narration.
var narratorNames = map[string]bool{
	"旁白": true, "解说": true, "解说员": true, "画外音": true,
	"narrator": true, "voiceover": true,
}

// isNarratorName reports whether name denotes a narrator / voice-over source.
func isNarratorName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if narratorNames[trimmed] {
		return true
	}
	return narratorNames[normalizeLex(trimmed)]
}

// cjkNonSpeakerMarkers are grammatical/time words showing that a Han run
// before a colon is a clause rather than a person's name.
var cjkNonSpeakerMarkers = []string{
	"如果", "因为", "所以", "但是", "可是", "不过", "然后", "虽然",
	"尽管", "即使", "只要", "只有", "无论", "不管", "今天", "明天",
	"昨天", "现在", "这里", "那里", "什么", "怎么", "为什么",
}

func hasWordRune(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
