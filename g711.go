package wavfix

// DecodeMuLaw decodes one ITU-T G.711 μ-law code to normalized float64.
func DecodeMuLaw(code byte) float64 {
	return float64(muLawToInt16(code)) / 32768.0
}

// DecodeALaw decodes one ITU-T G.711 A-law code to normalized float64.
func DecodeALaw(code byte) float64 {
	return float64(aLawToInt16(code)) / 32768.0
}

// DecodeG711 decodes interleaved G.711 bytes using the effective fmt tag.
// It is provided for analysis and float-duration checks; Repair never calls it
// and therefore cannot change the audio payload.
func DecodeG711(payload []byte, format *FormatChunk) []float64 {
	if format == nil {
		return nil
	}
	decode := DecodeMuLaw
	if format.EffectiveTag == FormatALaw {
		decode = DecodeALaw
	}
	values := make([]float64, len(payload))
	for i, code := range payload {
		values[i] = decode(code)
	}
	return values
}

// Float64Samples decodes the file's G.711 data to normalized interleaved floats.
func (f *File) Float64Samples() []float64 {
	if f == nil || f.Data == nil || f.Format == nil || !f.Format.IsG711() {
		return nil
	}
	return DecodeG711(f.Data.Payload, f.Format)
}

// FloatDuration returns the timeline duration of interleaved float samples.
func FloatDuration(floatValues, channels int, sampleRate uint32) float64 {
	if channels <= 0 || sampleRate == 0 {
		return 0
	}
	return float64(floatValues) / float64(channels) / float64(sampleRate)
}

func muLawToInt16(code byte) int16 {
	value := ^code
	sign := value & 0x80
	exponent := (value >> 4) & 0x07
	mantissa := int32(value & 0x0f)
	sample := ((mantissa << 3) + 0x84) << exponent
	sample -= 0x84
	if sign != 0 {
		sample = -sample
	}
	return int16(sample)
}

func aLawToInt16(code byte) int16 {
	value := code ^ 0x55
	sign := value & 0x80
	exponent := (value >> 4) & 0x07
	mantissa := int32(value & 0x0f)

	var sample int32
	switch exponent {
	case 0:
		sample = (mantissa << 4) + 8
	case 1:
		sample = (mantissa << 4) + 0x108
	default:
		sample = ((mantissa << 4) + 0x108) << (exponent - 1)
	}
	if sign == 0 {
		sample = -sample
	}
	return int16(sample)
}
