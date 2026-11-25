# 20261126

Repository initialized for 20261126.

## vobsub-angle

Go-native VobSub multi-angle subtitle stream selector.

- `vobsub/` — library: parses the `.idx` stream declarations (`id: <lang>, index: <n>`),
  validates `.sub` offsets (MPEG-PS pack start code + subpicture sub-stream id `0x20+n`),
  and switches angle by context number while keeping the playback position.
- `cmd/vobsub-angle/` — demo CLI.

```sh
go test ./...
go run ./cmd/vobsub-angle -idx movie.idx -sub movie.sub -angle 1 -at 00:11:02:500
```

Requesting an angle that is not declared returns a `MissingAngleError` listing the
available angles (CLI exit code 3) instead of rendering empty subtitles.
