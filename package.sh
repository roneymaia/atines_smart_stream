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

# Report exactly where things died instead of leaving an empty dist/ silently.
CURRENT="inicialização"
trap 'rc=$?; { echo; echo ">> ERRO ($rc) durante: $CURRENT"; echo ">> Veja a mensagem logo acima para a causa."; } >&2; exit $rc' ERR

need() { command -v "$1" >/dev/null 2>&1; }

# Pick a Python for zip creation (Windows packages).
PYBIN=""
for c in python3 python py; do if need "$c"; then PYBIN="$c"; break; fi; done

# goos goarch btbn-name archive-ext exe-ext
ALL_TARGETS=(
  "linux amd64 linux64 tar.xz NONE"
  "linux arm64 linuxarm64 tar.xz NONE"
  "windows amd64 win64 zip .exe"
  "windows arm64 winarm64 zip .exe"
)

want() { # want <key> [filters...]; no filters => match all
  local key="$1"; shift
  [ "$#" -eq 0 ] && return 0
  for a in "$@"; do [ "$a" = "$key" ] && return 0; done
  return 1
}

# Preflight: only require tools for the targets actually being built.
preflight() {
  CURRENT="checagem de ferramentas (preflight)"
  local miss=() need_xz=0 need_unzip=0 need_py=0
  for t in "${ALL_TARGETS[@]}"; do
    read -r goos goarch _ ext _ <<< "$t"
    want "$goos/$goarch" "$@" || continue
    [ "$ext" = "tar.xz" ] && need_xz=1
    [ "$ext" = "zip" ] && { need_unzip=1; need_py=1; }
  done
  need go   || miss+=("go        (instale o Go 1.22+ e garanta que 'go' está no PATH)")
  need curl || miss+=("curl")
  need tar  || miss+=("tar")
  [ "$need_xz" = 1 ] && { need xz || miss+=("xz        (para extrair o ffmpeg do Linux .tar.xz)"); }
  [ "$need_unzip" = 1 ] && { need unzip || miss+=("unzip     (para extrair o ffmpeg do Windows .zip)"); }
  [ "$need_py" = 1 ] && { [ -n "$PYBIN" ] || miss+=("python3   (para criar o .zip do Windows; no Windows pode ser 'python')"); }
  if [ "${#miss[@]}" -gt 0 ]; then
    echo "Faltam ferramentas para empacotar:" >&2
    printf '  - %s\n' "${miss[@]}" >&2
    echo "Instale o que falta e rode de novo." >&2
    exit 1
  fi
}

fetch_ffmpeg() { # btbn ext exe destdir
  local btbn=$1 ext=$2 exe=$3 dest=$4
  local archive="$CACHE/ffmpeg-${btbn}.${ext}"
  local url="$BASE/ffmpeg-master-latest-${btbn}-gpl.${ext}"
  if [ ! -f "$archive" ]; then
    CURRENT="download do ffmpeg ($btbn) de $url"
    echo "  fetch $url"
    curl -fL --retry 3 -o "$archive.part" "$url"
    mv "$archive.part" "$archive"
  fi
  CURRENT="extração do ffmpeg ($btbn)"
  mkdir -p "$dest"
  local tmp; tmp=$(mktemp -d)
  if [ "$ext" = "tar.xz" ]; then
    tar -xJf "$archive" -C "$tmp"
  else
    unzip -q -o "$archive" -d "$tmp"
  fi
  cp "$tmp"/*/bin/ffmpeg"$exe"  "$dest/ffmpeg$exe"
  cp "$tmp"/*/bin/ffprobe"$exe" "$dest/ffprobe$exe"
  cp "$tmp"/*/LICENSE.txt "$dest/FFMPEG-LICENSE.txt" 2>/dev/null || true
  rm -rf "$tmp"
}

preflight "$@"
mkdir -p "$CACHE"

built=0
for t in "${ALL_TARGETS[@]}"; do
  read -r goos goarch btbn ext exe <<< "$t"
  [ "$exe" = "NONE" ] && exe=""
  want "$goos/$goarch" "$@" || continue
  echo "==> $goos/$goarch"
  mkdir -p "$DIST"

  vd="$VEND/${goos}_${goarch}"
  if [ ! -f "$vd/ffmpeg$exe" ] || [ ! -f "$vd/ffprobe$exe" ]; then
    fetch_ffmpeg "$btbn" "$ext" "$exe" "$vd"
  fi

  CURRENT="compilação do binário Go para $goos/$goarch"
  echo "  build go binary"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -ldflags "-s -w" -o "$vd/${APP}${exe}" .

  CURRENT="montagem do pacote $goos/$goarch"
  stage="$DIST/${APP}-${goos}-${goarch}"
  rm -rf "$stage" && mkdir -p "$stage"
  cp "$vd/${APP}${exe}" "$vd/ffmpeg$exe" "$vd/ffprobe$exe" "$stage/"
  [ -f "$vd/FFMPEG-LICENSE.txt" ] && cp "$vd/FFMPEG-LICENSE.txt" "$stage/"
  [ -f README.md ] && cp README.md "$stage/"

  CURRENT="criação do arquivo compactado $goos/$goarch"
  echo "  archive"
  if [ "$goos" = "windows" ]; then
    ( cd "$DIST" && "$PYBIN" -m zipfile -c "${APP}-${goos}-${goarch}.zip" "${APP}-${goos}-${goarch}" )
    echo "    -> $DIST/${APP}-${goos}-${goarch}.zip"
  else
    tar -czf "$DIST/${APP}-${goos}-${goarch}.tar.gz" -C "$DIST" "${APP}-${goos}-${goarch}"
    echo "    -> $DIST/${APP}-${goos}-${goarch}.tar.gz"
  fi
  built=$((built+1))
done

CURRENT="finalização"
if [ "$built" -eq 0 ]; then
  echo "Nenhum alvo casou com '$*'. Use: linux/amd64 linux/arm64 windows/amd64 windows/arm64" >&2
  exit 1
fi
echo "Done. $built pacote(s) em $DIST/ (cada um: binário + ffmpeg + ffprobe + licenças):"
ls -1 "$DIST"/*.tar.gz "$DIST"/*.zip 2>/dev/null || true
