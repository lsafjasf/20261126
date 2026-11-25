package ac3select

type testFrameSpec struct {
	bsmod   BitstreamMode
	acmod   AudioCodingMode
	lfeon   bool
	couple  bool
	coupled []bool
	start   int
	end     int
	crc     bool
}

type bitWriter struct {
	buf []byte
	pos int
}

func newBitWriter(size int) *bitWriter {
	return &bitWriter{buf: make([]byte, size)}
}

func (w *bitWriter) writeBits(value, n int) {
	for i := n - 1; i >= 0; i-- {
		bit := (value >> i) & 1
		byteIndex := w.pos >> 3
		bitIndex := uint(7 - (w.pos & 7))
		if bit == 1 {
			w.buf[byteIndex] |= 1 << bitIndex
		}
		w.pos++
	}
}

func (w *bitWriter) writeBit(v bool) {
	if v {
		w.writeBits(1, 1)
	} else {
		w.writeBits(0, 1)
	}
}

func makeTestFrame(spec testFrameSpec) []byte {
	const frameCode = 20 // 384/417/576 words; fscod=1 gives 834 bytes
	const frameSize = 834
	w := newBitWriter(frameSize)
	w.writeBits(0x0B77, 16)
	w.writeBits(0, 16) // crc1
	w.writeBits(1, 2)  // fscod: 44.1 kHz
	w.writeBits(frameCode, 6)
	w.writeBits(8, 5) // bsid
	w.writeBits(int(spec.bsmod), 3)
	w.writeBits(int(spec.acmod), 3)

	switch spec.acmod {
	case ACMod2_0:
		w.writeBits(0, 2) // dsurmod
	default:
		if spec.acmod&1 == 1 && spec.acmod != ACMod1_0 {
			w.writeBits(0, 2) // cmixlev
		}
		if spec.acmod&4 == 4 {
			w.writeBits(0, 2) // surmixlev
		}
	}
	w.writeBit(spec.lfeon)

	iterations := 1
	if spec.acmod == ACModDualMono {
		iterations = 2
	}
	for i := 0; i < iterations; i++ {
		w.writeBits(-20&31, 5) // dialnorm
		w.writeBit(false)      // compre
		w.writeBit(false)      // langcode exists
		w.writeBit(false)      // audio production info exists
	}
	w.writeBit(false) // copyright
	w.writeBit(false) // original
	w.writeBit(false) // timecod1e
	w.writeBit(false) // timecod2e
	w.writeBit(false) // addbsie

	fbw := fullBandChannelCount[spec.acmod]
	for i := 0; i < fbw; i++ {
		w.writeBit(false) // block switch
	}
	for i := 0; i < fbw; i++ {
		w.writeBit(false) // dither flags
	}
	for i := 0; i < iterations; i++ {
		w.writeBit(false) // dynrnge
	}

	w.writeBit(true) // cplstre[0]
	w.writeBit(spec.couple)
	if spec.couple && spec.acmod >= ACMod2_0 {
		for i := 0; i < fbw; i++ {
			included := i < len(spec.coupled) && spec.coupled[i]
			w.writeBit(included)
		}
		if spec.acmod == ACMod2_0 {
			w.writeBit(false) // phase flags in use
		}
		start := spec.start
		end := spec.end
		if end == 0 {
			start, end = 10, 17
		}
		w.writeBits(start, 4)
		w.writeBits(end-3, 4)
		for i := 0; i < end-start-1; i++ {
			w.writeBit(i == 0) // one explicit merge boundary
		}
	}

	if spec.crc {
		crc := crc16ANSI(w.buf[2 : frameSize-2])
		w.buf[frameSize-2] = byte(crc >> 8)
		w.buf[frameSize-1] = byte(crc)
	}
	return w.buf
}
