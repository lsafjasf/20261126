package aiffmeta

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
)

// WriteInspection prints a human-readable chunk table and metadata report.
func WriteInspection(w io.Writer, f *File) error {
	if f == nil {
		return fmt.Errorf("nil file")
	}
	path := f.Path
	if path == "" {
		path = "<memory>"
	}
	fmt.Fprintf(w, "AIFF/AIFC metadata report\n")
	fmt.Fprintf(w, "path: %s\n", path)
	fmt.Fprintf(w, "size: %d bytes\n", f.Size)
	fmt.Fprintf(w, "form: %s declared-size=%d data-start=%d declared-end=%d",
		f.Format, f.FormSize, f.FormDataStart, f.FormEnd)
	if f.FormTruncated {
		fmt.Fprint(w, " (truncated)")
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "FORM header: offset=0 data=8 size=%d padded-end=%d\n",
		f.FormSize, int64(8)+int64(f.FormSize))

	fmt.Fprintln(w, "\nCOMM:")
	if f.COMM == nil {
		fmt.Fprintln(w, "  missing or unparseable")
	} else {
		fmt.Fprintf(w, "  channels=%d frames=%d sample-size=%d sample-rate=%g",
			f.COMM.Channels, f.COMM.SampleFrames, f.COMM.SampleSize, f.COMM.SampleRate)
		if f.COMM.IsAIFC {
			fmt.Fprintf(w, " compression=%s", strconv.Quote(f.COMM.CompressionType))
			if f.COMM.CompressionName != "" {
				fmt.Fprintf(w, " compression-name=%s", strconv.Quote(f.COMM.CompressionName))
			}
		}
		fmt.Fprintf(w, " extended-rate=%s\n", f.COMM.ExtendedRateHex)
	}

	fmt.Fprintln(w, "\nSSND:")
	if f.SSND == nil {
		fmt.Fprintln(w, "  missing or unparseable")
	} else {
		fmt.Fprintf(w, "  offset=%d block-size=%d audio-offset=%d audio-size=%d\n",
			f.SSND.Offset, f.SSND.BlockSize, f.SSND.AudioOffset, f.SSND.AudioSize)
	}

	fmt.Fprintln(w, "\nChunk offset table (absolute bytes; size excludes header and padding):")
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "idx\tID\tkind\theader\tdata\tsize\tstored\tdata-end\tpadded-end\tnext")
	for _, chunk := range f.Chunks {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
			chunk.Index, printableID(chunk.ID), chunk.Kind, chunk.HeaderOffset,
			chunk.DataOffset, chunk.Size, chunk.StoredSize, chunk.DataEnd,
			chunk.PaddedEnd, chunk.NextOffset)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	fmt.Fprintln(w, "\nText metadata:")
	writeTextMetadata(w, f.Text)
	fmt.Fprintln(w, "\nWarnings:")
	writeIssues(w, f.Warnings)
	return nil
}

func writeTextMetadata(w io.Writer, text TextMetadata) {
	anyValue := false
	if text.Name != "" {
		fmt.Fprintf(w, "  NAME: %s\n", strconv.Quote(text.Name))
		anyValue = true
	}
	if text.Author != "" {
		fmt.Fprintf(w, "  AUTH: %s\n", strconv.Quote(text.Author))
		anyValue = true
	}
	if text.Copyright != "" {
		fmt.Fprintf(w, "  (c) : %s\n", strconv.Quote(text.Copyright))
		anyValue = true
	}
	for i, annotation := range text.Annotations {
		fmt.Fprintf(w, "  ANNO[%d]: %s\n", i, strconv.Quote(annotation))
		anyValue = true
	}
	for i, comment := range text.Comments {
		fmt.Fprintf(w, "  COMT[%d]: timestamp=%d (%s) marker=%d text=%s\n",
			i, comment.TimeStamp, comment.Time().Format("2006-01-02T15:04:05Z"),
			comment.MarkerID, strconv.Quote(comment.Text))
		anyValue = true
	}
	for i, marker := range text.Markers {
		fmt.Fprintf(w, "  MARK[%d]: id=%d position=%d name=%s\n",
			i, marker.ID, marker.Position, strconv.Quote(marker.Name))
		anyValue = true
	}
	if !anyValue {
		fmt.Fprintln(w, "  none")
	}
}

func writeIssues(w io.Writer, issues []Issue) {
	if len(issues) == 0 {
		fmt.Fprintln(w, "  none")
		return
	}
	for _, issue := range issues {
		where := ""
		if issue.Chunk != "" {
			where = issue.Chunk + " "
		}
		fmt.Fprintf(w, "  %s at %d: %s%s\n", issue.Severity, issue.Offset, where, issue.Message)
	}
}

// WriteRepackReport prints what a rewrite kept or dropped.
func WriteRepackReport(w io.Writer, report *RepackReport) error {
	if report == nil {
		return fmt.Errorf("nil report")
	}
	fmt.Fprintf(w, "repack: %d -> %d bytes, kept=%d dropped=%d audio=%d bytes\n",
		report.InputBytes, report.OutputBytes, report.KeptChunks,
		len(report.DroppedChunks), report.AudioBytes)
	if report.AudioSHA256 != "" {
		fmt.Fprintf(w, "audio sha256: %s\n", report.AudioSHA256)
	}
	if len(report.DroppedChunks) > 0 {
		fmt.Fprintln(w, "dropped chunks:")
		for _, chunk := range report.DroppedChunks {
			fmt.Fprintf(w, "  #%d %s offset=%d size=%d\n",
				chunk.Index, printableID(chunk.ID), chunk.HeaderOffset, chunk.Size)
		}
	}
	writeLost(w, report.Lost)
	if report.TrailingBytesOmitted > 0 {
		fmt.Fprintf(w, "trailing bytes omitted: %d\n", report.TrailingBytesOmitted)
	}
	if len(report.Warnings) > 0 {
		fmt.Fprintln(w, "source warnings:")
		writeIssues(w, report.Warnings)
	}
	return nil
}

// WriteComparison prints audio equality followed by every lost metadata item.
func WriteComparison(w io.Writer, report *CompareReport) error {
	if report == nil {
		return fmt.Errorf("nil report")
	}
	if report.FormatChanged {
		fmt.Fprintln(w, "format: changed")
	} else {
		fmt.Fprintln(w, "format: unchanged")
	}
	if !report.Audio.Comparable {
		fmt.Fprintf(w, "audio: unavailable (%s)\n", report.Audio.Note)
	} else if report.Audio.Equal {
		fmt.Fprintf(w, "audio: identical (%d bytes, sha256=%s)\n",
			report.Audio.SourceBytes, report.Audio.SourceSHA256)
	} else {
		fmt.Fprintf(w, "audio: CHANGED (source=%d bytes sha256=%s, result=%d bytes sha256=%s)\n",
			report.Audio.SourceBytes, report.Audio.SourceSHA256,
			report.Audio.ResultBytes, report.Audio.ResultSHA256)
	}
	if report.COMM.Changed {
		fmt.Fprintln(w, "COMM: changed")
		for _, difference := range report.COMM.Differences {
			fmt.Fprintf(w, "  %s\n", difference)
		}
	} else if report.COMM.Comparable {
		fmt.Fprintln(w, "COMM: unchanged")
	}
	if !report.SSND.SourcePresent && !report.SSND.ResultPresent {
		fmt.Fprintln(w, "SSND: absent in both files")
	} else if !report.SSND.Comparable {
		fmt.Fprintln(w, "SSND: missing in one file")
	} else if report.SSND.OffsetChanged || report.SSND.BlockSizeChanged {
		fmt.Fprintf(w, "ssnd parameters: offset-changed=%t block-size-changed=%t\n",
			report.SSND.OffsetChanged, report.SSND.BlockSizeChanged)
	}
	if len(report.Metadata.MissingChunks) > 0 {
		fmt.Fprintln(w, "missing chunks:")
		for _, chunk := range report.Metadata.MissingChunks {
			fmt.Fprintf(w, "  %s offset=%d size=%d kind=%s\n",
				printableID(chunk.ID), chunk.HeaderOffset, chunk.Size, chunk.Kind)
		}
	}
	if len(report.Metadata.AddedChunks) > 0 {
		fmt.Fprintln(w, "added chunks:")
		for _, chunk := range report.Metadata.AddedChunks {
			fmt.Fprintf(w, "  %s offset=%d size=%d kind=%s\n",
				printableID(chunk.ID), chunk.HeaderOffset, chunk.Size, chunk.Kind)
		}
	}
	writeLost(w, report.Metadata.Lost)
	return nil
}

func writeLost(w io.Writer, lost []LostItem) {
	if len(lost) == 0 {
		fmt.Fprintln(w, "lost metadata: none")
		return
	}
	fmt.Fprintln(w, "lost metadata:")
	for _, item := range lost {
		switch item.Kind {
		case "comment":
			fmt.Fprintf(w, "  comment chunk=%s timestamp=%d marker=%d text=%s\n",
				item.ChunkID, item.TimeStamp, item.MarkerID, strconv.Quote(item.Text))
		case "marker":
			fmt.Fprintf(w, "  marker chunk=%s id=%d position=%d name=%s\n",
				item.ChunkID, item.MarkerID, item.Position, strconv.Quote(item.Text))
		default:
			fmt.Fprintf(w, "  %s chunk=%s text=%s\n",
				item.Kind, item.ChunkID, strconv.Quote(item.Text))
		}
	}
}

// WriteJSON writes an indented JSON representation with a trailing newline.
func WriteJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func printableID(id string) string {
	if len(id) == 0 {
		return "<empty>"
	}
	for _, r := range id {
		if r < 0x20 || r > 0x7e {
			return strconv.Quote(id)
		}
	}
	return strings.ReplaceAll(id, "\t", "\\t")
}
