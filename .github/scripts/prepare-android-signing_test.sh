#!/usr/bin/env bash
# Regression test for prepare-android-signing.sh. Uses real keytool and throwaway keystores made here.
set -euo pipefail

script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/prepare-android-signing.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

fixed_alias=rel
fixed_password='Fixed pass-1'
FIXED_PASSWORD="$fixed_password" keytool -genkeypair -keystore "$work/fixed.jks" -alias "$fixed_alias" \
  -keyalg RSA -keysize 2048 -validity 30 -dname "CN=hitomado test fixed" \
  -storepass:env FIXED_PASSWORD -keypass:env FIXED_PASSWORD
# `base64` の既定の折り返し（76 桁）のまま使う。Secrets に折り返し付きで貼られても通ること。
fixed_b64="$(base64 < "$work/fixed.jks")"

fail=0
ok() { echo "ok   $1"; }
ng() { echo "FAIL $1"; fail=1; }

# run_case NAME EXPECT(pass|fail) [VAR=value ...]
#   → $raw（全出力）・$out（add-mask 行を除いた出力）・$envfile・$temp を残す
run_case() {
  local name="$1" expect="$2" status=0
  shift 2
  temp="$(mktemp -d "$work/case.XXXXXX")"
  envfile="$temp/github_env"
  : > "$envfile"
  raw="$(env -i PATH="$PATH" HOME="$HOME" RUNNER_TEMP="$temp" GITHUB_ENV="$envfile" "$@" \
    bash "$script" 2>&1)" || status=$?
  # ::add-mask:: の行は、ランナーが値を伏せるための指示で、ログには出ない。それ以外の行に値が出ていないかを見る
  # （$out）。add-mask 行そのものは $raw で検査する。
  out="$(grep -v '^::add-mask::' <<< "$raw" || true)"
  if { [ "$expect" = pass ] && [ "$status" -eq 0 ]; } || { [ "$expect" = fail ] && [ "$status" -ne 0 ]; }; then
    ok "$name (exit $status)"
  else
    ng "$name: expected $expect, exit $status"
    echo "$raw"
  fi
}
# 失敗の種類を取り違えていないこと: エラーメッセージの一部を見る。
assert_error() { assert "$1: error is \"$2\"" grep -qF -e "::error::$2" <<< "$out"; }
assert_masked() { assert "$1: ::add-mask:: for the value" grep -qxF "::add-mask::$2" <<< "$raw"; }
env_value() { sed -n "s/^$1=//p" "$envfile"; }
assert() {
  local name="$1"
  shift
  if "$@"; then ok "$name"; else ng "$name"; fi
}

# 1. 固定鍵: 4 つ揃っていれば、その鍵を使う。
run_case "fixed key" pass \
  KEYSTORE_BASE64="$fixed_b64" KEYSTORE_PASSWORD="$fixed_password" KEY_ALIAS="$fixed_alias" KEY_PASSWORD="$fixed_password"
assert "fixed: keystore is the registered one" cmp -s "$temp/android-release.jks" "$work/fixed.jks"
assert "fixed: env points at keystore" test "$(env_value ANDROID_KEYSTORE_FILE)" = "$temp/android-release.jks"
assert "fixed: env alias" test "$(env_value ANDROID_KEY_ALIAS)" = "$fixed_alias"
assert "fixed: env passwords" test "$(env_value ANDROID_KEYSTORE_PASSWORD)|$(env_value ANDROID_KEY_PASSWORD)" = "$fixed_password|$fixed_password"
assert_masked "fixed (store password)" "$fixed_password"
# shellcheck disable=SC2016  # $1 $2 $3 は bash -c の引数。ここで展開させない
assert "fixed: log has no password or base64" bash -c '! grep -qF -e "$1" -e "$2" <<< "$3"' _ "$fixed_password" "$(head -n 1 <<< "$fixed_b64")" "$out"

# 2. 一部だけ設定: 使い捨て鍵に落とさず失敗する。何が足りないかを出す。
run_case "partial secrets fail" fail \
  KEYSTORE_BASE64="$fixed_b64" KEYSTORE_PASSWORD="$fixed_password"
assert_error "partial" "The fixed release key is only partly configured; missing: ANDROID_KEY_ALIAS ANDROID_KEY_PASSWORD"
assert "partial: names the missing secrets" grep -q 'missing: ANDROID_KEY_ALIAS ANDROID_KEY_PASSWORD' <<< "$out"
assert "partial: nothing exported" test ! -s "$envfile"

# 3. 4 つとも空・固定鍵を要求しない: 使い捨て鍵を作る（フォークの PR など）。
run_case "no secrets: throwaway key" pass REQUIRE_FIXED_KEY=false
assert "throwaway: alias ci" test "$(env_value ANDROID_KEY_ALIAS)" = ci
# shellcheck disable=SC2016  # $1 は bash -c の引数
assert "throwaway: keystore subject" env P="$(env_value ANDROID_KEYSTORE_PASSWORD)" bash -c \
  'keytool -list -v -keystore "$1" -storepass:env P 2>/dev/null | grep -q "CN=hitomado throwaway"' _ "$temp/android-release.jks"
assert "throwaway: passwords match each other and are set" test -n "$(env_value ANDROID_KEYSTORE_PASSWORD)" -a "$(env_value ANDROID_KEYSTORE_PASSWORD)" = "$(env_value ANDROID_KEY_PASSWORD)"
assert_masked "throwaway" "$(env_value ANDROID_KEYSTORE_PASSWORD)"
# shellcheck disable=SC2016  # $1 $2 は bash -c の引数
assert "throwaway: log has no password" bash -c '! grep -qF "$1" <<< "$2"' _ "$(env_value ANDROID_KEYSTORE_PASSWORD)" "$out"
run_case "no secrets, REQUIRE_FIXED_KEY unset: throwaway key" pass

# 4. 4 つとも空・固定鍵を要求する（リリースの公開）: 失敗する。
run_case "no secrets, fixed key required" fail REQUIRE_FIXED_KEY=true
assert_error "required" "The fixed release key is not registered"
assert "required: nothing exported" test ! -s "$envfile"
assert "required: no keystore left" test ! -e "$temp/android-release.jks"

# 5. 固定鍵を要求していて 4 つ揃っていれば通る。
run_case "fixed key, fixed key required" pass REQUIRE_FIXED_KEY=true \
  KEYSTORE_BASE64="$fixed_b64" KEYSTORE_PASSWORD="$fixed_password" KEY_ALIAS="$fixed_alias" KEY_PASSWORD="$fixed_password"

# 6. 登録の取り違え: ビルドの前に落ち、keystore を残さない。
run_case "wrong store password" fail \
  KEYSTORE_BASE64="$fixed_b64" KEYSTORE_PASSWORD=wrong KEY_ALIAS="$fixed_alias" KEY_PASSWORD=wrong
assert_error "wrong password" "The keystore cannot be opened with ANDROID_KEYSTORE_PASSWORD"
assert "wrong password: no keystore left" test ! -e "$temp/android-release.jks"
assert "wrong password: nothing exported" test ! -s "$envfile"
run_case "wrong alias" fail \
  KEYSTORE_BASE64="$fixed_b64" KEYSTORE_PASSWORD="$fixed_password" KEY_ALIAS=nope KEY_PASSWORD="$fixed_password"
assert_error "wrong alias" "The keystore cannot be opened with ANDROID_KEYSTORE_PASSWORD, or ANDROID_KEY_ALIAS is not in it"
run_case "not base64" fail \
  KEYSTORE_BASE64='***' KEYSTORE_PASSWORD="$fixed_password" KEY_ALIAS="$fixed_alias" KEY_PASSWORD="$fixed_password"
assert_error "not base64" "ANDROID_KEYSTORE_BASE64 is not valid base64 of a keystore file"
run_case "base64 of something that is not a keystore" fail \
  KEYSTORE_BASE64="$(echo hello | base64)" KEYSTORE_PASSWORD="$fixed_password" KEY_ALIAS="$fixed_alias" KEY_PASSWORD="$fixed_password"
assert_error "not a keystore" "The keystore cannot be opened with ANDROID_KEYSTORE_PASSWORD"

# 6b. 鍵のパスワードの取り違え（`keytool -list` では気づけない）。ビルドの前に落ち、keystore を残さない。
#     PKCS12（上の fixed.jks。keytool の既定）はストアと同じ値だけ。JKS は別の鍵パスワードを持てる。
run_case "PKCS12: key password differs from store password" fail \
  KEYSTORE_BASE64="$fixed_b64" KEYSTORE_PASSWORD="$fixed_password" KEY_ALIAS="$fixed_alias" KEY_PASSWORD=other-key-pass
assert_error "PKCS12 key password" "The key cannot be read with ANDROID_KEY_PASSWORD"
assert "PKCS12 key password: no keystore left" test ! -e "$temp/android-release.jks"
assert "PKCS12 key password: nothing exported" test ! -s "$envfile"

jks_store='Jks store-1'
jks_key='Jks key-2'
STORE="$jks_store" KEY="$jks_key" keytool -genkeypair -keystore "$work/legacy.jks" -storetype JKS -alias "$fixed_alias" \
  -keyalg RSA -keysize 2048 -validity 30 -dname "CN=hitomado test jks" \
  -storepass:env STORE -keypass:env KEY 2> /dev/null
jks_b64="$(base64 < "$work/legacy.jks")"
run_case "JKS: separate key password" pass \
  KEYSTORE_BASE64="$jks_b64" KEYSTORE_PASSWORD="$jks_store" KEY_ALIAS="$fixed_alias" KEY_PASSWORD="$jks_key"
assert "JKS: env key password" test "$(env_value ANDROID_KEY_PASSWORD)" = "$jks_key"
assert "JKS: no key-check file left" test ! -e "$temp/android-key-check.p12"
assert_masked "JKS (key password)" "$jks_key"
run_case "JKS: key password is the store password" fail \
  KEYSTORE_BASE64="$jks_b64" KEYSTORE_PASSWORD="$jks_store" KEY_ALIAS="$fixed_alias" KEY_PASSWORD="$jks_store"
assert_error "JKS key password" "The key cannot be read with ANDROID_KEY_PASSWORD"
assert "JKS key password: no keystore left" test ! -e "$temp/android-release.jks"
assert "JKS key password: nothing exported" test ! -s "$envfile"

# 6c. REQUIRE_FIXED_KEY の表記ゆれ: True・1 でも固定鍵を要求する。読めない値は黙って使い捨て鍵に落とさず失敗する。
for v in True TRUE 1; do
  run_case "REQUIRE_FIXED_KEY=$v, no secrets" fail REQUIRE_FIXED_KEY="$v"
  assert_error "REQUIRE_FIXED_KEY=$v" "The fixed release key is not registered"
done
for v in false False 0; do
  run_case "REQUIRE_FIXED_KEY=$v, no secrets: throwaway key" pass REQUIRE_FIXED_KEY="$v"
done
run_case "REQUIRE_FIXED_KEY=yes is rejected" fail REQUIRE_FIXED_KEY=yes
assert_error "REQUIRE_FIXED_KEY=yes" "REQUIRE_FIXED_KEY must be true, false, 1 or 0"
assert "REQUIRE_FIXED_KEY=yes: nothing exported" test ! -s "$envfile"

# 7. GITHUB_ENV へ別の変数を注入できる値は受け付けない。
run_case "newline in key password" fail \
  KEYSTORE_BASE64="$fixed_b64" KEYSTORE_PASSWORD="$fixed_password" KEY_ALIAS="$fixed_alias" KEY_PASSWORD=$'x\nANDROID_KEY_ALIAS=evil'
assert_error "newline" "ANDROID_KEY_PASSWORD contains a newline"
assert "newline: nothing exported" test ! -s "$envfile"

exit "$fail"
