package vorbiscomment

import (
	"bytes"
	"fmt"
)

const (
	// DefaultMaxValueBytes is intentionally conservative. Applications that
	// carry large artwork in a value should raise this through Options.
	DefaultMaxValueBytes = 64 * 1024

	maxUint32 = uint64(^uint32(0))
)

var packetMagic = []byte{0x03, 'v', 'o', 'r', 'b', 'i', 's'}

// Options controls lenient parsing. The zero value is ready for use.
type Options struct {
	// BaseOffset is added to every reported offset. Set it to the absolute file
	// position of the comment packet when parsing an embedded packet.
	BaseOffset int64

	// MaxValueBytes is the policy limit for a single comment value. Values over
	// this size are retained byte-for-byte and reported as manual findings.
	MaxValueBytes int
}

func (o Options) maxValueBytes() int {
	if o.MaxValueBytes > 0 {
		return o.MaxValueBytes
	}
	return DefaultMaxValueBytes
}

// Entry is one user comment. Raw is retained so a conservative rewrite can
// preserve unknown keys, unusual casing, duplicate entries, and malformed
// values exactly as they arrived.
type Entry struct {
	Index        int    `json:"index"`
	Key          string `json:"key"`
	Value        string `json:"value"`
	Raw          []byte `json:"-"`
	RawKey       []byte `json:"-"`
	RawValue     []byte `json:"-"`
	Offset       int64  `json:"offset"`
	Length       int    `json:"length"`
	KeyOffset    int64  `json:"key_offset"`
	KeyLength    int    `json:"key_length"`
	ValueOffset  int64  `json:"value_offset"`
	ValueLength  int    `json:"value_length"`
	HasSeparator bool   `json:"has_separator"`
	Truncated    bool   `json:"truncated,omitempty"`
}

// Comment is a leniently decoded Vorbis comment block. Entries remain in their
// original order; Parse never maps them into a key->value dictionary.
type Comment struct {
	Vendor       []byte  `json:"-"`
	VendorString string  `json:"vendor"`
	Entries      []Entry `json:"entries"`
	Issues       []Issue `json:"issues"`

	// HeaderOffset is the absolute offset of the four-byte vendor length.
	HeaderOffset int64 `json:"header_offset"`
	// HeaderLength is the number of bytes occupied by the comment structure.
	HeaderLength   int  `json:"header_length"`
	HasPacketMagic bool `json:"has_packet_magic"`
	HasFramingBit  bool `json:"has_framing_bit"`
	// Complete is false when the declared vendor, count, or comment bytes were
	// truncated. Such a block is inspectable but cannot be rewritten safely.
	Complete bool `json:"complete"`

	raw       []byte
	remainder []byte
}

func (c *Comment) Raw() []byte {
	return append([]byte(nil), c.raw...)
}

func (c *Comment) RequiresManualConfirmation() bool {
	return hasManualIssue(c.Issues)
}

func (c *Comment) RepairableIssues() []Issue {
	return issuesByClass(c.Issues, Repairable)
}

func (c *Comment) ManualIssues() []Issue {
	return issuesByClass(c.Issues, ManualConfirmation)
}

func (c *Comment) EntryIssues(index int) []Issue {
	var out []Issue
	for _, issue := range c.Issues {
		if issue.EntryIndex == index {
			out = append(out, issue)
		}
	}
	return out
}

func issuesByClass(issues []Issue, class Class) []Issue {
	var out []Issue
	for _, issue := range issues {
		if issue.Class == class {
			out = append(out, issue)
		}
	}
	return out
}

// Values returns all values for key, case-insensitively, without discarding
// duplicates.
func (c *Comment) Values(key string) [][]byte {
	var out [][]byte
	for _, entry := range c.Entries {
		if bytes.EqualFold(entry.RawKey, []byte(key)) {
			out = append(out, append([]byte(nil), entry.RawValue...))
		}
	}
	return out
}

func (c *Comment) String() string {
	return fmt.Sprintf("vendor=%q entries=%d issues=%d",
		c.VendorString, len(c.Entries), len(c.Issues))
}
