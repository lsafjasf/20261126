package imsc

import (
	"errors"
	"fmt"
	"strings"
)

// Reason classifies why an element could not be satisfied. Every reason is a
// stable, non-empty string so failures can be matched programmatically and are
// never reported as blank messages.
type Reason string

const (
	ReasonMalformedXML     Reason = "malformed_xml"
	ReasonMissingAttr      Reason = "missing_attribute"
	ReasonEmptyAttr        Reason = "empty_attribute"
	ReasonDuplicateID      Reason = "duplicate_id"
	ReasonBadClock         Reason = "bad_clock_expression"
	ReasonBadTiming        Reason = "inconsistent_timing"
	ReasonBadEncoding      Reason = "unsupported_encoding"
	ReasonBadBase64        Reason = "invalid_base64_data"
	ReasonUnsupportedImage Reason = "unsupported_image_type"
	ReasonUnreadableImage  Reason = "unreadable_image_data"
	ReasonExternalImage    Reason = "external_image_reference"
	ReasonUnresolvedRef    Reason = "unresolved_image_reference"
	ReasonUnknownRegion    Reason = "unknown_region"
	ReasonRegionNoImage    Reason = "region_without_background_image"
)

// ErrNoImages is returned by Load when the document does not contain any
// decodable smpte:image resource. The partially built document is still
// returned so callers can inspect the element-level errors.
var ErrNoImages = errors.New("imsc: document contains no decodable images")

// ElementError attaches a failure to the exact element that caused it. Element
// and Message are always non-empty; constructing an ElementError with an empty
// element or message panics, because an empty string must never masquerade as
// a successful load.
type ElementError struct {
	Element string
	Line    int
	Reason  Reason
	Message string
}

// NewElementError builds an ElementError. It panics when element or message is
// blank, or when reason is blank, so no error can ever carry empty identifying
// information.
func NewElementError(element string, line int, reason Reason, message string) *ElementError {
	if strings.TrimSpace(element) == "" {
		panic("imsc: NewElementError requires a non-empty element identifier")
	}
	if strings.TrimSpace(message) == "" {
		panic("imsc: NewElementError requires a non-empty message")
	}
	if strings.TrimSpace(string(reason)) == "" {
		panic("imsc: NewElementError requires a non-empty reason")
	}
	return &ElementError{
		Element: element,
		Line:    line,
		Reason:  reason,
		Message: message,
	}
}

func (e *ElementError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s (line %d): %s: %s", e.Element, e.Line, e.Reason, e.Message)
	}
	return fmt.Sprintf("%s: %s: %s", e.Element, e.Reason, e.Message)
}

// ErrorList aggregates element-level errors while preserving their order.
// It may also wrap sentinel errors such as ErrNoImages, which remain
// reachable through errors.Is via Unwrap.
type ErrorList struct {
	errs []error
}

// append adds an error to the list.
func (l *ErrorList) append(err error) {
	l.errs = append(l.errs, err)
}

// All returns the ordered underlying errors.
func (l *ErrorList) All() []error { return l.errs }

// Len reports how many errors the list holds.
func (l *ErrorList) Len() int { return len(l.errs) }

// ElementErrors returns only the concrete ElementError values.
func (l *ErrorList) ElementErrors() []*ElementError {
	var out []*ElementError
	for _, err := range l.errs {
		var elemErr *ElementError
		if errors.As(err, &elemErr) {
			out = append(out, elemErr)
		}
	}
	return out
}

func (l ErrorList) Error() string {
	parts := make([]string, 0, len(l.errs))
	for _, err := range l.errs {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "; ")
}

// Unwrap exposes joined sentinel errors (for example ErrNoImages) to errors.Is.
func (l ErrorList) Unwrap() []error { return l.errs }

// AtLine returns the error's source line, or 0 when err is not an
// ElementError.
func AtLine(err error) int {
	var elemErr *ElementError
	if errors.As(err, &elemErr) {
		return elemErr.Line
	}
	return 0
}
