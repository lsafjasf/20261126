// Package cuemap builds logical-character to screen-order mappings for
// Chinese/Arabic/Japanese subtitle cues and performs deterministic ruby-aware
// horizontal or vertical layout.
//
// The package uses only the Go standard library. Test materials are generated
// by:
//
//	go generate ./...
//
//go:generate go run ./cmd/genfixtures
package cuemap
