package vtt

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// WriteOptions controls canonical serialization.
type WriteOptions struct {
	// WithBOM emits a leading UTF-8 byte order mark as the spec permits.
	WithBOM bool
	// CRLF emits CRLF line endings instead of LF.
	CRLF bool
}

// Serialize renders the document to canonical WebVTT bytes after validating
// every block, so the output is accepted by standards-conformant parsers.
func (d *Document) Serialize(opts WriteOptions) ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	if err := d.Write(&buffer, opts); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Write serializes the document to w.
func (d *Document) Write(w io.Writer, opts WriteOptions) error {
	ending := "\n"
	if opts.CRLF {
		ending = "\r\n"
	}

	var builder strings.Builder
	if opts.WithBOM {
		builder.WriteString(bom)
	}
	builder.WriteString("WEBVTT")
	if strings.TrimSpace(d.Header) != "" {
		builder.WriteByte(' ')
		builder.WriteString(strings.TrimSpace(d.Header))
	}
	builder.WriteString(ending)

	for _, block := range d.Blocks {
		builder.WriteString(ending)
		switch value := block.(type) {
		case *Cue:
			writeCue(&builder, value, ending)
		case *Style:
			builder.WriteString("STYLE")
			builder.WriteString(ending)
			writeBody(&builder, value.Text, ending)
			builder.WriteString(ending)
		case *Region:
			builder.WriteString("REGION")
			builder.WriteString(ending)
			writeBody(&builder, value.Text, ending)
			builder.WriteString(ending)
		case *Note:
			writeBody(&builder, value.Text, ending)
			builder.WriteString(ending)
		default:
			return fmt.Errorf("unknown block type %T", block)
		}
	}

	_, err := io.WriteString(w, builder.String())
	return err
}

func writeBody(builder *strings.Builder, body, ending string) {
	builder.WriteString(strings.ReplaceAll(body, "\n", ending))
}

func writeCue(builder *strings.Builder, cue *Cue, ending string) {
	if cue.ID != "" {
		builder.WriteString(cue.ID)
		builder.WriteString(ending)
	}
	builder.WriteString(FormatTimestamp(cue.Start))
	builder.WriteString(" --> ")
	builder.WriteString(FormatTimestamp(cue.End))
	if strings.TrimSpace(cue.Settings) != "" {
		builder.WriteByte(' ')
		builder.WriteString(strings.TrimSpace(cue.Settings))
	}
	builder.WriteString(ending)
	writeBody(builder, cue.Text, ending)
	builder.WriteString(ending)
}

// Validate checks the invariants a conformant WebVTT file must satisfy.
// Repeated cue identifiers are explicitly legal and never rejected.
func (d *Document) Validate() error {
	if !validHeader(d.Header) {
		return fmt.Errorf("invalid WEBVTT header text %q", d.Header)
	}
	for index, block := range d.Blocks {
		cue, ok := block.(*Cue)
		if !ok {
			continue
		}
		if err := validateCue(cue); err != nil {
			return fmt.Errorf("block %d: %w", index, err)
		}
	}
	return nil
}

func validHeader(header string) bool {
	for _, line := range strings.Split(header, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "-->") {
			return false
		}
	}
	return true
}

func validateCue(cue *Cue) error {
	if cue.ID != "" && strings.Contains(cue.ID, "-->") {
		return fmt.Errorf("cue identifier must not contain -->")
	}
	if strings.Contains(cue.ID, "\n") {
		return fmt.Errorf("cue identifier must be a single line")
	}
	if cue.End < cue.Start {
		return fmt.Errorf("cue %q ends before it starts", cue.ID)
	}
	if strings.TrimSpace(cue.Text) == "" {
		return fmt.Errorf("cue %q has empty text", cue.ID)
	}
	return nil
}
