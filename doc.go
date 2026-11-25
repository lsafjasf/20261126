// Package ac3select parses raw AC-3 synchronization frames and groups them
// into program segments.
//
// The parser deliberately reads only the metadata needed for program and
// channel selection: syncinfo, BSI (including bitstream mode, audio coding
// mode and LFE indication), and the first audio block's coupling strategy.
// Complete audio decoding is intentionally not performed.
//
// ParseBytes and Read never stop at a bad frame. Structural errors are added
// to Report.BadFrames, and scanning resumes at the next plausible AC-3 sync
// word or at a trusted declared frame boundary.
package ac3select
