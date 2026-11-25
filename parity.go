package cea608

// oddParity reports whether b has an odd number of set bits.
// CEA-608 stores the odd parity bit in bit 7.
func oddParity(b byte) bool {
	b ^= b >> 4
	b ^= b >> 2
	b ^= b >> 1
	return b&1 == 1
}

func normalizedPair(raw1, raw2 byte) Pair {
	return Pair{
		B1:   raw1 & 0x7f,
		B2:   raw2 & 0x7f,
		Raw1: raw1,
		Raw2: raw2,
	}
}

// WithOddParity returns the seven-bit value with its odd parity bit added.
// It is mainly useful in tests and encoders.
func WithOddParity(b byte) byte {
	b &= 0x7f
	if oddParity(b) {
		return b
	}
	return b | 0x80
}
