// Package vorbiscomment provides lenient, location-aware reading and
// conservative rewriting of Vorbis comments.
//
// Parse retains every entry in input order, including duplicate and unknown
// keys. Findings distinguish lossless structural repairs from values that must
// be confirmed by a person. Marshal never guesses at invalid UTF-8, over-limit
// values, or duplicate-key conflicts: it writes their original bytes.
package vorbiscomment
