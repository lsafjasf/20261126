package edl

import (
	"fmt"
	"io"
	"strings"
)

// RenderText writes the three-bucket report in a reviewer-friendly form.
func RenderText(w io.Writer, rep Report) {
	fmt.Fprintf(w, "EDL relocation report — project %s (online %s)\n",
		rep.ProjectID, rep.OnlineVersion)
	fmt.Fprintf(w, "summary: %d applied, %d pending review, %d dead\n\n",
		rep.AppliedCount, rep.PendingCount, rep.DeadCount)

	renderBucket(w, "APPLIED", rep.Applied)
	renderBucket(w, "PENDING REVIEW", rep.Pending)
	renderBucket(w, "DEAD", rep.Dead)
}

func renderBucket(w io.Writer, title string, rs []Resolution) {
	fmt.Fprintf(w, "== %s (%d) ==\n", title, len(rs))
	if len(rs) == 0 {
		fmt.Fprintln(w, "  (none)")
		fmt.Fprintln(w)
		return
	}
	for _, r := range rs {
		op := r.Offline
		fmt.Fprintf(w, "  seq %d [%s] %s\n", r.Seq, r.Action, r.Reason)
		fmt.Fprintf(w, "    offline: clip_id=%s hash=%s path=%s\n",
			op.OldClipID, shortHash(op.MediaHash), op.OldPath)
		if r.ResolvedTo != nil {
			fmt.Fprintf(w, "    online : clip_id=%s hash=%s path=%s\n",
				r.ResolvedTo.ClipID, shortHash(r.ResolvedTo.MediaHash),
				r.ResolvedTo.Path)
		}
		if len(r.Candidates) > 0 {
			fmt.Fprintf(w, "    candidates:\n")
			for _, c := range r.Candidates {
				dims := matchDims(c)
				fmt.Fprintf(w, "      - clip_id=%s hash=%s path=%s [%s]\n",
					c.ClipID, shortHash(c.MediaHash), c.Path, strings.Join(dims, ","))
			}
		}
		for _, warn := range r.Warnings {
			fmt.Fprintf(w, "    ! %s\n", warn)
		}
	}
	fmt.Fprintln(w)
}

func matchDims(c Candidate) []string {
	var dims []string
	if c.MatchID {
		dims = append(dims, "id")
	}
	if c.MatchHash {
		dims = append(dims, "hash")
	}
	if c.MatchName {
		dims = append(dims, "name")
	}
	if len(dims) == 0 {
		dims = append(dims, "none")
	}
	return dims
}
