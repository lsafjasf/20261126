package media

// MediaInfo describes one media file (proxy or original) as probed
// externally (e.g. ffprobe) and fed to the matcher. All scoring math
// is done in Go; this struct is pure input data.
type MediaInfo struct {
	ID               string `json:"id"`
	Path             string `json:"path"`
	Kind             string `json:"kind"` // "proxy" or "original"
	DurationMs       int64  `json:"duration_ms"`
	FrameCount       int64  `json:"frame_count"`
	FPSNum           int    `json:"fps_num"`
	FPSDen           int    `json:"fps_den"`
	Width            int    `json:"width"`
	Height           int    `json:"height"`
	FirstFrameHash   string `json:"first_frame_hash"`  // hex perceptual hash of first frame
	AudioFingerprint string `json:"audio_fingerprint"` // hex chromaprint-like fingerprint
	Codec            string `json:"codec"`
	FileSizeBytes    int64  `json:"file_size_bytes"`
}

// FPS returns the frame rate as a float64.
func (m MediaInfo) FPS() float64 {
	if m.FPSDen == 0 {
		return 0
	}
	return float64(m.FPSNum) / float64(m.FPSDen)
}

// FrameDurationMs returns the duration of one frame in milliseconds.
func (m MediaInfo) FrameDurationMs() float64 {
	fps := m.FPS()
	if fps <= 0 {
		return 0
	}
	return 1000.0 / fps
}

// Index is the full media pool loaded from JSON.
type Index struct {
	Media []MediaInfo `json:"media"`
}

// Proxies returns all entries of kind "proxy".
func (ix Index) Proxies() []MediaInfo {
	var out []MediaInfo
	for _, m := range ix.Media {
		if m.Kind == "proxy" {
			out = append(out, m)
		}
	}
	return out
}

// Originals returns all entries of kind "original".
func (ix Index) Originals() []MediaInfo {
	var out []MediaInfo
	for _, m := range ix.Media {
		if m.Kind == "original" {
			out = append(out, m)
		}
	}
	return out
}

// ByID finds a media entry by ID.
func (ix Index) ByID(id string) (MediaInfo, bool) {
	for _, m := range ix.Media {
		if m.ID == id {
			return m, true
		}
	}
	return MediaInfo{}, false
}
