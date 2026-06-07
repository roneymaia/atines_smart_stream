#!/usr/bin/env bash
# Builds release packages: each = the Go binary + a matching static ffmpeg/ffprobe,
# archived per platform into dist/. Downloads ffmpeg from BtbN/FFmpeg-Builds on
# first run (cached in vendor-ffmpeg/). Optionally pass targets to limit, e.g.:
#   ./package.sh                 # all four targets
#   ./package.sh linux/amd64     # just one
set -euo pipefail

APP=atines-smart-stream
BASE="https://github.com/BtbN/FFmpeg-Builds/releases/download/latest"
CACHE=".cache/ffmpeg"
VEND="vendor-ffmpeg"
DIST="dist"
mkdir -p "$CACHE" "$DIST"

# goos goarch btbn-name archive-ext exe-ext
ALL_TARGETS=(
  "linux amd64 linux64 tar.xz NONE"
  "linux arm64 linuxarm64 tar.xz NONE"
  "windows amd64 win64 zip .exe"
  "windows arm64 winarm64 zip .exe"
)

want() { # filter targets by CLI args (os/arch); empty args => all
  [ "$#" -eq 0 ] && return 0
  local key="$1"; shift
  for a in "$@"; do [ "$a" = "$key" ] && return 0; done
  return 1
}

fetch_ffmpeg() { # btbn ext exe destdir
  local btbn=$1 ext=$2 exe=$3 dest=$4
  local archive="$CACHE/ffmpeg-${btbn}.${ext}"
  local url="$BASE/ffmpeg-master-latest-${btbn}-gpl.${ext}"
  [ -f "$archive" ] || { echo "  fetch $url"; curl -fL --retry 3 -o "$archive" "$url"; }
  mkdir -p "$dest"
  local tmp; tmp=$(mktemp -d)
  if [ "$ext" = "tar.xz" ]; then
    tar -xJf "$archive" -C "$tmp"
  else
    unzip -q -o "$archive" -d "$tmp"
  fi
  cp "$tmp"/*/bin/ffmpeg"$exe"  "$dest/ffmpeg$exe"
  cp "$tmp"/*/bin/ffprobe"$exe" "$dest/ffprobe$exe"
  # carry ffmpeg's license alongside (GPL build)
  cp "$tmp"/*/LICENSE.txt "$dest/FFMPEG-LICENSE.txt" 2>/dev/null || true
  rm -rf "$tmp"
}

for t in "${ALL_TARGETS[@]}"; do
  read -r goos goarch btbn ext exe <<< "$t"
  [ "$exe" = "NONE" ] && exe=""
  want "$goos/$goarch" "$@" || continue
  echo "==> $goos/$goarch"

  vd="$VEND/${goos}_${goarch}"
  if [ ! -f "$vd/ffmpeg$exe" ] || [ ! -f "$vd/ffprobe$exe" ]; then
    fetch_ffmpeg "$btbn" "$ext" "$exe" "$vd"
  fi

  echo "  build go binary"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -ldflags "-s -w" -o "$vd/${APP}${exe}" .

  stage="$DIST/${APP}-${goos}-${goarch}"
  rm -rf "$stage" && mkdir -p "$stage"
  cp "$vd/${APP}${exe}" "$vd/ffmpeg$exe" "$vd/ffprobe$exe" "$stage/"
  [ -f "$vd/FFMPEG-LICENSE.txt" ] && cp "$vd/FFMPEG-LICENSE.txt" "$stage/"
  [ -f README.md ] && cp README.md "$stage/"

  echo "  archive"
  if [ "$goos" = "windows" ]; then
    ( cd "$DIST" && python3 -m zipfile -c "${APP}-${goos}-${goarch}.zip" "${APP}-${goos}-${goarch}" )
    echo "    -> $DIST/${APP}-${goos}-${goarch}.zip"
  else
    tar -czf "$DIST/${APP}-${goos}-${goarch}.tar.gz" -C "$DIST" "${APP}-${goos}-${goarch}"
    echo "    -> $DIST/${APP}-${goos}-${goarch}.tar.gz"
  fi
done

echo "Done. Pacotes em $DIST/ (cada um: binário + ffmpeg + ffprobe + licenças)."
