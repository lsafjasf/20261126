# proxyrelink

Multi-dimensional proxy/original video matcher. Built for the case where a
proxy has the **same duration** as its original but a **different first-frame
hash and audio fingerprint** (typical after re-encoding): no single signal is
decisive, so every dimension is scored, weighted and shown with its
confidence. All computation is pure Go (stdlib only).

## Evidence dimensions

| dimension | weight | confidence | notes |
|---|---|---|---|
| duration | 0.25 | 0.95 | exact / ±1 frame / ±2 frames / exponential decay |
| frame_count | 0.20 | 0.95 | exact / ±1 / ±2 / decay |
| audio_fingerprint | 0.20 | 0.80 | bit-level Hamming similarity of hex fingerprints |
| fps | 0.10 | 0.90 | must match exactly |
| first_frame_hash | 0.10 | 0.60 | low weight: re-encodes legitimately differ |
| filename | 0.10 | 0.70 | stem similarity after stripping `_proxy` etc. |
| resolution_scale | 0.05 | 0.80 | clean rational downscale (2x, 4x, ...) scores 1 |

Aggregate score = weighted mean over available dimensions; confidence =
weighted mean of per-dimension confidence. Verdicts: `auto` (score ≥ 0.85 and
confidence ≥ 0.70), `review` (≥ 0.60), `weak` otherwise.

## Input

`media.json` holds externally probed metadata (ffprobe etc.) per file:
`id`, `path`, `kind` (`proxy`/`original`), `duration_ms`, `frame_count`,
`fps_num`/`fps_den`, `width`/`height`, `first_frame_hash`,
`audio_fingerprint`, `codec`, `file_size_bytes`. See `testdata/media.json`.

## Commands

```sh
# Rank candidates, print every evidence dimension with score/weight/confidence
go run ./cmd/proxyrelink score -index testdata/media.json [-proxy p1] [-json]

# Interactive review: list / best / show / lock / reject / clear / locked
go run ./cmd/proxyrelink review -index testdata/media.json -decisions decisions.json

# Replace locked proxies on a timeline; cut points are copied verbatim
go run ./cmd/proxyrelink relink -index testdata/media.json \
    -decisions decisions.json -timeline testdata/timeline.json [-tolerance N] [-json]
```

## Lock / reject semantics

- `lock` confirms a pair; the proxy and original leave the candidate pool.
- `reject` hides a pair from future rankings; `clear` undoes either.
- A locked pair cannot be rejected (unlock first) and vice versa.
- Decisions persist to JSON after every action.

## Cut-point guarantee

`relink` never recomputes in/out points. A cut is swapped from proxy to
original only when all checks pass, otherwise it keeps the proxy and reports
an error:

- timeline fps == original fps,
- `|proxy.frame_count - original.frame_count| <= tolerance` (default 0),
- cut out-point ≤ original frame count,
- cut range is valid (`0 <= in < out`).

## Tests

```sh
go test ./...
```
