# BWF time-reference diagnostic

This repository contains a read-only Broadcast Wave (BWF/WAVE) diagnostic
written with the Go standard library only. It does not decode or rewrite audio.

## What it checks

- RIFF/RF64 chunk structure, declared sizes and truncation
- `fmt ` sample rate, byte rate, block alignment and frame duration
- `data` frames (or compressed `fact` sample count)
- `bext` fixed fields and 64-bit `TimeReference`
- `iXML` `START_TIME`, `TIMECODE_RATE`, DF/NDF, `TIMECODE_SAMPLE_RATE`,
  `BWF_TIME_REFERENCE`, project/scene/take metadata
- optional project JSON sidecar (`-project`) with start/duration/sample-rate

Every start declaration is normalized to `(sample, sampleRate)`. Only starts at
the same sample rate are compared. The report separates sample-level
agreement, sub-frame differences, and differences larger than one timecode
frame. `bext.TimeReference` is treated as an absolute first-sample index, not
as duration.

## Usage

```sh
go run ./cmd/bwfdiag -project testdata/timeref-conflict.project.json \
  testdata/timeref-conflict.bwf

go run ./cmd/bwfdiag -json -project testdata/timeref-conflict.project.json \
  testdata/timeref-conflict.bwf
```

Build the binary with:

```sh
go build -o artifacts/bwfdiag ./cmd/bwfdiag
```

The checked-in fixture is intentionally metadata-inconsistent: `bext` starts
384 samples after the iXML declaration, while the sidecar duration is 384
samples longer than the `data` payload. It demonstrates that the tool reports
evidence, competing explanations and undecidable provenance instead of
silently choosing one metadata field.

Regenerate the fixture with:

```sh
go run ./cmd/mkfixture -dir testdata
```

## Verification

```sh
go test ./...
go vet ./...
```
