# Timeline import/export frame accuracy

This repository contains a self-contained Go reproduction and regression suite
for a timeline import/export defect:

- total sequence duration survives a round trip;
- cuts at and after every ten-minute boundary move by one frame;
- gaps, speed changes, and overlaps must all remain intact.

No external editor or media tool is used. The implementation and every test use
only the Go standard library.

## Model

`timeline.go` stores sequence and source positions as integer frame numbers.
Frame rate and clip speed are exact rational values. A clip carries:

- sequence in/out;
- source in/out;
- speed in source frames per sequence frame.

`codec.go` writes the authoritative total duration as a raw frame count and
writes every clip boundary as SMPTE timecode. `timecode.go` implements
non-drop-frame timecode for integer rates and canonical drop-frame timecode for
29.97 and 59.94.

## Minimal counterexample

`testdata/minimal-counterexample.json` is the smallest counterexample found by
the shrinker:

```text
rate           30000/1001 (29.97 drop-frame)
duration       17983 frames
track          V1
clip           00:09:59;29 -> 00:10:00;00
source         00:09:59;29 -> 00:10:00;00
```

The old exporter had an off-by-one special case for exact ten-minute
boundaries and added one frame before formatting the timecode. At the cut it
emitted `00:10:00;01`, so import produced sequence and source out at frame
17983 instead of 17982. The separately stored duration was still 17983, which
is why the total duration looked correct while the cut moved by one frame. The
same defect occurs at each further ten-minute boundary.

## Fix

`FrameToTimecode` now uses the canonical SMPTE drop-frame reinsertion formula.
The tenth minute of each ten-minute group is handled by the same formula; there
is no ad hoc `+1` group correction. The legacy encoder remains only in the test
file to prove that the counterexample is real and that the shrinker finds it.

## Tests

```sh
go test ./...
go test -run 'TestRoundTripProperty|TestMinimalCounterexampleArtifact|TestShrinkerFindsMinimalCounterexample' -v
```

The property test generates random timelines across several rates and compares
the following item by item after export/import:

- timeline duration and trailing gap;
- track and clip structure;
- sequence in/out and source in/out;
- speed;
- gaps and overlaps between adjacent clips;
- sequence-to-source and source-to-sequence probes.

The generator explicitly covers multiple tracks, gaps, variable speed, overlap,
and an exact ten-minute cut. `TestShrinkerFindsMinimalCounterexample` checks
that the shrinker reduces a multi-track fixture to the committed one-clip
counterexample. `TestLegacyEveryTenMinutesAddsOneFrame` and
`TestMinimalCounterexampleArtifact` pin both the failure mode and the fix.
