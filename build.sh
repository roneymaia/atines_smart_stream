#!/usr/bin/env bash
set -euo pipefail
APP=atines-smart-stream
OUT=bin
rm -rf "$OUT" && mkdir -p "$OUT"

build() {
  local goos=$1 goarch=$2 ext=$3
  echo "==> $goos/$goarch"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -ldflags "-s -w" -o "$OUT/${APP}-${goos}-${goarch}${ext}" .
}

build linux   amd64 ""
build linux   arm64 ""
build windows amd64 ".exe"
build windows arm64 ".exe"

echo
echo "Binários em $OUT/:"
ls -la "$OUT"
echo
echo "No release, coloque ffmpeg/ffprobe (e .exe no Windows) da plataforma"
echo "ao lado de cada binário, ou garanta que estejam no PATH."
