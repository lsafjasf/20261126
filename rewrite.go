package vorbiscomment

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrCannotEncode reports a structural value that cannot be represented in a
// Vorbis comment block. Field-level defects do not produce this error.
var ErrCannotEncode = errors.New("vorbis comment: cannot encode")

// ErrIncompleteComment means the input was truncated and cannot be rewritten
// without inventing or dropping bytes.
var ErrIncompleteComment = errors.New("vorbis comment: input is structurally incomplete")

// RewriteMode selects how aggressively anomalies are changed.
type RewriteMode string

const (
	// RewriteConservative emits all vendor and entry bytes exactly as parsed.
	// Duplicate and unknown fields remain in place.
	RewriteConservative RewriteMode = "conservative"
	// RewriteSafe additionally drops zero padding and restores a missing packet
	// framing bit. It still keeps every entry, value, and unknown key intact.
	RewriteSafe RewriteMode = "safe"
)

// RewriteOptions controls serialization. The zero value is conservative.
type RewriteOptions struct {
	Mode RewriteMode
}

// Marshal re-encodes the comment without interpreting or merging entry keys.
// Raw entry bytes are used, so invalid UTF-8 and over-limit values survive.
func (c *Comment) Marshal(opts RewriteOptions) ([]byte, error) {
	mode := opts.Mode
	if mode == "" {
		mode = RewriteConservative
	}
	if mode != RewriteConservative && mode != RewriteSafe {
		return nil, fmt.Errorf("%w: unknown rewrite mode %q", ErrCannotEncode, mode)
	}
	if !c.Complete {
		return nil, ErrIncompleteComment
	}
	if c.HasPacketMagic && c.Vendor == nil {
		return nil, fmt.Errorf("%w: packet magic present but vendor is unavailable", ErrCannotEncode)
	}

	var out []byte
	if c.HasPacketMagic {
		out = append(out, packetMagic...)
	}
	if uint64(len(c.Vendor)) > maxUint32 {
		return nil, fmt.Errorf("%w: vendor is %d bytes", ErrCannotEncode, len(c.Vendor))
	}
	if uint64(len(c.Entries)) > maxUint32 {
		return nil, fmt.Errorf("%w: too many entries: %d", ErrCannotEncode, len(c.Entries))
	}
	out = appendUint32(out, len(c.Vendor))
	out = append(out, c.Vendor...)
	out = appendUint32(out, len(c.Entries))
	for i := range c.Entries {
		entry := &c.Entries[i]
		raw := entry.Raw
		if !entry.HasSeparator && len(entry.Raw) == 0 {
			raw = encodeEntry(entry)
		}
		if uint64(len(raw)) > maxUint32 {
			return nil, fmt.Errorf("%w: entry %d is %d bytes", ErrCannotEncode, i, len(raw))
		}
		out = appendUint32(out, len(raw))
		out = append(out, raw...)
	}

	trailer := c.remainder
	if mode == RewriteSafe {
		trailer = safeTrailer(c)
	}
	if uint64(len(out))+uint64(len(trailer)) > maxUint32 {
		return nil, fmt.Errorf("%w: comment block is too large", ErrCannotEncode)
	}
	out = append(out, trailer...)
	return out, nil
}

// Rewrite parses and conservatively serializes data. It returns the diagnostics
// even when a repairable or manual finding is present.
func Rewrite(data []byte, parseOpts Options, rewriteOpts RewriteOptions) ([]byte, *Comment, error) {
	comment, parseErr := Parse(data, parseOpts)
	if parseErr != nil {
		return nil, comment, parseErr
	}
	out, err := comment.Marshal(rewriteOpts)
	if err != nil {
		return nil, comment, err
	}
	return out, comment, nil
}

func encodeEntry(entry *Entry) []byte {
	out := make([]byte, 0, len(entry.RawKey)+1+len(entry.RawValue))
	out = append(out, entry.RawKey...)
	out = append(out, '=')
	out = append(out, entry.RawValue...)
	return out
}

func safeTrailer(c *Comment) []byte {
	if c.HasFramingBit {
		return []byte{1}
	}
	if len(c.remainder) == 0 {
		if c.HasPacketMagic {
			return []byte{1}
		}
		return nil
	}
	if allZero(c.remainder) {
		if c.HasPacketMagic {
			return []byte{1}
		}
		return nil
	}
	return c.remainder
}

func appendUint32(dst []byte, n int) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(n))
	return append(dst, buf[:]...)
}
