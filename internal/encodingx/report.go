package encodingx

import "fmt"

// Notes returns human-readable encoding findings, including a baseline note
// for clean UTF-8 so reports always state what was checked.
func (r *Result) Notes() []string {
	out := append([]string(nil), r.NotesList...)
	switch r.Encoding {
	case KindUTF8:
		out = append(out, "source encoding: valid UTF-8 without BOM")
	case KindUTF8BOM:
		out = append(out, "source encoding: UTF-8 with BOM; BOM must be stripped before rendering")
	case KindUTF16LEBOM, KindUTF16BEBOM, KindUTF16LENOBM, KindUTF16BENOBM:
		out = append(out, "source encoding: UTF-16; a UTF-8 render pipeline will misread this as NUL-separated garbage")
	case KindUTF32LEBOM, KindUTF32BEBOM:
		out = append(out, "source encoding: UTF-32; not accepted by most subtitle renderers")
	case KindGB18030:
		// Already noted during decode.
	case KindUnknown:
		out = append(out, "source encoding could not be identified")
	}
	if r.HasReplacement {
		out = append(out, fmt.Sprintf("contains %d U+FFFD replacement rune(s): byte-level data loss already happened before font lookup", r.ReplacementCount))
	}
	if r.NulCount > 0 {
		out = append(out, fmt.Sprintf("contains %d embedded NUL byte(s); libass/ffmpeg subtitle demuxers may truncate the cue here", r.NulCount))
	}
	return out
}
