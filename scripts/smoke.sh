#!/usr/bin/env bash
# End-to-end smoke test for medref. Uses only fresh mktemp directories.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d)
PROJ="$WORK/proj"
DRIVE="$WORK/drive"
mkdir -p "$PROJ/media" "$DRIVE/Media"

go -C "$ROOT" build -o "$WORK/medref" ./cmd/medref
MEDREF="$WORK/medref"

printf x > "$PROJ/media/Clip A.MOV"
printf x > "$PROJ/media/a.mov"
# Composed é (U+00E9) on disk; NFD spelling (e + U+0301) referenced below.
printf x > "$PROJ/media/café.mov"
printf x > "$PROJ/media/한글.mov"
printf x > "$DRIVE/Media/Clip A.MOV"

cd "$PROJ"
"$MEDREF" init -root . -platform linux

# NFD "café" = cafe + combining acute (CC 81 in UTF-8).
NFD_CAFE=$(printf 'media/cafe\xcc\x81.mov')
"$MEDREF" add \
  "media/Clip A.MOV" \
  "media/clip a.mov" \
  "media/a.mov" \
  "media/a.MOV" \
  "$NFD_CAFE" \
  "media/한글.mov" \
  "./media/../media/Clip A.MOV" \
  "media/ghost.mov"

"$MEDREF" map -volume 'D:' -target "$DRIVE"
"$MEDREF" add 'D:\Media\Clip A.MOV' 'd:/media/clip a.mov'
"$MEDREF" add '\\NAS\share\b.mov'

echo "--- status before move ---"
"$MEDREF" status

NEWPROJ="$WORK/moved/proj"
mkdir -p "$NEWPROJ/media"
cp "$PROJ/media/Clip A.MOV" "$NEWPROJ/media/Clip A.MOV"
cp "$PROJ/media/a.mov" "$NEWPROJ/media/a.mov"
cp "$PROJ/media/café.mov" "$NEWPROJ/media/café.mov"
cp "$PROJ/media/한글.mov" "$NEWPROJ/media/한글.mov"
cp "$PROJ/medref.json" "$NEWPROJ/medref.json"

echo "--- relocate ---"
"$MEDREF" relocate -root "$NEWPROJ" -m "$NEWPROJ/medref.json"

echo "--- status after move ---"
"$MEDREF" status -m "$NEWPROJ/medref.json"

echo "--- json report ---"
"$MEDREF" recheck -m "$NEWPROJ/medref.json" -json

echo "SMOKE OK: $WORK"
