#!/usr/bin/env bash
# Print the release tag to publish for a commit (default: HEAD), run inside the repository.
#
# - A stable tag (vX.Y.Z) already on the commit is reused, so re-running a release is idempotent.
# - Otherwise the highest stable tag gets its patch bumped (v0.1.0 -> v0.1.1).
# - With no stable tag yet, the first version is v0.1.0.
# Pre-release tags (v1.0.0-rc1) are ignored when choosing the base.
set -euo pipefail

sha="$(git rev-parse --verify "${1:-HEAD}^{commit}")"
stable='^v[0-9]+\.[0-9]+\.[0-9]+$'

# git の失敗は伝播させる（タグが取れていないのに v0.1.0 を出さない）。`|| true` は grep の「該当なし」だけに付ける。
points_at="$(git tag --points-at "$sha" --list 'v*')"
on_commit="$(grep -E "$stable" <<<"$points_at" | sort -V | tail -n 1 || true)"
if [ -n "$on_commit" ]; then
  echo "$on_commit"
  exit 0
fi

all_tags="$(git tag --list 'v*')"
latest="$(grep -E "$stable" <<<"$all_tags" | sort -V | tail -n 1 || true)"
if [ -z "$latest" ]; then
  echo "v0.1.0"
  exit 0
fi

IFS=. read -r major minor patch <<<"${latest#v}"
echo "v${major}.${minor}.$((patch + 1))"
