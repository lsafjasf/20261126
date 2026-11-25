package vorbiscomment

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func makeBody(vendor string, entries ...string) []byte {
	body := make([]byte, 4, 32)
	binary.LittleEndian.PutUint32(body, uint32(len(vendor)))
	body = append(body, vendor...)
	body = appendUint32(body, len(entries))
	for _, entry := range entries {
		body = appendUint32(body, len(entry))
		body = append(body, entry...)
	}
	return body
}

func makePacket(vendor string, entries ...string) []byte {
	body := makeBody(vendor, entries...)
	packet := append([]byte(nil), packetMagic...)
	packet = append(packet, body...)
	return append(packet, 1)
}

func findIssue(t *testing.T, comment *Comment, code Code) Issue {
	t.Helper()
	for _, issue := range comment.Issues {
		if issue.Code == code {
			return issue
		}
	}
	t.Fatalf("missing issue %q in %#v", code, comment.Issues)
	return Issue{}
}

func TestParseDuplicateKeysPreservesOrder(t *testing.T) {
	comment, err := Parse(makePacket("encoder", "ARTIST=one", "artist=two", "X-UNKNOWN=keep"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(comment.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(comment.Entries))
	}
	if comment.Entries[0].Key != "ARTIST" || comment.Entries[1].Key != "artist" {
		t.Fatalf("keys were not retained in order: %#v", comment.Entries)
	}
	duplicate := findIssue(t, comment, CodeDuplicateKey)
	if !duplicate.IsRepairable() {
		t.Fatalf("duplicate class = %s, want repairable", duplicate.Class)
	}
	conflict := findIssue(t, comment, CodeConflictingDuplicate)
	if !conflict.RequiresManualConfirmation() {
		t.Fatalf("conflict class = %s, want manual", conflict.Class)
	}
	if got := comment.Values("artist"); len(got) != 2 ||
		string(got[0]) != "one" || string(got[1]) != "two" {
		t.Fatalf("Values(artist) = %q", got)
	}
}

func TestParseInvalidUTF8IsLocalized(t *testing.T) {
	body := makeBody("encoder", "TITLE=\xff")
	lengthAt := 4 + len("encoder")
	entryAt := lengthAt + 4
	badValueAt := entryAt + 4 + len("TITLE=")

	comment, err := Parse(body, Options{BaseOffset: 100})
	if err != nil {
		t.Fatal(err)
	}
	if string(comment.Entries[0].RawValue) != "\xff" {
		t.Fatalf("raw value = %q", comment.Entries[0].RawValue)
	}
	issue := findIssue(t, comment, CodeInvalidUTF8)
	if issue.Offset != int64(100+badValueAt) || issue.Length != 1 {
		t.Fatalf("invalid UTF-8 location = %d+%d, want %d+1",
			issue.Offset, issue.Length, 100+badValueAt)
	}
	if !issue.RequiresManualConfirmation() {
		t.Fatalf("invalid UTF-8 must require manual confirmation")
	}
}

func TestParseOverlongValueIsRetained(t *testing.T) {
	value := strings.Repeat("x", 33)
	comment, err := Parse(makeBody("encoder", "COMMENT="+value), Options{MaxValueBytes: 32})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(comment.Entries[0].RawValue); got != value {
		t.Fatalf("value was changed: len=%d", len(got))
	}
	issue := findIssue(t, comment, CodeValueTooLong)
	if !issue.RequiresManualConfirmation() {
		t.Fatalf("overlong value class = %s, want manual", issue.Class)
	}
	if issue.EntryIndex != 0 {
		t.Fatalf("entry index = %d, want 0", issue.EntryIndex)
	}
}

func TestParsePacketOffsetIncludesMagic(t *testing.T) {
	packet := makePacket("v")
	comment, err := Parse(packet, Options{BaseOffset: 10})
	if err != nil {
		t.Fatal(err)
	}
	if comment.HeaderOffset != 17 {
		t.Fatalf("header offset = %d", comment.HeaderOffset)
	}
	if comment.Entries != nil {
		t.Fatalf("unexpected entries: %#v", comment.Entries)
	}
	if !comment.HasPacketMagic || !comment.HasFramingBit {
		t.Fatalf("packet flags = magic:%v framing:%v",
			comment.HasPacketMagic, comment.HasFramingBit)
	}
}

func TestParseMalformedFieldsAreNotFatal(t *testing.T) {
	body := makeBody("encoder", "MISSING", "=empty-key")
	comment, err := Parse(body, Options{})
	if err != nil {
		t.Fatal(err)
	}
	findIssue(t, comment, CodeMissingSeparator)
	findIssue(t, comment, CodeEmptyKey)
	if len(comment.Entries) != 2 {
		t.Fatalf("entries = %d", len(comment.Entries))
	}
}

func TestParseTooShortStillReturnsDiagnostic(t *testing.T) {
	comment, err := Parse([]byte{1, 2, 3}, Options{BaseOffset: 7})
	if !errors.Is(err, ErrMissingVendorLength) {
		t.Fatalf("error = %v", err)
	}
	if comment.Complete {
		t.Fatal("truncated input marked complete")
	}
	issue := findIssue(t, comment, CodeTruncatedVendor)
	if issue.Offset != 7 || issue.Length != 3 {
		t.Fatalf("location = %d+%d", issue.Offset, issue.Length)
	}
}

func TestTruncatedCommentCannotBeRewrittenAsComplete(t *testing.T) {
	input := makeBody("encoder", "TITLE=hello")
	input = input[:len(input)-2]
	comment, err := Parse(input, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if comment.Complete {
		t.Fatal("truncated comment marked complete")
	}
	if _, err := comment.Marshal(RewriteOptions{}); !errors.Is(err, ErrIncompleteComment) {
		t.Fatalf("Marshal error = %v, want ErrIncompleteComment", err)
	}
}

func TestUnknownKeysSurviveConservativeRewrite(t *testing.T) {
	input := makeBody("encoder",
		"X-CUSTOM=opaque", "TITLE=hello", "X-CUSTOM=second", "BAD=\xff", "EMPTY=")
	out, comment, err := Rewrite(input, Options{}, RewriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, input) {
		t.Fatalf("conservative rewrite changed bytes\nin:  %x\nout: %x", input, out)
	}
	if len(comment.Entries) != 5 {
		t.Fatalf("entries = %d, want 5", len(comment.Entries))
	}
	if got := string(comment.Entries[3].RawValue); got != "\xff" {
		t.Fatalf("invalid UTF-8 value changed to %q", got)
	}
}

func TestSafeModeRepairsOnlyKnownTrailer(t *testing.T) {
	padded := append(makeBody("encoder", "TITLE=hello"), 0, 0, 0)
	out, _, err := Rewrite(padded, Options{}, RewriteOptions{Mode: RewriteSafe})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, makeBody("encoder", "TITLE=hello")) {
		t.Fatalf("zero padding was not removed: %x", out)
	}

	unknown := append(makeBody("encoder", "TITLE=hello"), 0xde, 0xad)
	out, comment, err := Rewrite(unknown, Options{}, RewriteOptions{Mode: RewriteSafe})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, unknown) {
		t.Fatalf("unknown trailer was changed: %x", out)
	}
	findIssue(t, comment, CodeTrailingData)
}

func FuzzParseNeverPanics(f *testing.F) {
	f.Add([]byte{})
	f.Add(makeBody("encoder", "TITLE=hello"))
	f.Add(makePacket("vendor", "ARTIST=one", "artist=two"))
	f.Fuzz(func(t *testing.T, data []byte) {
		comment, err := Parse(data, Options{MaxValueBytes: 32})
		if err != nil {
			return
		}
		if comment.Complete {
			_, _ = comment.Marshal(RewriteOptions{Mode: RewriteSafe})
		}
	})
}
