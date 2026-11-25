package bwf

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func (f *File) ParseIXML() (*IXML, error) {
	ch := f.Chunk("iXML")
	if ch == nil {
		return nil, fmt.Errorf("missing iXML chunk")
	}
	raw, err := f.PayloadFull(ch)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimSpace(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}))
	raw = bytes.TrimRight(raw, "\x00")
	if len(raw) == 0 {
		return nil, fmt.Errorf("iXML chunk is empty")
	}
	x, err := parseIXML(raw)
	if err != nil {
		return nil, err
	}
	x.PayloadSize = ch.Size
	return x, nil
}

func parseIXML(raw []byte) (*IXML, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	x := &IXML{}
	var stack []string
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("parse iXML XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) == 0 {
				x.Root = t.Name.Local
			}
			stack = append(stack, t.Name.Local)
			text.Reset()
		case xml.CharData:
			if len(stack) > 0 {
				text.Write(t)
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("iXML has an unexpected closing element")
			}
			name := stack[len(stack)-1]
			path := strings.Join(stack, "/")
			value := strings.TrimSpace(text.String())
			if value != "" {
				x.Fields = append(x.Fields, IXMLField{Path: path, Name: name, Value: value})
				x.setKnown(name, value)
			}
			stack = stack[:len(stack)-1]
			text.Reset()
		case xml.ProcInst, xml.Comment, xml.Directive:
		}
	}
	if x.Root == "" {
		return nil, fmt.Errorf("iXML has no root element")
	}
	if x.StartTime == nil {
		if v, ok := x.firstValue("START_TIME", "START_TIMECODE"); ok {
			if tc, err := ParseTimecode(v, x.TimecodeRate); err == nil {
				x.StartTime = &tc
			}
		}
	}
	if x.StartTime != nil && strings.EqualFold(strings.TrimSpace(x.TimecodeFlag), "DF") {
		x.StartTime.DropFrame = true
	}
	return x, nil
}

func (x *IXML) setKnown(name, value string) {
	key := strings.ToUpper(strings.TrimSpace(name))
	switch key {
	case "PROJECT":
		if x.Project == "" {
			x.Project = value
		}
	case "TAPE":
		if x.Tape == "" {
			x.Tape = value
		}
	case "SCENE":
		if x.Scene == "" {
			x.Scene = value
		}
	case "TAKE":
		if x.Take == "" {
			x.Take = value
		}
	case "NOTE":
		if x.Note == "" {
			x.Note = value
		}
	case "TIMECODE_RATE":
		if x.TimecodeRate == "" {
			x.TimecodeRate = value
		}
	case "TIMECODE_FLAG":
		if x.TimecodeFlag == "" {
			x.TimecodeFlag = value
		}
	case "TIMECODE_SAMPLE_RATE":
		if x.TimecodeSampleRate == 0 {
			if n, err := strconv.ParseUint(value, 10, 32); err == nil {
				x.TimecodeSampleRate = uint32(n)
			}
		}
	case "START_TIME", "START_TIMECODE":
		if x.StartTime == nil {
			if tc, err := ParseTimecode(value, x.TimecodeRate); err == nil {
				x.StartTime = &tc
			}
		}
	case "BWF_TIME_REFERENCE", "BWF_TIMEREFERENCE", "TIME_REFERENCE", "TIMEREFERENCE":
		if x.DirectTimeReference == nil {
			if n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64); err == nil {
				x.DirectTimeReference = &n
			}
		}
	}
}

func (x *IXML) Values(name string) []string {
	key := strings.ToUpper(name)
	var out []string
	for _, f := range x.Fields {
		if strings.ToUpper(f.Name) == key {
			out = append(out, f.Value)
		}
	}
	return out
}

func (x *IXML) firstValue(names ...string) (string, bool) {
	for _, name := range names {
		for _, f := range x.Fields {
			if strings.EqualFold(f.Name, name) {
				return f.Value, true
			}
		}
	}
	return "", false
}
