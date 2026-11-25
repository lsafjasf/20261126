package channelmap

import (
	"math"
	"sort"
)

// Features is the content evidence extracted from one interleaved wire channel.
type Features struct {
	WireIndex           int     `json:"wire_index"`
	SampleCount         int     `json:"sample_count"`
	WindowStart         int     `json:"window_start"`
	RMS                 float64 `json:"rms"`
	Peak                float64 `json:"peak"`
	Mean                float64 `json:"mean"`
	ZeroCrossingRate    float64 `json:"zero_crossing_rate"`
	DominantFrequencyHz float64 `json:"dominant_frequency_hz,omitempty"`
	DominantMagnitude   float64 `json:"dominant_magnitude,omitempty"`
	SpectralCentroidHz  float64 `json:"spectral_centroid_hz,omitempty"`
	LowBandEnergyRatio  float64 `json:"low_band_energy_ratio,omitempty"`
	MidBandEnergyRatio  float64 `json:"mid_band_energy_ratio,omitempty"`
	HighBandEnergyRatio float64 `json:"high_band_energy_ratio,omitempty"`
	ClippingSampleRatio float64 `json:"clipping_sample_ratio"`
	Active              bool    `json:"active"`
}

const analysisWindow = 8192

// ExtractFeatures computes comparable, explainable features for every wire channel.
func ExtractFeatures(channels [][]float64, sampleRate int) []Features {
	out := make([]Features, len(channels))
	for i, samples := range channels {
		out[i] = extractOne(i, samples, sampleRate)
	}
	return out
}

func extractOne(index int, samples []float64, sampleRate int) Features {
	f := Features{WireIndex: index, SampleCount: len(samples)}
	if len(samples) == 0 || sampleRate <= 0 {
		return f
	}

	var sum, energy, clip float64
	zc := 0
	for i, v := range samples {
		sum += v
		energy += v * v
		a := math.Abs(v)
		if a > f.Peak {
			f.Peak = a
		}
		if a >= 0.999 {
			clip++
		}
		if i > 0 && ((samples[i-1] >= 0 && v < 0) || (samples[i-1] < 0 && v >= 0)) {
			zc++
		}
	}
	n := float64(len(samples))
	f.Mean = sum / n
	f.RMS = math.Sqrt(energy / n)
	f.ZeroCrossingRate = float64(zc) / n * float64(sampleRate) / 2.0
	f.ClippingSampleRatio = clip / n
	f.Active = f.RMS > 1e-4 && f.Peak > 1e-3

	start, window := loudestWindow(samples, analysisWindow)
	f.WindowStart = start
	if len(window) >= 32 {
		dom, mag, centroid, low, mid, high := spectrumFeatures(window, sampleRate)
		f.DominantFrequencyHz = round3(dom)
		f.DominantMagnitude = round6(mag)
		f.SpectralCentroidHz = round3(centroid)
		f.LowBandEnergyRatio = round6(low)
		f.MidBandEnergyRatio = round6(mid)
		f.HighBandEnergyRatio = round6(high)
	}
	return f
}

func loudestWindow(samples []float64, size int) (int, []float64) {
	if len(samples) <= size {
		return 0, samples
	}
	bestStart, bestEnergy := 0, -1.0
	for start := 0; start+size <= len(samples); start += size / 2 {
		var e float64
		for _, v := range samples[start : start+size] {
			e += v * v
		}
		if e > bestEnergy {
			bestEnergy, bestStart = e, start
		}
	}
	return bestStart, samples[bestStart : bestStart+size]
}

func spectrumFeatures(window []float64, rate int) (dominant float64, magnitude float64, centroid float64, low, mid, high float64) {
	n := nextPowerOfTwo(len(window))
	re := make([]float64, n)
	im := make([]float64, n)
	for i, v := range window {
		w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(len(window)-1))
		re[i] = v * w
	}
	fft(re, im)

	mags := make([]float64, n/2)
	total, weighted := 0.0, 0.0
	lowE, midE, highE := 0.0, 0.0, 0.0
	peakBin, peakMag := 1, 0.0
	binHz := float64(rate) / float64(n)
	for bin := 1; bin < n/2; bin++ {
		m := math.Hypot(re[bin], im[bin]) * 2 / float64(len(window))
		mags[bin] = m
		power := m * m
		hz := float64(bin) * binHz
		total += power
		weighted += hz * power
		switch {
		case hz < 120:
			lowE += power
		case hz < 2000:
			midE += power
		default:
			highE += power
		}
		if m > peakMag {
			peakMag, peakBin = m, bin
		}
	}
	if total > 0 {
		centroid = weighted / total
		low, mid, high = lowE/total, midE/total, highE/total
	}
	if peakBin > 1 && peakBin+1 < n/2 {
		a, b, c := mags[peakBin-1], mags[peakBin], mags[peakBin+1]
		denom := a - 2*b + c
		delta := 0.0
		if math.Abs(denom) > 1e-12 {
			delta = 0.5 * (a - c) / denom
			if delta < -1 || delta > 1 {
				delta = 0
			}
		}
		dominant = (float64(peakBin) + delta) * binHz
		magnitude = peakMag
	}
	return
}

func nextPowerOfTwo(v int) int {
	n := 1
	for n < v {
		n <<= 1
	}
	if n > 1<<20 {
		return 1 << 20
	}
	return n
}

// fft is an iterative radix-2 Cooley-Tukey transform. Inputs must have power-of-two length.
func fft(re, im []float64) {
	n := len(re)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		angle := -2 * math.Pi / float64(length)
		wr, wi := math.Cos(angle), math.Sin(angle)
		for start := 0; start < n; start += length {
			cr, ci := 1.0, 0.0
			half := length / 2
			for j := 0; j < half; j++ {
				a := start + j
				b := a + half
				tr := cr*re[b] - ci*im[b]
				ti := cr*im[b] + ci*re[b]
				re[b] = re[a] - tr
				im[b] = im[a] - ti
				re[a] += tr
				im[a] += ti
				ncr := cr*wr - ci*wi
				ci = cr*wi + ci*wr
				cr = ncr
			}
		}
	}
}

// Correlation returns a deterministic Pearson-like similarity in [-1,1].
// It is used for optional reference-template matching, not metadata inference.
func Correlation(a, b []float64) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n < 16 {
		return 0
	}
	a = a[:n]
	b = b[:n]
	var ma, mb float64
	for i := 0; i < n; i++ {
		ma += a[i]
		mb += b[i]
	}
	ma /= float64(n)
	mb /= float64(n)
	var num, da, db float64
	for i := 0; i < n; i++ {
		x, y := a[i]-ma, b[i]-mb
		num += x * y
		da += x * x
		db += y * y
	}
	if da <= 0 || db <= 0 {
		return 0
	}
	return num / math.Sqrt(da*db)
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }

// SortedDominantFrequencies is a small helper for reports/tests.
func SortedDominantFrequencies(features []Features) []float64 {
	out := make([]float64, 0, len(features))
	for _, f := range features {
		if f.Active {
			out = append(out, f.DominantFrequencyHz)
		}
	}
	sort.Float64s(out)
	return out
}
