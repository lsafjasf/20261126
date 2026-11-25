// Package aache maps AAC-HE access units to presentation time without a media
// framework.
//
// AAC core frames have a fixed duration (1024 core samples by default), while
// SBR and PS configuration may be carried in only a subset of access units.
// The configuration carried by one frame remains effective until a later frame
// changes or disables it. A timeline therefore has three distinct layers:
//
//   - core frame duration: core samples divided by the core sample rate;
//   - extension state: the SBR/PS configuration effective at each frame;
//   - actual output samples: the decoded sample count, either observed or
//     inferred from the effective extension state.
//
// BuildTimeline applies the persistent rule. Compare additionally builds the
// frame-local mapping that loses configuration on frames without an update and
// reports the first divergence, which is normally the first audible duration
// jump after an edit.
package aache
