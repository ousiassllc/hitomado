#!/usr/bin/env bash
# Regression test for next-version.sh, using throwaway git repositories.
set -euo pipefail

script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/next-version.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cd "$work"
git init -q .
# 利用者の git 設定（署名・hooksPath など）に左右されないようにする。
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example.com GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example.com

fail=0
expect() {
  local name="$1" want="$2" got
  got="$(bash "$script" "${@:3}")"
  if [ "$got" = "$want" ]; then
    echo "ok   $name: $got"
  else
    echo "FAIL $name: want $want, got $got"
    fail=1
  fi
}
commit() { git commit -q --allow-empty -m "$1"; }

commit c1
expect "no tags -> first version" v0.1.0

git tag v0.1.0
commit c2
expect "patch bump" v0.1.1

git tag v0.1.1
commit c3
git tag v0.1.9
commit c4
expect "patch carries past 9, no minor rollover" v0.1.10

git tag v0.1.10
commit c5
git tag v0.2.0-rc1
git tag foo
git tag v1
expect "pre-release and non-semver tags are ignored" v0.1.11

git tag v0.2.0
commit c6
expect "highest version wins, not newest-created" v0.2.1

# Re-run on an already released commit reuses its tag instead of bumping again.
commit c7
git tag v0.2.1
expect "already tagged commit is idempotent" v0.2.1

# 同じコミットに pre-release タグしか無ければ再利用せず bump する。注釈付きタグは再利用する。
commit c8
git tag v0.2.3-rc1
expect "pre-release tag on the commit is not reused" v0.2.2
git tag -a v0.2.2 -m release
expect "annotated tag on the commit is reused" v0.2.2
commit c9
expect "explicit commit argument" v0.2.2 HEAD~1
expect "next after annotated tag" v0.2.3

[ "$fail" = 0 ]
