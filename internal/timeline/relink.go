package timeline

import (
	"encoding/json"
	"fmt"
	"os"

	"proxyrelink/internal/match"
	"proxyrelink/internal/media"
)

// Cut is one clip segment on the editing timeline. InFrame/OutFrame are
// frame indices into the source media; OutFrame is exclusive.
type Cut struct {
	Seq      int    `json:"seq"`
	ClipID   string `json:"clip_id"` // media ID currently referenced (usually a proxy)
	InFrame  int64  `json:"in_frame"`
	OutFrame int64  `json:"out_frame"`
}

// Timeline is an edit decision list expressed in frames.
type Timeline struct {
	Name   string `json:"name"`
	FPSNum int    `json:"fps_num"`
	FPSDen int    `json:"fps_den"`
	Cuts   []Cut  `json:"cuts"`
}

// LoadTimeline reads a timeline JSON file.
func LoadTimeline(path string) (Timeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Timeline{}, fmt.Errorf("read timeline: %w", err)
	}
	var tl Timeline
	if err := json.Unmarshal(data, &tl); err != nil {
		return Timeline{}, fmt.Errorf("parse timeline: %w", err)
	}
	return tl, nil
}

// CutStatus describes how one cut was (or was not) relinked.
type CutStatus struct {
	Cut
	Action  string `json:"action"` // "relinked" | "kept" | "error"
	NewClip string `json:"new_clip,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// RelinkReport is the result of replacing locked proxies with originals.
// Cut points (InFrame/OutFrame) are copied verbatim and never modified.
type RelinkReport struct {
	Timeline  string      `json:"timeline"`
	FPSNum    int         `json:"fps_num"`
	FPSDen    int         `json:"fps_den"`
	Cuts      []CutStatus `json:"cuts"`
	Relinked  int         `json:"relinked"`
	Unchanged int         `json:"unchanged"`
	Errors    int         `json:"errors"`
}

// Relink replaces every timeline cut that references a locked proxy with
// its locked original, after verifying the swap cannot move any cut point:
// same frame rate, enough frames in the original, and equal frame counts
// (tolerance 0 by default). On any failed check the cut keeps its proxy.
func Relink(tl Timeline, ix media.Index, dec *match.Decisions, frameTolerance int64) RelinkReport {
	rep := RelinkReport{Timeline: tl.Name, FPSNum: tl.FPSNum, FPSDen: tl.FPSDen}
	locked := map[string]string{} // proxyID -> originalID
	for _, d := range dec.LockedPairs() {
		locked[d.ProxyID] = d.OriginalID
	}
	for _, cut := range tl.Cuts {
		st := CutStatus{Cut: cut, Action: "kept"}
		origID, isLocked := locked[cut.ClipID]
		if !isLocked {
			st.Reason = "clip is not a locked proxy"
			rep.Cuts = append(rep.Cuts, st)
			rep.Unchanged++
			continue
		}
		proxy, okP := ix.ByID(cut.ClipID)
		orig, okO := ix.ByID(origID)
		if !okP || !okO {
			st.Action, st.Reason = "error", "locked pair references unknown media"
			rep.Cuts = append(rep.Cuts, st)
			rep.Errors++
			continue
		}
		if reason := verifySwap(tl, proxy, orig, cut, frameTolerance); reason != "" {
			st.Action, st.Reason = "error", reason
			rep.Cuts = append(rep.Cuts, st)
			rep.Errors++
			continue
		}
		st.Action = "relinked"
		st.NewClip = orig.ID
		rep.Cuts = append(rep.Cuts, st)
		rep.Relinked++
	}
	return rep
}

// verifySwap returns "" when replacing proxy with orig leaves every cut
// point valid, otherwise a human-readable reason.
func verifySwap(tl Timeline, proxy, orig media.MediaInfo, cut Cut, tol int64) string {
	if tl.FPSNum > 0 && tl.FPSDen > 0 && orig.FPSNum > 0 && orig.FPSDen > 0 {
		tlFPS := float64(tl.FPSNum) / float64(tl.FPSDen)
		if abs(tlFPS-orig.FPS()) > 0.001 {
			return fmt.Sprintf("frame rate mismatch: timeline %.3f vs original %.3f", tlFPS, orig.FPS())
		}
	}
	if proxy.FrameCount > 0 && orig.FrameCount > 0 {
		if diff := abs64(proxy.FrameCount - orig.FrameCount); diff > tol {
			return fmt.Sprintf("frame count differs by %d (tolerance %d): cut points would drift", diff, tol)
		}
	}
	if orig.FrameCount > 0 && cut.OutFrame > orig.FrameCount {
		return fmt.Sprintf("cut out-point %d exceeds original frame count %d", cut.OutFrame, orig.FrameCount)
	}
	if cut.InFrame < 0 || cut.OutFrame <= cut.InFrame {
		return fmt.Sprintf("invalid cut range [%d,%d)", cut.InFrame, cut.OutFrame)
	}
	return ""
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
