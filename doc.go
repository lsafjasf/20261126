// Package subselect fixes the shared-state and time-base bugs found in
// multi-browser subtitle language switching.
//
// The model is:
//
//   - Catalog: immutable tracks for one title; safe to share between browsers.
//   - Selector: stateless resolver for language, same-language versions and
//     the missing-track fallback chain. It never stores a current selection.
//   - Player: one browser session. It owns selection generation, transport
//     state and the atomic View snapshot (track identity + rebased Schedule).
//   - Hub / httpapi: session registry and HTTP boundary for two-browser
//     end-to-end acceptance.
//
// Every Switch resolves an ordered candidate chain, loads candidates with
// fallback, rebases cue timings through the chosen track's TimeBase, and
// commits one immutable View under the player lock. A generation guard
// discards slow out-of-order switch completions.
package subselect
