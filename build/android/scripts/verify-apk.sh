#!/usr/bin/env bash
# 作った APK の中身を検査する（android:verify）。CI でも同じものを使う。
# 使い方: verify-apk.sh <apk> <applicationId> <X.Y.Z> [<SDK のパス>]
# 検査: packageName・versionName・versionCode・targetSdk・権限（許可リスト）・allowBackup=false・署名。
# 署名の種類（デバッグ鍵か）は表示する。EXPECT_SIGNING=debug|release を付けると、その種類でなければ失敗にする。
set -euo pipefail

apk="${1:?usage: verify-apk.sh <apk> <applicationId> <X.Y.Z> [sdk-root]}"
app_id="${2:?}"
version="${3:?}"
sdk_root="${4:-${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$HOME/Android/Sdk}}}"

[ -f "$apk" ] || { echo "verify: $apk not found" >&2; exit 1; }

# 最新の build-tools から aapt2 / apksigner を探す（PATH にあればそれを使う）。
tool() {
  local name="$1" found
  if command -v "$name" >/dev/null 2>&1; then command -v "$name"; return; fi
  found="$(ls -d "$sdk_root"/build-tools/*/"$name" 2>/dev/null | sort -V | tail -n 1 || true)"
  [ -n "$found" ] || { echo "verify: $name not found (install Android build-tools)" >&2; exit 1; }
  echo "$found"
}
aapt2="$(tool aapt2)"
apksigner="$(tool apksigner)"

fail=0
check() { # check <説明> <条件が真なら 0>
  if "${@:2}"; then echo "OK   $1"; else echo "FAIL $1" >&2; fail=1; fi
}

IFS=. read -r major minor patch <<<"$version"
want_code=$((major * 10000000 + minor * 10000 + patch))
[ "$want_code" -ge 1 ] || want_code=1

badging="$("$aapt2" dump badging "$apk")"
package_line="$(grep -m1 '^package:' <<<"$badging")"
echo "$package_line"
grep -m1 '^minSdkVersion:' <<<"$badging" || true
grep -m1 '^targetSdkVersion:' <<<"$badging" || true
grep '^native-code:' <<<"$badging" || true

check "packageName=$app_id" grep -q "name='$app_id'" <<<"$package_line"
check "versionName=$version" grep -q "versionName='$version'" <<<"$package_line"
check "versionCode=$want_code" grep -q "versionCode='$want_code'" <<<"$package_line"
check "minSdkVersion=21" grep -q "^minSdkVersion:'21'" <<<"$badging"
check "targetSdkVersion=36" grep -q "^targetSdkVersion:'36'" <<<"$badging"

# 権限は許可リスト。増えたらここと AndroidManifest.xml を一緒に直す（意図しない権限の混入を防ぐ）。
# WAKE_LOCK・RECEIVE_BOOT_COMPLETED は WorkManager（#88）がマニフェストのマージで足す。FOREGROUND_SERVICE は使わないので許可しない（取り除く）。
# DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION は androidx.core が <applicationId>.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION を足す。
allowed_re="^(android\.permission\.(INTERNET|VIBRATE|ACCESS_NETWORK_STATE|POST_NOTIFICATIONS|WAKE_LOCK|RECEIVE_BOOT_COMPLETED)|${app_id//./\\.}\.DYNAMIC_RECEIVER_NOT_EXPORTED_PERMISSION)$"
perms="$(sed -n "s/^uses-permission: name='\([^']*\)'.*/\1/p" <<<"$badging")"
echo "permissions:"
sed 's/^/  /' <<<"$perms"
unexpected="$(grep -Ev "$allowed_re" <<<"$perms" || true)"
check "permissions are all allowed${unexpected:+ (unexpected: $(tr '\n' ' ' <<<"$unexpected"))}" test -z "$unexpected"

manifest="$("$aapt2" dump xmltree --file AndroidManifest.xml "$apk")"
check "no WailsForegroundService (foreground service is not used)" bash -c '! grep -q "WailsForegroundService" <<<"$1"' _ "$manifest"
check "allowBackup=false" grep -Eq ':allowBackup\(0x[0-9a-f]+\)=false$' <<<"$manifest"
check "dataExtractionRules is set" grep -Eq ':dataExtractionRules\(0x[0-9a-f]+\)=@' <<<"$manifest"
check "not debuggable" bash -c '! grep -q ":debuggable(" <<<"$1"' _ "$manifest"

# E2E 用ビルド（タグ e2e。Gmail・資格情報をフェイクに差し替える。internal/e2e）の印が、この APK のネイティブライブラリに無いこと（#90）。
# 印は internal/e2e.Marker の文字列。release の通常ビルド・production タグのビルドでは入らない。
e2e_marker="hitomado-e2e-backend-v1"
e2e_hits="$(unzip -p "$apk" 'lib/*/libwails.so' | grep -ac "$e2e_marker" || true)"
check "no E2E backend in libwails.so" test "${e2e_hits:-0}" = 0

# 署名: 検証して証明書の持ち主を表示する。デバッグ鍵は "CN=Android Debug"。
certs="$("$apksigner" verify --print-certs "$apk")"
subject="$(sed -n 's/^Signer #1 certificate DN: //p' <<<"$certs")"
if grep -q 'CN=Android Debug' <<<"$subject"; then
  kind=debug
  echo "SIGNING: DEBUG key ($subject). Local install only; not a release signature."
else
  kind=release
  echo "SIGNING: non-debug key ($subject)"
fi
if [ -n "${EXPECT_SIGNING:-}" ]; then
  check "signing kind is ${EXPECT_SIGNING}" test "$kind" = "$EXPECT_SIGNING"
fi

exit "$fail"
