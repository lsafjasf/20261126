package ac3select

import "io"

// bitReader is a big-endian, MSB-first bit reader used by AC-3 syntax.
type bitReader struct {
	data []byte
	pos  int64 // absolute bit position in data
}

func newBitReader(data []byte) *bitReader {
	return &bitReader{data: data}
}

func (r *bitReader) BitPos() int64 {
	return r.pos
}

func (r *bitReader) BytePos() int64 {
	return r.pos >> 3
}

func (r *bitReader) remaining() int64 {
	return int64(len(r.data))*8 - r.pos
}

func (r *bitReader) readBits(n uint) (int, error) {
	if n > 31 {
		return 0, io.ErrUnexpectedEOF
	}
	if int64(n) > r.remaining() {
		return 0, io.ErrUnexpectedEOF
	}

	value := 0
	for i := uint(0); i < n; i++ {
		byteIndex := r.pos >> 3
		bitIndex := uint(7 - (r.pos & 7))
		bit := int((r.data[byteIndex] >> bitIndex) & 1)
		value = (value << 1) | bit
		r.pos++
	}
	return value, nil
}

func (r *bitReader) readBit() (bool, error) {
	v, err := r.readBits(1)
	return v == 1, err
}

func (r *bitReader) skipBits(n int64) error {
	if n < 0 || n > r.remaining() {
		return io.ErrUnexpectedEOF
	}
	r.pos += n
	return nil
}

// alignByte moves to the next byte boundary. It is currently only used by
// tests and diagnostics; AC-3 fields are packed continuously.
func (r *bitReader) alignByte() error {
	if rem := r.pos & 7; rem != 0 {
		return r.skipBits(8 - rem)
	}
	return nil
}
