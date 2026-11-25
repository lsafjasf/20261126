package edlframe

import (
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// Rate is an exact rational frame rate.
type Rate struct {
	Numerator   int64
	Denominator int64
}

// Timebase combines the real frame rate with the timecode label base.
type Timebase struct {
	Rate          Rate
	TimecodeBase  int
	DropFrame     bool
	CanonicalSpec string
}

var decimalRatePattern = regexp.MustCompile(`^\d+\.\d+$`)

// ParseTimebase accepts common EDL declarations such as "24", "25",
// "29.97DF", "30000/1001", and an explicit label base "12.5@25".
func ParseTimebase(spec string) (Timebase, error) {
	original := strings.TrimSpace(spec)
	if original == "" {
		return Timebase{}, fmt.Errorf("empty frame rate")
	}

	normalized := strings.ToUpper(original)
	drop := false
	switch {
	case strings.HasSuffix(normalized, "NDF"):
		normalized = strings.TrimSpace(normalized[:len(normalized)-3])
	case strings.HasSuffix(normalized, "DF"):
		drop = true
		normalized = strings.TrimSpace(normalized[:len(normalized)-2])
	}

	rateSpec := normalized
	base := 0
	if at := strings.IndexByte(normalized, '@'); at >= 0 {
		rateSpec = strings.TrimSpace(normalized[:at])
		baseSpec := strings.TrimSpace(normalized[at+1:])
		parsedBase, err := strconv.Atoi(baseSpec)
		if err != nil || parsedBase <= 0 {
			return Timebase{}, fmt.Errorf("invalid timecode base %q", baseSpec)
		}
		base = parsedBase
	}

	rate, err := parseRate(rateSpec)
	if err != nil {
		return Timebase{}, err
	}
	if base == 0 {
		base = inferTimecodeBase(rate)
	}
	if base <= 0 {
		return Timebase{}, fmt.Errorf("cannot infer timecode base for %q", original)
	}
	if drop && base != 30 && base != 60 {
		return Timebase{}, fmt.Errorf("drop-frame timecode is supported only for 30/60 label bases, got %d", base)
	}

	tb := Timebase{Rate: rate, TimecodeBase: base, DropFrame: drop}
	tb.CanonicalSpec = tb.String()
	return tb, nil
}

func parseRate(spec string) (Rate, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return Rate{}, fmt.Errorf("empty frame rate")
	}

	if strings.Contains(spec, "/") {
		parts := strings.Split(spec, "/")
		if len(parts) != 2 {
			return Rate{}, fmt.Errorf("invalid rational frame rate %q", spec)
		}
		num, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil || num <= 0 {
			return Rate{}, fmt.Errorf("invalid frame-rate numerator %q", spec)
		}
		den, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || den <= 0 {
			return Rate{}, fmt.Errorf("invalid frame-rate denominator %q", spec)
		}
		return Rate{Numerator: num, Denominator: den}, nil
	}

	if decimalRatePattern.MatchString(spec) {
		switch spec {
		case "23.976", "23.98":
			return Rate{24000, 1001}, nil
		case "29.97":
			return Rate{30000, 1001}, nil
		case "59.94":
			return Rate{60000, 1001}, nil
		}
		r, ok := new(big.Rat).SetString(spec)
		if !ok {
			return Rate{}, fmt.Errorf("invalid decimal frame rate %q", spec)
		}
		return Rate{Numerator: r.Num().Int64(), Denominator: r.Denom().Int64()}, nil
	}

	fps, err := strconv.ParseInt(spec, 10, 64)
	if err != nil || fps <= 0 {
		return Rate{}, fmt.Errorf("invalid frame rate %q", spec)
	}
	return Rate{Numerator: fps, Denominator: 1}, nil
}

func inferTimecodeBase(rate Rate) int {
	switch {
	case rate.Denominator == 1:
		return int(rate.Numerator)
	case rate.Numerator == 24000 && rate.Denominator == 1001:
		return 24
	case rate.Numerator == 30000 && rate.Denominator == 1001:
		return 30
	case rate.Numerator == 60000 && rate.Denominator == 1001:
		return 60
	default:
		rounded := (rate.Numerator*2 + rate.Denominator) / (2 * rate.Denominator)
		return int(rounded)
	}
}

// Equal reports whether two rational rates represent the same value.
func (r Rate) Equal(other Rate) bool {
	return r.Numerator*other.Denominator == other.Numerator*r.Denominator
}

func (r Rate) ratioString() string {
	if r.Denominator == 1 {
		return strconv.FormatInt(r.Numerator, 10)
	}
	return fmt.Sprintf("%d/%d", r.Numerator, r.Denominator)
}

// String returns a compact canonical declaration.
func (tb Timebase) String() string {
	s := tb.Rate.ratioString()
	if tb.TimecodeBase != inferTimecodeBase(tb.Rate) {
		s += "@" + strconv.Itoa(tb.TimecodeBase)
	}
	if tb.DropFrame {
		s += "DF"
	}
	return s
}

// FramesPerDay is the timecode wrap boundary for this label format.
func (tb Timebase) FramesPerDay() int64 {
	base := int64(tb.TimecodeBase)
	if !tb.DropFrame {
		return base * 24 * 60 * 60
	}
	drop := int64(dropFrameIncrement(tb.TimecodeBase))
	dropMinutes := int64(24*60 - 24*60/10)
	return base*24*60*60 - drop*dropMinutes
}

func dropFrameIncrement(base int) int {
	return base / 15
}

// ScaleFrames converts a frame count/duration by preserving wall-clock time.
// Non-integer results use round-half-up. Inputs are normally non-negative.
func ScaleFrames(frames int64, from, to Timebase) int64 {
	if frames == 0 {
		return 0
	}
	num := new(big.Rat).SetInt64(frames)
	ratio := new(big.Rat).SetFrac(
		big.NewInt(to.Rate.Numerator).Mul(big.NewInt(to.Rate.Numerator), big.NewInt(from.Rate.Denominator)),
		big.NewInt(from.Rate.Numerator).Mul(big.NewInt(from.Rate.Numerator), big.NewInt(to.Rate.Denominator)),
	)
	num.Mul(num, ratio)

	n := num.Num()
	d := num.Denom()
	absN := new(big.Int).Abs(n)
	q, rem := new(big.Int).QuoRem(absN, d, new(big.Int))
	twiceRem := new(big.Int).Lsh(rem, 1)
	if twiceRem.Cmp(d) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	result := q.Int64()
	if n.Sign() < 0 {
		return -result
	}
	return result
}
