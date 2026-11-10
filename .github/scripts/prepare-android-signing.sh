#!/usr/bin/env bash
# release APK の署名鍵を決め、build/android/app/build.gradle が読む環境変数 4 つを $GITHUB_ENV へ書く。
# .github/actions/build-android-apk が呼ぶ。テストは prepare-android-signing_test.sh。
#
# 入力（環境変数。値はログに出さない）:
#   KEYSTORE_BASE64    keystore（.jks）を base64 にしたもの（Secrets ANDROID_KEYSTORE_BASE64）
#   KEYSTORE_PASSWORD  keystore のパスワード（Secrets ANDROID_KEYSTORE_PASSWORD）
#   KEY_ALIAS          鍵のエイリアス（Secrets ANDROID_KEY_ALIAS）
#   KEY_PASSWORD       鍵のパスワード（Secrets ANDROID_KEY_PASSWORD）
#   REQUIRE_FIXED_KEY  "true" のとき、固定鍵が無ければ使い捨て鍵に落とさず失敗する（リリースの公開用）
#
# 動き:
#   4 つとも設定あり … 固定鍵を使う（base64 を復号して keytool で開けることを確認する）
#   4 つとも空       … REQUIRE_FIXED_KEY=true なら失敗、それ以外は使い捨て鍵を作る（フォークの PR など）
#   一部だけ設定     … 登録ミスなので失敗する（使い捨て鍵に黙って落とさない）
set -euo pipefail

: "${GITHUB_ENV:?GITHUB_ENV is not set}"
: "${RUNNER_TEMP:?RUNNER_TEMP is not set}"

b64="${KEYSTORE_BASE64:-}"
store_password="${KEYSTORE_PASSWORD:-}"
alias_name="${KEY_ALIAS:-}"
key_password="${KEY_PASSWORD:-}"
require_fixed="${REQUIRE_FIXED_KEY:-false}"

keystore="${RUNNER_TEMP}/android-release.jks"

# GITHUB_ENV は 1 行 1 変数。改行や CR が入ると別の変数を注入できてしまうので受け付けない。
reject_newline() {
  local name="$1" value="$2"
  case "$value" in
    *$'\n'* | *$'\r'*) echo "::error::${name} contains a newline" >&2; exit 1 ;;
  esac
}

if [ -n "$b64$store_password$alias_name$key_password" ]; then
  missing=()
  [ -n "$b64" ] || missing+=(ANDROID_KEYSTORE_BASE64)
  [ -n "$store_password" ] || missing+=(ANDROID_KEYSTORE_PASSWORD)
  [ -n "$alias_name" ] || missing+=(ANDROID_KEY_ALIAS)
  [ -n "$key_password" ] || missing+=(ANDROID_KEY_PASSWORD)
  if [ "${#missing[@]}" -gt 0 ]; then
    echo "::error::The fixed release key is only partly configured; missing: ${missing[*]}" >&2
    exit 1
  fi

  reject_newline ANDROID_KEYSTORE_PASSWORD "$store_password"
  reject_newline ANDROID_KEY_ALIAS "$alias_name"
  reject_newline ANDROID_KEY_PASSWORD "$key_password"
  echo "::add-mask::${store_password}"
  echo "::add-mask::${key_password}"

  # base64 の途中の空白・改行（`base64` が折り返した出力をそのまま貼った場合）は無視する。
  if ! printf '%s' "$b64" | tr -d ' \t\r\n' | base64 -d > "$keystore" 2> /dev/null || [ ! -s "$keystore" ]; then
    rm -f "$keystore"
    echo "::error::ANDROID_KEYSTORE_BASE64 is not valid base64 of a keystore file" >&2
    exit 1
  fi
  # パスワードは環境変数で渡す（コマンドラインに載せない）。値を取り違えていたら、ビルドの前にここで落ちる。
  if ! PASSWORD="$store_password" keytool -list -keystore "$keystore" -alias "$alias_name" \
    -storepass:env PASSWORD > /dev/null 2>&1; then
    rm -f "$keystore"
    echo "::error::The keystore cannot be opened with ANDROID_KEYSTORE_PASSWORD, or ANDROID_KEY_ALIAS is not in it" >&2
    exit 1
  fi
  echo "::notice::Signing with the fixed release key from Secrets (alias: ${alias_name})"
else
  if [ "$require_fixed" = "true" ]; then
    echo "::error::The fixed release key is not registered, so the release APK is not built with a throwaway key. Register the Secrets listed in docs/environment/android.md section 4.1 (ANDROID_KEYSTORE_BASE64, ANDROID_KEYSTORE_PASSWORD, ANDROID_KEY_ALIAS, ANDROID_KEY_PASSWORD) and re-run." >&2
    exit 1
  fi
  alias_name=ci
  store_password="$(openssl rand -hex 16)"
  key_password="$store_password"
  echo "::add-mask::${store_password}"
  STORE_PASSWORD="$store_password" keytool -genkeypair -keystore "$keystore" -alias "$alias_name" \
    -keyalg RSA -keysize 2048 -validity 3650 -dname "CN=hitomado throwaway" \
    -storepass:env STORE_PASSWORD -keypass:env STORE_PASSWORD
  echo "::warning::Signing with a throwaway key: this APK cannot update an APK signed with another key."
fi

{
  echo "ANDROID_KEYSTORE_FILE=${keystore}"
  echo "ANDROID_KEY_ALIAS=${alias_name}"
  echo "ANDROID_KEYSTORE_PASSWORD=${store_password}"
  echo "ANDROID_KEY_PASSWORD=${key_password}"
} >> "$GITHUB_ENV"
