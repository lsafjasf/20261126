package anchor

import (
	"strconv"
	"strings"
)

// PieceKey builds the durable identity of a split event fragment. The key
// contains only the event identity and its source-media interval — never a
// timeline timestamp — so it remains the same across re-cuts and across
// save/reload cycles. Fragment boundaries are derived from EDL cut points,
// which are themselves deterministic, so the same inputs always yield the
// same keys.
func PieceKey(eventID string, a Anchor) string {
	return eventID + "#" + a.MediaID + ":" +
		strconv.FormatInt(int64(a.Start), 10) + "-" +
		strconv.FormatInt(int64(a.End), 10)
}

// AnchorKey is the identity for a legacy event's manual anchor decision.
func AnchorKey(eventID string) string {
	return eventID
}

// parsePieceKey reverses PieceKey. The boolean is false if the key is not in
// the canonical piece-key shape.
func parsePieceKey(key string) (eventID string, a Anchor, ok bool) {
	hash := strings.Index(key, "#")
	if hash <= 0 || hash == len(key)-1 {
		return "", Anchor{}, false
	}
	eventID = key[:hash]
	rest := key[hash+1:]
	colon := strings.Index(rest, ":")
	if colon <= 0 || colon == len(rest)-1 {
		return "", Anchor{}, false
	}
	a.MediaID = rest[:colon]
	interval := rest[colon+1:]
	dash := strings.Index(interval, "-")
	if dash <= 0 || dash == len(interval)-1 {
		return "", Anchor{}, false
	}
	start, err1 := strconv.ParseInt(interval[:dash], 10, 64)
	end, err2 := strconv.ParseInt(interval[dash+1:], 10, 64)
	if err1 != nil || err2 != nil {
		return "", Anchor{}, false
	}
	a.Start = Ticks(start)
	a.End = Ticks(end)
	return eventID, a, true
}
