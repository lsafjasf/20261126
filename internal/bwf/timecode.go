package bwf

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

func ParseRate(s string) (Rate, error) {
	orig := strings.TrimSpace(s)
	v := strings.ToUpper(orig)
	v = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(v, "DF"), "NDF"))
	if v == "" {
		return Rate{}, fmt.Errorf("timecode rate is empty")
	}
	if strings.Contains(v, "/") {
		parts := strings.Split(v, "/")
		if len(parts) != 2 {
			return Rate{}, fmt.Errorf("invalid rate %q", orig)
		}
		num, err1 := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		den, err2 := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err1 != nil || err2 != nil || num <= 0 || den <= 0 {
			return Rate{}, fmt.Errorf("invalid rate %q", orig)
		}
		return Rate{Num: num, Den: den, Text: orig}, nil
	}
	switch v {
	case "23.976", "23.98":
		return Rate{Num: 24000, Den: 1001, Text: orig}, nil
	case "29.97":
		return Rate{Num: 30000, Den: 1001, Text: orig}, nil
	case "59.94":
		return Rate{Num: 60000, Den: 1001, Text: orig}, nil
	case "119.88":
		return Rate{Num: 120000, Den: 1001, Text: orig}, nil
	}
	value, err := strconv.ParseFloat(v, 64)
	if err != nil || value <= 0 {
		return Rate{}, fmt.Errorf("invalid rate %q", orig)
	}
	whole := int64(value)
	if float64(whole) == value {
		return Rate{Num: whole, Den: 1, Text: orig}, nil
	}
	return Rate{}, fmt.Errorf("non-integer rate %q requires an exact rational", orig)
}

func (r Rate) NominalFPS() int64 {
	if r.Num <= 0 || r.Den <= 0 {
		return 0
	}
	q := r.Num / r.Den
	if r.Num%r.Den != 0 {
		q++
	}
	return q
}

func (r Rate) ActualFPS() float64 {
	if r.Den == 0 {
		return 0
	}
	return float64(r.Num) / float64(r.Den)
}

func ParseTimecode(s, rateText string) (Timecode, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return Timecode{}, fmt.Errorf("timecode is empty")
	}
	rate, err := ParseRate(rateText)
	if err != nil {
		return Timecode{}, err
	}
	drop := strings.Contains(v, ";")
	if strings.Count(v, ";") > 1 {
		return Timecode{}, fmt.Errorf("timecode %q has too many semicolons", s)
	}
	parts := strings.Split(strings.ReplaceAll(v, ";", ":"), ":")
	if len(parts) != 4 {
		return Timecode{}, fmt.Errorf("timecode %q must have four fields", s)
	}
	nums := make([]int64, 4)
	for i, p := range parts {
		n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil || n < 0 {
			return Timecode{}, fmt.Errorf("invalid timecode field %q in %q", p, s)
		}
		nums[i] = n
	}
	tc := Timecode{
		Text: v, Hours: nums[0], Minutes: nums[1],
		Seconds: nums[2], Frames: nums[3], DropFrame: drop,
	}
	nominal := rate.NominalFPS()
	if tc.Minutes >= 60 || tc.Seconds >= 60 || tc.Frames >= nominal {
		return Timecode{}, fmt.Errorf("timecode %q is out of range for %s fps", s, rateText)
	}
	if drop {
		dropCount := int64(0)
		switch {
		case rate.Num == 30000 && rate.Den == 1001:
			dropCount = 2
		case rate.Num == 60000 && rate.Den == 1001:
			dropCount = 4
		default:
			return Timecode{}, fmt.Errorf("drop-frame timecode is invalid at rate %s", rateText)
		}
		if tc.Minutes%10 != 0 && tc.Seconds == 0 && tc.Frames < dropCount {
			return Timecode{}, fmt.Errorf("timecode %q names a dropped frame", s)
		}
	}
	return tc, nil
}

func (tc Timecode) FrameNumber(rate Rate) (int64, error) {
	if rate.Num <= 0 || rate.Den <= 0 {
		return 0, fmt.Errorf("invalid rate")
	}
	nominal := rate.NominalFPS()
	if tc.Frames < 0 || tc.Frames >= nominal {
		return 0, fmt.Errorf("frame field %d outside 0..%d", tc.Frames, nominal-1)
	}
	total := ((tc.Hours*3600+tc.Minutes*60+tc.Seconds)*nominal + tc.Frames)
	if !tc.DropFrame {
		return total, nil
	}
	drop := int64(0)
	switch {
	case rate.Num == 30000 && rate.Den == 1001:
		drop = 2
	case rate.Num == 60000 && rate.Den == 1001:
		drop = 4
	default:
		return 0, fmt.Errorf("drop-frame timecode requires a 29.97/59.94 rate")
	}
	if tc.Minutes%10 != 0 && tc.Seconds == 0 && tc.Frames < drop {
		return 0, fmt.Errorf("timecode names a dropped frame")
	}
	totalMinutes := tc.Hours*60 + tc.Minutes
	return total - drop*totalMinutes + drop*(totalMinutes/10), nil
}

func TimecodeSample(tc Timecode, rate Rate, sampleRate uint32) (int64, error) {
	frame, err := tc.FrameNumber(rate)
	if err != nil {
		return 0, err
	}
	return frameToSamples(frame, rate, sampleRate)
}

func frameToSamples(frame int64, rate Rate, sampleRate uint32) (int64, error) {
	if frame < 0 || rate.Num <= 0 || rate.Den <= 0 || sampleRate == 0 {
		return 0, fmt.Errorf("cannot map negative/invalid frame to samples")
	}
	num := big.NewInt(frame)
	num.Mul(num, big.NewInt(rate.Den))
	num.Mul(num, big.NewInt(int64(sampleRate)))
	den := big.NewInt(rate.Num)
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	r.Mul(r, big.NewInt(2))
	if r.Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() || q.Sign() < 0 {
		return 0, fmt.Errorf("sample position overflows int64")
	}
	return q.Int64(), nil
}

func FrameSamples(rate Rate, sampleRate uint32) float64 {
	if rate.Num <= 0 {
		return 0
	}
	return float64(rate.Den) * float64(sampleRate) / float64(rate.Num)
}
