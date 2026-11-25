// Package aiffmeta parses AIFF and AIFC containers without assuming a
// particular chunk order. It records every top-level chunk's offsets, parses
// COMM, SSND, COMT, MARK and the standard text chunks, streams SSND audio
// verbatim, and compares a resaved file against its source to report metadata
// loss.
//
// Repack rewrites the original chunk sequence and copies payload bytes
// unchanged. It never decodes or re-encodes audio.
package aiffmeta
