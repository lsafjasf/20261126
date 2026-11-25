// Package imsc loads and validates IMSC (TTML image profile) subtitle
// documents.
//
// # Images
//
// Every smpte:image element is checked for xml:id, image type and Base64
// encoding. The payload is decoded and sniffed as PNG or JPEG and must match
// the declared type. Broken resources are reported as ElementError values and
// never appear in Document.Images, so a failed load can never be consumed as a
// successful image.
//
// # Regions and references
//
// smpte:backgroundImage accepts an ordered space-separated reference list.
// Only same-document fragments ("#id") are allowed; external URIs are
// rejected. Cue resolution uses the IMSC priority: inline tt:p background,
// then span background, then the referenced region. The first decodable
// reference in each list wins; unresolved references fall through to the next
// and are recorded on the offending element.
//
// # Timing
//
// begin/end accept TTML offset-time ("1.5s", "200ms") and clock-time
// ("HH:MM:SS[.fraction]" or "HH:MM:SS:FF") expressions. Invalid timing keeps
// the cue but flags TimingValid=false and adds an element error. An omitted
// end produces an open-ended cue.
//
// # Fallback behaviour
//
// A cue whose images cannot be resolved gets a non-nil Placeholder carrying
// the failure reason and every failed reference. The cue's paragraph text is
// exposed as FallbackText so the text track can show it instead of the
// missing image. FallbackText is only set when real, non-whitespace text
// exists; empty strings are never treated as content or as success.
//
// # No images
//
// When the whole document has no decodable image, Load still returns the
// built Document together with ErrNoImages in the error chain (errors.Is).
// Every cue then carries a Placeholder, making the no-image behaviour
// explicit instead of silently succeeding.
package imsc
