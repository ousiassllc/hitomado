#!/usr/bin/env bash
# check-elf-align.sh の回帰テスト（#205）。ホストの cc で 4KB・16KB 整列の共有ライブラリを作って検査させる。
# libwails.so の 4KB 整列（Align 0x1000）を見逃さないこと、16KB 整列を通すこと、複数ファイルの扱いを確かめる。
set -euo pipefail

script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check-elf-align.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

command -v cc >/dev/null 2>&1 || { echo "check-elf-align_test: cc not found" >&2; exit 2; }

printf 'int answer(void) { return 42; }\n' >"$work/lib.c"
cc -shared -fPIC -Wl,-z,max-page-size=4096 -Wl,-z,common-page-size=4096 -o "$work/aligned4k.so" "$work/lib.c"
cc -shared -fPIC -Wl,-z,max-page-size=16384 -Wl,-z,common-page-size=16384 -o "$work/aligned16k.so" "$work/lib.c"
printf 'not an elf\n' >"$work/garbage.so"

fail=0
expect() { # expect <名前> <期待する終了コード> <引数...>
  local name="$1" want="$2" got=0
  bash "$script" "${@:3}" >/dev/null 2>&1 || got=$?
  if [ "$got" = "$want" ]; then
    echo "ok   $name: exit $got"
  else
    echo "FAIL $name: want exit $want, got $got"
    fail=1
  fi
}

expect "16KB aligned passes" 0 "$work/aligned16k.so"
expect "4KB aligned fails" 1 "$work/aligned4k.so"
expect "one 4KB among 16KB fails" 1 "$work/aligned16k.so" "$work/aligned4k.so"
expect "not an ELF fails" 1 "$work/garbage.so"
expect "missing file is a usage error" 2 "$work/missing.so"
expect "no arguments is a usage error" 2

exit "$fail"
