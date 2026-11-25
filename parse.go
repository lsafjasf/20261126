package vorbiscomment

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf8"
)

// ErrMissingVendorLength means there were not even four bytes available to
// start a Vorbis comment structure.
var ErrMissingVendorLength = errors.New("vorbis comment: missing vendor length")

// Parse leniently decodes a raw Vorbis comment body or a complete packet with
// the 0x03 "vorbis" prefix. Field-level defects are returned as diagnostics;
// they do not cause the whole document to be rejected.
func Parse(data []byte, opts Options) (*Comment, error) {
	stored := append([]byte(nil), data...)
	headerOffset := opts.BaseOffset
	hasMagic := bytes.HasPrefix(stored, packetMagic)
	if hasMagic {
		headerOffset += int64(len(packetMagic))
	}
	c := &Comment{
		HeaderOffset:   headerOffset,
		HeaderLength:   len(stored),
		HasPacketMagic: hasMagic,
		Complete:       true,
		raw:            stored,
	}

	body := stored
	bodyOffset := opts.BaseOffset
	if c.HasPacketMagic {
		body = stored[len(packetMagic):]
		bodyOffset += int64(len(packetMagic))
	}
	if len(body) < 4 {
		c.Complete = false
		c.Issues = append(c.Issues, Issue{
			Code:       CodeTruncatedVendor,
			Class:      ManualConfirmation,
			Message:    "fewer than four bytes remain for the vendor length",
			Offset:     bodyOffset,
			Length:     len(body),
			EntryIndex: -1,
		})
		return c, fmt.Errorf("%w: have %d bytes", ErrMissingVendorLength, len(body))
	}

	if err := parseBody(c, body, bodyOffset, opts); err != nil {
		return c, err
	}
	addDuplicateDiagnostics(c, opts)
	return c, nil
}

// ParseBody is Parse without automatic packet-magic detection.
func ParseBody(data []byte, opts Options) (*Comment, error) {
	stored := append([]byte(nil), data...)
	c := &Comment{
		HeaderOffset: opts.BaseOffset,
		HeaderLength: len(stored),
		Complete:     true,
		raw:          stored,
	}
	if len(stored) < 4 {
		c.Complete = false
		c.Issues = append(c.Issues, Issue{
			Code:       CodeTruncatedVendor,
			Class:      ManualConfirmation,
			Message:    "fewer than four bytes remain for the vendor length",
			Offset:     opts.BaseOffset,
			Length:     len(stored),
			EntryIndex: -1,
		})
		return c, fmt.Errorf("%w: have %d bytes", ErrMissingVendorLength, len(stored))
	}
	if err := parseBody(c, stored, opts.BaseOffset, opts); err != nil {
		return c, err
	}
	addDuplicateDiagnostics(c, opts)
	return c, nil
}

func parseBody(c *Comment, body []byte, base int64, opts Options) error {
	vendorLength := uint64(binary.LittleEndian.Uint32(body[:4]))
	vendorStart := 4
	off := vendorStart
	if vendorLength > uint64(len(body)-off) {
		c.Complete = false
		c.Vendor = append([]byte(nil), body[off:]...)
		c.VendorString = displayString(c.Vendor)
		c.Issues = append(c.Issues, Issue{
			Code:       CodeTruncatedVendor,
			Class:      ManualConfirmation,
			Message:    fmt.Sprintf("vendor declares %d bytes, only %d remain", vendorLength, len(body)-off),
			Offset:     base + int64(vendorStart),
			Length:     len(body) - off,
			EntryIndex: -1,
		})
		return nil
	}

	vendorEnd := off + int(vendorLength)
	c.Vendor = append([]byte(nil), body[off:vendorEnd]...)
	c.VendorString = displayString(c.Vendor)
	if !utf8.Valid(c.Vendor) {
		at := firstInvalidUTF8(c.Vendor)
		c.Issues = append(c.Issues, Issue{
			Code:       CodeVendorInvalidUTF8,
			Class:      ManualConfirmation,
			Message:    "vendor string is not valid UTF-8",
			Offset:     base + int64(off+at),
			Length:     utf8RuneLength(c.Vendor[at:]),
			EntryIndex: -1,
		})
	}
	off = vendorEnd

	if len(body)-off < 4 {
		c.Complete = false
		c.Issues = append(c.Issues, Issue{
			Code:       CodeTruncatedCommentCount,
			Class:      ManualConfirmation,
			Message:    "fewer than four bytes remain for the comment count",
			Offset:     base + int64(off),
			Length:     len(body) - off,
			EntryIndex: -1,
		})
		return nil
	}

	declared := uint64(binary.LittleEndian.Uint32(body[off : off+4]))
	off += 4
	for index := uint64(0); index < declared; index++ {
		if len(body)-off < 4 {
			c.Complete = false
			c.Issues = append(c.Issues, Issue{
				Code:       CodeTruncatedCommentCount,
				Class:      ManualConfirmation,
				Message:    fmt.Sprintf("comment count declares %d entries, %d were present", declared, index),
				Offset:     base + int64(off),
				Length:     len(body) - off,
				EntryIndex: -1,
			})
			break
		}

		length := uint64(binary.LittleEndian.Uint32(body[off : off+4]))
		recordStart := off + 4
		truncated := length > uint64(len(body)-recordStart)
		available := int(length)
		if truncated {
			c.Complete = false
			available = len(body) - recordStart
		}
		raw := body[recordStart : recordStart+available]
		entry := parseEntry(int(index), raw, base+int64(recordStart), opts)
		entry.Truncated = truncated
		c.Entries = append(c.Entries, entry)
		if truncated {
			c.Issues = append(c.Issues, Issue{
				Code:       CodeTruncatedComment,
				Class:      ManualConfirmation,
				Message:    fmt.Sprintf("comment declares %d bytes, only %d remain", length, available),
				Offset:     base + int64(recordStart),
				Length:     available,
				EntryIndex: int(index),
				Key:        entry.Key,
			})
			off = len(body)
			break
		}
		off = recordStart + available
	}

	parseRemainder(c, body[off:], base+int64(off))
	return nil
}

func parseRemainder(c *Comment, rest []byte, absolute int64) {
	c.remainder = append([]byte(nil), rest...)
	if len(rest) == 0 {
		if c.HasPacketMagic {
			c.Issues = append(c.Issues, Issue{
				Code:       CodeMissingFramingBit,
				Class:      Repairable,
				Message:    "comment packet has no framing bit",
				Offset:     absolute,
				EntryIndex: -1,
			})
		}
		return
	}
	if len(rest) == 1 && rest[0] == 1 {
		c.HasFramingBit = true
		return
	}
	if allZero(rest) {
		c.Issues = append(c.Issues, Issue{
			Code:       CodeTrailingData,
			Class:      Repairable,
			Message:    "zero padding after the last comment can be discarded",
			Offset:     absolute,
			Length:     len(rest),
			EntryIndex: -1,
		})
		return
	}
	c.Issues = append(c.Issues, Issue{
		Code:       CodeTrailingData,
		Class:      ManualConfirmation,
		Message:    "unrecognized bytes follow the declared comments",
		Offset:     absolute,
		Length:     len(rest),
		EntryIndex: -1,
	})
}

func parseEntry(index int, raw []byte, absolute int64, opts Options) Entry {
	entry := Entry{
		Index:       index,
		Raw:         raw,
		Offset:      absolute,
		Length:      len(raw),
		KeyOffset:   absolute,
		ValueOffset: absolute + int64(len(raw)),
	}
	separator := bytes.IndexByte(raw, '=')
	if separator < 0 {
		entry.Key = displayString(raw)
		entry.RawKey = raw
		entry.KeyLength = len(raw)
		entry.Value = ""
		return entry
	}

	entry.HasSeparator = true
	entry.RawKey = raw[:separator]
	entry.RawValue = raw[separator+1:]
	entry.Key = displayString(entry.RawKey)
	entry.Value = displayString(entry.RawValue)
	entry.KeyLength = separator
	entry.ValueOffset = absolute + int64(separator+1)
	entry.ValueLength = len(entry.RawValue)
	return entry
}

func addDuplicateDiagnostics(c *Comment, opts Options) {
	seen := make(map[string]int)
	for i := range c.Entries {
		entry := &c.Entries[i]
		if !entry.HasSeparator || len(entry.RawKey) == 0 {
			addEntryIssues(c, entry, opts)
			continue
		}
		normalized := string(bytes.ToLower(entry.RawKey))
		if first, ok := seen[normalized]; ok {
			c.Issues = append(c.Issues, Issue{
				Code:       CodeDuplicateKey,
				Class:      Repairable,
				Message:    fmt.Sprintf("key duplicates entry %d; both entries are retained", first),
				Offset:     entry.KeyOffset,
				Length:     entry.KeyLength,
				EntryIndex: entry.Index,
				Key:        entry.Key,
			})
			if !bytes.Equal(c.Entries[first].RawValue, entry.RawValue) {
				c.Issues = append(c.Issues, Issue{
					Code:       CodeConflictingDuplicate,
					Class:      ManualConfirmation,
					Message:    fmt.Sprintf("value differs from entry %d; choose a value manually", first),
					Offset:     entry.ValueOffset,
					Length:     entry.ValueLength,
					EntryIndex: entry.Index,
					Key:        entry.Key,
				})
			}
		} else {
			seen[normalized] = i
		}
		addEntryIssues(c, entry, opts)
	}
}

func addEntryIssues(c *Comment, entry *Entry, opts Options) {
	if !entry.HasSeparator {
		c.Issues = append(c.Issues, Issue{
			Code:       CodeMissingSeparator,
			Class:      ManualConfirmation,
			Message:    "comment has no '=' separator; raw bytes are retained",
			Offset:     entry.Offset,
			Length:     entry.Length,
			EntryIndex: entry.Index,
			Key:        entry.Key,
		})
		return
	}
	if len(entry.RawKey) == 0 {
		c.Issues = append(c.Issues, Issue{
			Code:       CodeEmptyKey,
			Class:      ManualConfirmation,
			Message:    "comment key is empty",
			Offset:     entry.KeyOffset,
			Length:     0,
			EntryIndex: entry.Index,
		})
	} else if !validFieldName(entry.RawKey) {
		c.Issues = append(c.Issues, Issue{
			Code:       CodeInvalidFieldName,
			Class:      ManualConfirmation,
			Message:    "comment key must use printable ASCII excluding '='",
			Offset:     entry.KeyOffset,
			Length:     entry.KeyLength,
			EntryIndex: entry.Index,
			Key:        entry.Key,
		})
	}
	if !utf8.Valid(entry.RawKey) {
		at := firstInvalidUTF8(entry.RawKey)
		c.Issues = append(c.Issues, Issue{
			Code:       CodeInvalidUTF8,
			Class:      ManualConfirmation,
			Message:    "comment key is not valid UTF-8; raw bytes are retained",
			Offset:     entry.KeyOffset + int64(at),
			Length:     utf8RuneLength(entry.RawKey[at:]),
			EntryIndex: entry.Index,
			Key:        entry.Key,
		})
	}
	if !utf8.Valid(entry.RawValue) {
		at := firstInvalidUTF8(entry.RawValue)
		c.Issues = append(c.Issues, Issue{
			Code:       CodeInvalidUTF8,
			Class:      ManualConfirmation,
			Message:    "comment value is not valid UTF-8; raw bytes are retained",
			Offset:     entry.ValueOffset + int64(at),
			Length:     utf8RuneLength(entry.RawValue[at:]),
			EntryIndex: entry.Index,
			Key:        entry.Key,
		})
	}
	if entry.ValueLength > opts.maxValueBytes() {
		c.Issues = append(c.Issues, Issue{
			Code:       CodeValueTooLong,
			Class:      ManualConfirmation,
			Message:    fmt.Sprintf("comment value is %d bytes; limit is %d", entry.ValueLength, opts.maxValueBytes()),
			Offset:     entry.ValueOffset,
			Length:     entry.ValueLength,
			EntryIndex: entry.Index,
			Key:        entry.Key,
		})
	}
}

func firstInvalidUTF8(b []byte) int {
	offset := 0
	for len(b) > 0 {
		if b[0] < utf8.RuneSelf {
			b = b[1:]
			offset++
			continue
		}
		_, size := utf8.DecodeRune(b)
		if size == 1 {
			return offset
		}
		b = b[size:]
		offset += size
	}
	return 0
}

func utf8RuneLength(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	_, size := utf8.DecodeRune(b)
	return size
}

func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
