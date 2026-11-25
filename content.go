package ttmlevents

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var whitespaceRun = regexp.MustCompile(`\s+`)

func collectTextRuns(doc *Document, tl *Timeline, styles *styleResolver) []TextRun {
	root := contentRoot(doc)
	if root == nil {
		return nil
	}

	runs := make([]TextRun, 0)
	runNumber := 0
	var walk func(n *Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Kind == ElementNode && isSkippedContentElement(n) {
			return
		}

		if n.Kind == TextNode {
			parent := n.Parent
			text := normalizeTextNode(n.Text, nearestXMLSpace(n))
			runNumber++
			run := TextRun{
				ID:            runID(runNumber),
				Kind:          "text",
				Text:          text,
				NodePath:      n.Path,
				ElementPath:   elementPath(parent),
				BlockPath:     nearestBlockPath(n),
				Whitespace:    strings.TrimSpace(text) == "",
				TimingVisible: true,
			}
			applyRunTiming(&run, tl.Get(parent.DocOrder))
			if parent != nil {
				run.Style = sortedProperties(styles.baseProperties(parent))
			}
			runs = append(runs, run)
			return
		}

		if n.Kind == ElementNode && n.Is("br") {
			runNumber++
			run := TextRun{
				ID:            runID(runNumber),
				Kind:          "break",
				Text:          "\n",
				ElementPath:   n.Path,
				NodePath:      n.Path,
				BlockPath:     nearestBlockPath(n),
				TimingVisible: true,
			}
			applyRunTiming(&run, tl.Get(n.DocOrder))
			run.Style = sortedProperties(styles.baseProperties(n))
			runs = append(runs, run)
			return
		}

		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(root)
	return runs
}

func runID(n int) string {
	return "R" + strconv.Itoa(n)
}

func contentRoot(doc *Document) *Node {
	if doc == nil || doc.Root == nil {
		return nil
	}
	var body *Node
	walkNodes(doc.Root, func(n *Node) bool {
		if n.Is("body") {
			body = n
			return false
		}
		return true
	})
	if body != nil {
		return body
	}
	return doc.Root
}

func isSkippedContentElement(n *Node) bool {
	return n.Is("metadata", "head", "styling", "style", "layout", "animate", "set", "animation")
}

func normalizeTextNode(raw string, preserve bool) string {
	if preserve {
		return raw
	}
	return whitespaceRun.ReplaceAllString(raw, " ")
}

func nearestXMLSpace(n *Node) bool {
	for cur := n; cur != nil; cur = cur.Parent {
		if value, ok := cur.AttrValue("space"); ok {
			return strings.EqualFold(value, "preserve")
		}
	}
	return false
}

func elementPath(n *Node) string {
	if n == nil {
		return ""
	}
	return n.Path
}

func nearestBlockPath(n *Node) string {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Kind == ElementNode && cur.Is("p", "div", "region") {
			return cur.Path
		}
	}
	return ""
}

func applyRunTiming(run *TextRun, timing *NodeTiming) {
	if timing == nil {
		run.Begin = 0
		run.BeginSeconds = 0
		run.TimingVisible = true
		return
	}
	interval := timing.Effective
	run.Begin = interval.Begin
	run.BeginSeconds = durationSeconds(interval.Begin)
	run.End = cloneDurationPtr(interval.End)
	if interval.End != nil {
		seconds := durationSeconds(*interval.End)
		run.EndSeconds = &seconds
	}
	run.TimingVisible = timing.Visible && !interval.Empty()
}

func runActiveAt(run TextRun, at time.Duration) bool {
	if !run.TimingVisible {
		return false
	}
	if at < run.Begin {
		return false
	}
	return run.End == nil || at < *run.End
}
