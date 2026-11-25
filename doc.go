// Package captionhub manages caption sessions scoped to live program segments.
//
// A program segment owns a [SessionInfo] identity; timeline resets inside that
// segment are represented by monotonically increasing epochs. Cues, checkpoints,
// and per-connection [ConnectionState] values all carry the exact session epoch
// they belong to, preventing delayed old cues from being rendered after a
// program switch or EXT-X-DISCONTINUITY.
package captionhub
