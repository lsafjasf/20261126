# 20261126

## IMSC image subtitle loader (Go)

A standard-library-only Go package and CLI for validating IMSC (TTML image
profile) documents that reference multiple images and regions.

### What is validated

- `smpte:image`: `xml:id`, `smpte:imageType` (or legacy `smpte:type`),
  `smpte:encoding="Base64"`, Base64 payload, and PNG/JPEG decode + type
  match. Broken images are element errors, never usable `Image` values.
- `smpte:backgroundImage`: ordered multi-reference lists with first-decodable
  fallback; same-document `#id` fragments only (external URIs rejected).
- Reference priority per cue: inline `tt:p` -> `span` -> referenced
  `tt:region`. Unknown regions and regions without a background image are
  reported on the exact paragraph.
- `begin`/`end`: offset-time (`1.5s`, `200ms`) and clock-time
  (`HH:MM:SS[.frac]`, `HH:MM:SS:FF`). Bad timing keeps the cue, sets
  `TimingValid=false`, and records an element error.
- Fallback: any unresolvable cue gets a non-nil `Placeholder` (reason,
  detail, failed references) and exposes trimmed paragraph text as
  `FallbackText`. Empty strings are never treated as content or success.
- No images: `Load` returns the document plus `ErrNoImages` (match with
  `errors.Is`); all cues carry placeholders, so the text-track behaviour is
  explicit.

Every error points at the concrete element via a positional path and source
line, e.g. `tt[1]/body[1]/div[1]/p[2] (line 16)`.

### Usage

```go
doc, err := imsc.Load(file)
if errors.Is(err, imsc.ErrNoImages) {
    // no decodable image anywhere; render text placeholders
}
for _, cue := range doc.Cues {
    if cue.Placeholder != nil {
        // show cue.FallbackText instead of the missing image
    }
}
```

### CLI

```sh
go run ./cmd/imsc-check examples/sample.xml
```

Exit code `0` means at least one decodable image and no element errors; `1`
means validation failures or no decodable images; `2` means usage/IO errors.

### Tests

```sh
go test ./...
```
