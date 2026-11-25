package vorbiscomment

import (
	"fmt"
	"strings"
)

// Class describes whether a finding can be handled mechanically without
// changing user data, or whether a person must decide what the value should be.
type Class string

const (
	// Repairable findings can be normalized without losing information.
	Repairable Class = "repairable"
	// ManualConfirmation findings are reported but their raw bytes are kept.
	// Guessing here would silently change metadata.
	ManualConfirmation Class = "manual_confirmation"
)

// Code is a stable machine-readable diagnostic identifier.
type Code string

const (
	CodeDuplicateKey          Code = "duplicate_key"
	CodeConflictingDuplicate  Code = "conflicting_duplicate"
	CodeInvalidUTF8           Code = "invalid_utf8"
	CodeValueTooLong          Code = "value_too_long"
	CodeInvalidFieldName      Code = "invalid_field_name"
	CodeMissingSeparator      Code = "missing_separator"
	CodeEmptyKey              Code = "empty_key"
	CodeTruncatedComment      Code = "truncated_comment"
	CodeTruncatedCommentCount Code = "truncated_comment_count"
	CodeTruncatedVendor       Code = "truncated_vendor"
	CodeVendorInvalidUTF8     Code = "vendor_invalid_utf8"
	CodeTrailingData          Code = "trailing_data"
	CodeMissingFramingBit     Code = "missing_framing_bit"
)

// Issue is a diagnostic tied to a byte range in the input. Offsets include the
// BaseOffset supplied in Options and are therefore usable as file offsets.
type Issue struct {
	Code       Code   `json:"code"`
	Class      Class  `json:"class"`
	Message    string `json:"message"`
	Offset     int64  `json:"offset"`
	Length     int    `json:"length"`
	EntryIndex int    `json:"entry_index"` // -1 for vendor/container findings
	Key        string `json:"key,omitempty"`
}

func (i Issue) IsRepairable() bool {
	return i.Class == Repairable
}

func (i Issue) RequiresManualConfirmation() bool {
	return i.Class == ManualConfirmation
}

func (i Issue) String() string {
	key := ""
	if i.Key != "" {
		key = " key=" + fmt.Sprintf("%q", i.Key)
	}
	return fmt.Sprintf("%s at %d+%d (%s): %s%s",
		i.Code, i.Offset, i.Length, i.Class, i.Message, key)
}

func hasManualIssue(issues []Issue) bool {
	for _, issue := range issues {
		if issue.RequiresManualConfirmation() {
			return true
		}
	}
	return false
}

func validFieldName(name []byte) bool {
	if len(name) == 0 {
		return false
	}
	for _, b := range name {
		if b < 0x20 || b > 0x7d || b == '=' {
			return false
		}
	}
	return true
}

func displayString(b []byte) string {
	return strings.ToValidUTF8(string(b), "\uFFFD")
}
