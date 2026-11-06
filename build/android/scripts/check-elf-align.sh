#!/usr/bin/env bash
# ELF（.so）のすべての LOAD セグメントが 16KB（0x4000）以上で整列しているか検査する（#205）。
# 16KB ページの端末で読み込めること、2027-02-01 以降の Play の要件（docs/environment/android-play-distribution.md §6.1）のため。
# 使い方: check-elf-align.sh <.so>...   終了コード: 0 = すべて 16KB 以上 / 1 = 1 つでも未満 / 2 = 使い方・道具の誤り
# readelf は READELF（環境変数）、PATH の readelf / llvm-readelf、NDK の llvm-readelf（SDK_ROOT 配下）の順に探す。
set -euo pipefail

min_align=$((0x4000))

[ "$#" -ge 1 ] || { echo "usage: check-elf-align.sh <so>..." >&2; exit 2; }

find_readelf() {
  local name sdk found
  if [ -n "${READELF:-}" ]; then echo "$READELF"; return; fi
  for name in readelf llvm-readelf; do
    if command -v "$name" >/dev/null 2>&1; then command -v "$name"; return; fi
  done
  sdk="${SDK_ROOT:-${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$HOME/Android/Sdk}}}"
  found="$(ls -d "$sdk"/ndk/*/toolchains/llvm/prebuilt/*/bin/llvm-readelf 2>/dev/null | sort -V | tail -n 1 || true)"
  [ -n "$found" ] || { echo "check-elf-align: readelf not found (install binutils or the Android NDK)" >&2; exit 2; }
  echo "$found"
}
readelf_bin="$(find_readelf)"

status=0
for so in "$@"; do
  [ -f "$so" ] || { echo "check-elf-align: $so not found" >&2; exit 2; }
  # LOAD 行の最後の列が Align（例: "  LOAD 0x000000 0x... 0x... 0x... 0x... R E 0x4000"）。
  aligns="$("$readelf_bin" -lW "$so" 2>/dev/null | awk '$1 == "LOAD" { print $NF }' || true)"
  [ -n "$aligns" ] || { echo "FAIL $so: no LOAD segments (not an ELF shared object?)" >&2; status=1; continue; }
  bad=""
  while IFS= read -r align; do
    if [ "$((align))" -lt "$min_align" ]; then bad="$bad $align"; fi
  done <<<"$aligns"
  if [ -n "$bad" ]; then
    echo "FAIL $so: LOAD segment Align$bad < 0x4000 (16KB)" >&2
    status=1
  else
    echo "OK   $so: all LOAD segments aligned to >= 0x4000 (16KB)"
  fi
done
exit "$status"
