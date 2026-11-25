package ac3select

import (
	"errors"
	"fmt"
)

var (
	ErrShortHeader     = errors.New("ac3: short header")
	ErrSyncWord        = errors.New("ac3: missing 0x0b77 sync word")
	ErrBitstreamID     = errors.New("ac3: invalid bitstream id")
	ErrUnsupportedEAC3 = errors.New("ac3: enhanced AC-3 bitstream id is not supported by this AC-3 parser")
	ErrSampleRateCode  = errors.New("ac3: reserved sample rate code")
	ErrFrameSizeCode   = errors.New("ac3: invalid frame size code")
	ErrFrameTruncated  = errors.New("ac3: frame extends beyond input")
	ErrCoupling        = errors.New("ac3: invalid coupling strategy")
)

// ParseHeader parses the fixed AC-3 syncinfo and BSI fields through lfeon.
// b may contain more than one frame; only the bytes at the start are used.
func ParseHeader(b []byte) (Header, error) {
	return parseHeader(newBitReader(b))
}

func parseHeader(br *bitReader) (Header, error) {
	b := br.data
	if len(b) < 7 {
		return Header{}, fmt.Errorf("%w: need at least 7 bytes, got %d", ErrShortHeader, len(b))
	}
	sync, err := br.readBits(16)
	if err != nil {
		return Header{}, err
	}
	if uint16(sync) != SyncWord {
		return Header{}, fmt.Errorf("%w: 0x%04x", ErrSyncWord, sync)
	}

	crc1, err := br.readBits(16)
	if err != nil {
		return Header{}, err
	}
	srCode, err := br.readBits(2)
	if err != nil {
		return Header{}, err
	}
	if srCode == 3 {
		return Header{}, fmt.Errorf("%w: code 3 is reserved", ErrSampleRateCode)
	}
	frameCode, err := br.readBits(6)
	if err != nil {
		return Header{}, err
	}
	if frameCode > 37 {
		return Header{}, fmt.Errorf("%w: %d", ErrFrameSizeCode, frameCode)
	}
	bsid, err := br.readBits(5)
	if err != nil {
		return Header{}, err
	}
	if bsid > 16 {
		return Header{}, fmt.Errorf("%w: %d", ErrBitstreamID, bsid)
	}
	if bsid > 10 {
		return Header{
			SyncWord: SyncWord, CRC1: uint16(crc1), SampleRateCode: srCode,
			FrameSizeCode: frameCode, BitstreamID: bsid,
		}, fmt.Errorf("%w: bsid=%d", ErrUnsupportedEAC3, bsid)
	}

	bsmodBits, err := br.readBits(3)
	if err != nil {
		return Header{}, err
	}
	acmodBits, err := br.readBits(3)
	if err != nil {
		return Header{}, err
	}
	bsmod := BitstreamMode(bsmodBits)
	acmod := AudioCodingMode(acmodBits)

	var dsur *int
	if acmod == ACMod2_0 {
		v, err := br.readBits(2)
		if err != nil {
			return Header{}, err
		}
		dsur = &v
	} else {
		if (acmod&1 == 1) && acmod != ACMod1_0 {
			if _, err := br.readBits(2); err != nil {
				return Header{}, err
			}
		}
		if acmod&4 == 4 {
			if _, err := br.readBits(2); err != nil {
				return Header{}, err
			}
		}
	}

	lfeon, err := br.readBit()
	if err != nil {
		return Header{}, err
	}

	sampleRate := sampleRateTable[srCode]
	shift := bsid - 8
	if shift < 0 {
		shift = 0
	}
	sampleRate >>= shift
	frameSize := frameSizeWords[frameCode][srCode] * 2
	fbw := fullBandChannelCount[acmod]
	channelCount := fbw
	if lfeon {
		channelCount++
	}
	track := bsmod.trackCategory(acmod, channelCount)

	h := Header{
		SyncWord:          SyncWord,
		CRC1:              uint16(crc1),
		SampleRateCode:    srCode,
		SampleRate:        sampleRate,
		FrameSizeCode:     frameCode,
		FrameSize:         frameSize,
		BitstreamID:       bsid,
		BitstreamMode:     bsmod,
		BitstreamModeName: bsmod.String(),
		ServiceRole:       track.Role,
		ACMod:             acmod,
		ACModName:         acmod.String(),
		Layout:            channelLayout(acmod, lfeon),
		LayoutShort:       layoutShort(acmod, fbw, lfeon),
		FullBandChannels:  fbw,
		LFEOn:             lfeon,
		ChannelCount:      channelCount,
		DolbySurroundMode: dsur,
		Coupling:          Coupling{AudioBlock: 0},
	}
	h.Channels = makeChannels(acmod, lfeon, h.Coupling)
	return h, nil
}

// ParseFrame parses a complete or prefix AC-3 frame and its block-zero
// coupling metadata. A returned Frame may carry a valid Header even when the
// returned error is non-nil.
func ParseFrame(b []byte) (Frame, error) {
	br := newBitReader(b)
	header, err := parseHeader(br)
	if err != nil {
		return Frame{Header: header}, err
	}
	if len(b) < header.FrameSize {
		return Frame{Header: header}, fmt.Errorf("%w: have %d bytes, frame size %d",
			ErrFrameTruncated, len(b), header.FrameSize)
	}

	frameBytes := b[:header.FrameSize]
	br.data = frameBytes
	if err := skipBSITail(br, header.BitstreamID, header.ACMod); err != nil {
		return Frame{Header: header}, fmt.Errorf("bsi: %w", err)
	}

	coupling, err := parseBlockZeroCoupling(br, header.ACMod)
	header.Coupling = coupling
	if err != nil {
		return Frame{Header: header}, err
	}
	header.Channels = makeChannels(header.ACMod, header.LFEOn, coupling)
	residual := crc16ANSI(frameBytes[2:header.FrameSize])
	header.CRC = &CRCInfo{Checked: true, Valid: residual == 0, Residual: residual}
	return Frame{Header: header}, nil
}

func skipBSITail(br *bitReader, bsid int, acmod AudioCodingMode) error {
	iterations := 1
	if acmod == ACModDualMono {
		iterations = 2
	}

	for i := 0; i < iterations; i++ {
		if _, err := br.readBits(5); err != nil { // dialnorm
			return err
		}
		if exists, err := br.readBit(); err != nil {
			return err
		} else if exists {
			if _, err := br.readBits(8); err != nil { // compr
				return err
			}
		}
		if exists, err := br.readBit(); err != nil { // langcode e
			return err
		} else if exists {
			if _, err := br.readBits(8); err != nil { // langcod
				return err
			}
		}
		if exists, err := br.readBit(); err != nil { // audprodie
			return err
		} else if exists {
			if _, err := br.readBits(7); err != nil { // mixlevel/roomtyp
				return err
			}
		}
	}

	if _, err := br.readBit(); err != nil { // copyrightb
		return err
	}
	if _, err := br.readBit(); err != nil { // origbs
		return err
	}

	if bsid != 6 {
		for i := 0; i < 2; i++ {
			if exists, err := br.readBit(); err != nil {
				return err
			} else if exists {
				if err := br.skipBits(14); err != nil { // timecod
					return err
				}
			}
		}
	} else {
		if exists, err := br.readBit(); err != nil {
			return err
		} else if exists {
			if err := br.skipBits(14); err != nil { // preferred downmix/mix levels
				return err
			}
		}
		if exists, err := br.readBit(); err != nil {
			return err
		} else if exists {
			if err := br.skipBits(14); err != nil { // surround/headphone/xbsi2
				return err
			}
		}
	}

	if exists, err := br.readBit(); err != nil { // addbsie
		return err
	} else if exists {
		countMinusOne, err := br.readBits(6) // addbsil
		if err != nil {
			return err
		}
		if err := br.skipBits(int64(8 * (countMinusOne + 1))); err != nil {
			return err
		}
	}
	return nil
}

func parseBlockZeroCoupling(br *bitReader, acmod AudioCodingMode) (Coupling, error) {
	c := Coupling{AudioBlock: 0}
	fbw := fullBandChannelCount[acmod]

	if err := br.skipBits(int64(fbw)); err != nil { // blksw
		return c, err
	}
	if err := br.skipBits(int64(fbw)); err != nil { // dithflg
		return c, err
	}

	drcIterations := 1
	if acmod == ACModDualMono {
		drcIterations = 2
	}
	for i := 0; i < drcIterations; i++ {
		if exists, err := br.readBit(); err != nil { // dynrnge
			return c, err
		} else if exists {
			if err := br.skipBits(8); err != nil { // dynrng
				return c, err
			}
		}
	}

	strategyPresent, err := br.readBit() // cplstre[0]
	if err != nil {
		return c, err
	}
	c.StrategyPresent = strategyPresent
	if !strategyPresent {
		c.Note = "block 0 has no new coupling strategy; previous state is unavailable at stream start"
		return c, nil
	}

	inUse, err := br.readBit() // cplinu[0]
	if err != nil {
		return c, err
	}
	c.InUse = inUse
	if !inUse {
		return c, nil
	}
	if acmod < ACMod2_0 {
		c.Note = "coupling signalled in mono/dual-mono mode is invalid"
		return c, fmt.Errorf("%w: acmod=%d does not permit coupling", ErrCoupling, acmod)
	}

	labels := ac3ChannelLabels[acmod]
	for ch := 0; ch < fbw; ch++ {
		included, err := br.readBit() // chincplN
		if err != nil {
			return c, err
		}
		if included {
			c.CoupledChannels = append(c.CoupledChannels, labels[ch])
		}
	}

	if acmod == ACMod2_0 {
		phaseInUse, err := br.readBit() // phsflginu
		if err != nil {
			return c, err
		}
		c.PhaseFlagsInUse = phaseInUse
	}

	start, err := br.readBits(4) // cplbegf
	if err != nil {
		return c, err
	}
	rawEnd, err := br.readBits(4) // cplendf minus 3
	if err != nil {
		return c, err
	}
	end := rawEnd + 3
	if start >= end {
		return c, fmt.Errorf("%w: start subband %d >= end %d", ErrCoupling, start, end)
	}
	c.StartSubband = start
	c.EndSubband = end
	c.StartFrequency = 37 + 12*start
	c.EndFrequency = 37 + 12*end

	subbandCount := end - start
	c.BandStructure = make([]bool, 0, subbandCount-1)
	for i := 0; i < subbandCount-1; i++ {
		merge, err := br.readBit() // cplbndstrc
		if err != nil {
			return c, err
		}
		c.BandStructure = append(c.BandStructure, merge)
	}

	sizes := []int{12}
	for subband := 1; subband < subbandCount; subband++ {
		merge := c.BandStructure[subband-1]
		if merge {
			sizes[len(sizes)-1] += 12
		} else {
			sizes = append(sizes, 12)
		}
	}
	c.BandSizes = sizes
	c.BandCount = len(sizes)
	return c, nil
}

func crc16ANSI(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x8005
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
