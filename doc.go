// Package adts validates and segments concatenated ADTS audio streams.
//
// Frames are located with the 12-bit syncword and advanced by their 13-bit
// frame_length. Consecutive valid frames form a Segment while their MPEG
// version, profile, sample rate, and channel configuration stay unchanged.
// Invalid or overlapping frames are reported in Report.BadFrames and scanning
// resumes at the next complete frame.
package adts
