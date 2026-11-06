#!/usr/bin/env bash
# Android の E2E（起動 → 一覧表示 → 返信下書き → WebView のレンダラ落ちからの復帰）。adb と UI Automator だけで画面を操作する。
# 本物の Google アカウントは使わない: E2E 用 APK（`task android:e2e:apk`。ビルドタグ e2e）は、Gmail・資格情報を
# シード済みのフェイク（アカウント 1 件・スレッド 3 件）に差し替えて起動する（internal/e2e。release には入らない）。
#
# 使い方: run.sh <e2e の apk> <applicationId>
# 環境変数:
#   ANDROID_SERIAL        対象の端末（adb の -s と同じ。端末が 1 台ならなくてよい）
#   E2E_OUT               スクリーンショット・ダンプ・logcat の出力先（既定: /tmp/hitomado-e2e-out）
#   E2E_LIST_TIMEOUT      起動後に一覧が出るまでの上限の秒（既定 600。KVM なしのエミュレーターは 1 分以上かかる）
#   E2E_STEP_TIMEOUT      その他の各手順の上限の秒（既定 300）
#   E2E_KILL_RENDERER     1 のとき、最後にレンダラのプロセスを kill -9 して復帰を確かめる（既定 1。adb root が要る＝
#                         エミュレーターの google_apis イメージ。root が取れなければこの手順だけ SKIP）
# 終了コード: 0＝すべて通った（SKIP を除く）、1＝どれかが失敗。
# docs/environment/android.md §17
set -euo pipefail

apk="${1:?usage: run.sh <apk> <applicationId>}"
app_id="${2:?usage: run.sh <apk> <applicationId>}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ui="$here/ui.py"
out="${E2E_OUT:-/tmp/hitomado-e2e-out}"
list_timeout="${E2E_LIST_TIMEOUT:-600}"
step_timeout="${E2E_STEP_TIMEOUT:-300}"
kill_renderer="${E2E_KILL_RENDERER:-1}"
activity="$app_id/com.wails.app.MainActivity"

# 件名・ラベルは internal/e2e/seed.go とフロントの文言（frontend/src/lib/messages）に合わせる。
subject="E2E seed thread 1"
reply_all_label="全員に返信"
# 返信の画面は、閉じるボタンの文言で見分ける（本文のエディタは contenteditable で、a11y では引用の文章が text になる）。
editor_label="閉じる（下書きを保存）"
# 本文のエディタには、返信では元のメールの引用が入っている。そこをタップして入力する。
editor_body_hint="が書きました"
typed_text="hello e2e draft"

adb_bin="$(command -v adb || echo "${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$HOME/Android/Sdk}}/platform-tools/adb")"
[ -x "$adb_bin" ] || { echo "adb not found (set ANDROID_HOME or put adb on PATH)" >&2; exit 1; }
adb() { "$adb_bin" "$@"; }
[ -f "$apk" ] || { echo "APK not found: $apk (build it with: wails3 task android:e2e:apk)" >&2; exit 1; }
command -v python3 >/dev/null || { echo "python3 is required (UI dump parsing)" >&2; exit 1; }
mkdir -p "$out"
rm -f "$out"/*.png "$out"/*.xml "$out"/*.txt 2>/dev/null || true

fail=0
now() { date +%s; }
log() { echo "[e2e $(date +%H:%M:%S)] $*"; }
pass() { echo "PASS $1"; }
failed() { echo "FAIL $1" >&2; fail=1; }
metric() { echo "METRIC $1=$2"; }

shot() { adb exec-out screencap -p >"$out/$1.png" 2>/dev/null || true; }
dump() { # 画面の XML を $out/ui.xml に取る（遷移中で失敗しても呼び出し側が再試行する）
  adb shell uiautomator dump /sdcard/hitomado-e2e-ui.xml >/dev/null 2>&1 || return 1
  adb exec-out cat /sdcard/hitomado-e2e-ui.xml >"$out/ui.xml" 2>/dev/null || return 1
}
# 前面に「応答なし」ダイアログ（システムアプリのもの。ソフトウェア描画のエミュレーターで出る）があれば、「待つ」を選んで閉じる。
# 現在の $out/ui.xml を見る（ダンプは呼び出し側が取る）。閉じたら 0。
dismiss_anr() {
  local xy
  "$ui" "$out/ui.xml" has "isn't responding" || return 1
  echo "WARN ANR dialog on screen; choosing 'Wait'" >&2
  xy="$("$ui" "$out/ui.xml" find "Wait")" || return 1
  # shellcheck disable=SC2086
  adb shell input tap $xy
}
# wait_for <説明> <上限の秒> <文字列>  … 画面に文字列（text か content-desc の部分一致）が出るまで待つ。
# 上限を超えたら、最後の画面（スクリーンショット・ダンプ・要素の一覧）を残して 1 を返す。
wait_for() {
  local what="$1" limit="$2" needle="$3" t0 dumps=0 dump_failures=0
  t0="$(now)"
  while :; do
    if dump; then
      dumps=$((dumps + 1))
      if "$ui" "$out/ui.xml" has "$needle"; then return 0; fi
      dismiss_anr || true
    else
      dump_failures=$((dump_failures + 1))
    fi
    if [ $(($(now) - t0)) -ge "$limit" ]; then
      shot "timeout-${what// /_}"
      cp "$out/ui.xml" "$out/timeout-${what// /_}.xml" 2>/dev/null || true
      echo "--- wait_for '$what' ('$needle') timed out after ${limit}s: $dumps dumps, $dump_failures dump failures; last screen:" >&2
      "$ui" "$out/ui.xml" texts >&2 || true
      return 1
    fi
    sleep 2
  done
}
# 画面から文字列を探して中心をタップする。ダンプは遷移・負荷で失敗することがあるので数回やり直す。
tap_text() {
  local xy
  for _ in 1 2 3 4 5; do
    if dump; then
      if xy="$("$ui" "$out/ui.xml" find "$1")"; then
        # shellcheck disable=SC2086
        adb shell input tap $xy
        return 0
      fi
      dismiss_anr || true
    fi
    sleep 2
  done
  echo "tap_text: '$1' was not found on screen:" >&2
  "$ui" "$out/ui.xml" texts >&2 || true
  return 1
}
app_pid() { adb shell pidof "$app_id" 2>/dev/null | tr -d '\r' | awk '{print $1}'; }

# ---- 準備 -----------------------------------------------------------------
log "waiting for the device"
adb wait-for-device
for _ in $(seq 1 120); do
  [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = 1 ] && break
  sleep 5
done
adb shell input keyevent KEYCODE_WAKEUP >/dev/null 2>&1 || true
adb shell wm dismiss-keyguard >/dev/null 2>&1 || true
metric device "$(adb shell getprop ro.product.model | tr -d '\r') api$(adb shell getprop ro.build.version.sdk | tr -d '\r') $(adb shell getprop ro.product.cpu.abi | tr -d '\r')"

log "installing $apk"
adb uninstall "$app_id" >/dev/null 2>&1 || true
adb install -r -t -g "$apk" >/dev/null
pass "install"

# ---- 1. 起動 --------------------------------------------------------------
adb logcat -c
adb shell am force-stop "$app_id"
log "cold start"
start_out="$(adb shell am start -W -n "$activity" | tr -d '\r')"
echo "$start_out"
total="$(sed -n 's/^TotalTime: *//p' <<<"$start_out")"
[ -n "$total" ] && metric am_start_total_ms "$total"
t_start="$(now)"
# KVM なしのエミュレーターでは am start -W が（Status: timeout で）待ち切れないことがある。起動できたかはプロセスの有無で見る。
if grep -q '^Status: ok' <<<"$start_out"; then metric am_start_status ok; else metric am_start_status "$(sed -n 's/^Status: *//p' <<<"$start_out")"; fi
pid="$(app_pid)"
[ -n "$pid" ] && pass "process is running (pid $pid)" || failed "process is running"

# ---- 2. 一覧 --------------------------------------------------------------
if wait_for "list" "$list_timeout" "$subject"; then
  metric start_to_list_s "$(($(now) - t_start))"
  pass "list shows the seeded thread ('$subject')"
  shot list
else
  failed "list shows the seeded thread ('$subject') within ${list_timeout}s"
fi
for n in 2 3; do
  "$ui" "$out/ui.xml" has "E2E seed thread $n" && pass "list shows 'E2E seed thread $n'" || failed "list shows 'E2E seed thread $n'"
done

# ---- 3. スレッドを開いて返信下書き ------------------------------------------
if tap_text "$subject" && wait_for "thread" "$step_timeout" "$reply_all_label"; then
  pass "thread opens (reply bar '$reply_all_label')"
  shot thread
  if tap_text "$reply_all_label" && wait_for "editor" "$step_timeout" "$editor_label"; then
    pass "reply editor opens"
    tap_text "$editor_body_hint" || true
    sleep 2
    adb shell input text "${typed_text// /%s}"
    sleep 1
    shot typed
    # 自動保存は編集停止の約 3 秒後。E2E のバックエンドが Gmail へ渡った下書きをファイルに残すので、それを読む。
    got=""
    t0="$(now)"
    while [ $(($(now) - t0)) -lt "$step_timeout" ]; do
      got="$(adb shell "run-as $app_id sh -c 'cat files/Hitomado/e2e/drafts/*.txt'" 2>/dev/null | tr -d '\r' || true)"
      grep -q "$typed_text" <<<"$got" && break
      sleep 3
    done
    if grep -q "$typed_text" <<<"$got"; then
      metric draft_saved_after_typing_s "$(($(now) - t0))"
      pass "draft reached Gmail (text '$typed_text')"
      grep -q 'thread: e2e-thread-1' <<<"$got" && pass "draft is attached to the thread" || failed "draft is attached to the thread"
    else
      failed "draft reached Gmail within ${step_timeout}s"
      echo "--- drafts dir ---"; adb shell "run-as $app_id ls -R files/Hitomado/e2e" 2>&1 || true
    fi
  else
    failed "reply editor opens"
  fi
else
  failed "thread opens (reply bar '$reply_all_label')"
fi

# ---- 4. レンダラ落ちからの復帰 -------------------------------------------------
if [ "$kill_renderer" = 1 ]; then
  if adb root >/dev/null 2>&1 && sleep 2 && adb wait-for-device && [ "$(adb shell id -u | tr -d '\r')" = 0 ]; then
    before="$(app_pid)"
    renderers="$(adb shell ps -A 2>/dev/null | tr -d '\r' | awk '/sandboxed_process/ {print $2}')"
    if [ -z "$renderers" ]; then
      failed "a WebView renderer process exists (sandboxed_process)"
    else
      log "killing renderer(s): $(tr "\n" " " <<<"$renderers")"
      adb logcat -c
      # shellcheck disable=SC2086
      adb shell kill -9 $renderers
      sleep 5
      after="$(app_pid)"
      if [ -n "$after" ] && [ "$after" = "$before" ]; then pass "app process survived the renderer kill (pid $after)"; else failed "app process survived the renderer kill (before=$before after=$after)"; fi
      # 作り直した WebView の a11y ツリーは更新されないことがあり（画面には一覧が出ているのにダンプには行が無い）、
      # 画面の確認は、「読み込み直した」ログとスクリーンショットの画素（空白の画面でない）で行う。a11y の一覧は参考に見る。
      t0="$(now)"
      until adb logcat -d -s WailsActivity:* | grep -q "Page loaded"; do
        [ $(($(now) - t0)) -ge "$list_timeout" ] && break
        sleep 2
      done
      if adb logcat -d -s WailsActivity:* | grep -q "Page loaded"; then pass "the app was loaded again after the renderer kill"; else failed "the app was loaded again after the renderer kill"; fi
      if adb logcat -d -s WailsActivity:* | grep -q "showing the error screen"; then failed "the error screen was not shown (recovery limit not reached)"; else pass "the error screen was not shown"; fi
      ratio=0
      t0="$(now)"
      while [ $(($(now) - t0)) -lt "$step_timeout" ]; do
        shot recovered
        ratio="$("$ui" "$out/recovered.png" inked 0.3 0.55 2>/dev/null || echo 0)"
        awk -v r="$ratio" 'BEGIN { exit !(r >= 0.02) }' && break
        sleep 3
      done
      metric recovered_list_ink_ratio "$ratio"
      if awk -v r="$ratio" 'BEGIN { exit !(r >= 0.02) }'; then pass "the list area is drawn again after the renderer kill ($(($(now) - t0))s)"; else failed "the list area is drawn again after the renderer kill (ink ratio $ratio)"; fi
      if wait_for "recovered-list" 20 "$subject"; then pass "a11y tree shows the list again too"; else echo "NOTE a11y tree of the recreated WebView does not show the list (screen pixels are checked instead)"; fi
      adb logcat -d -s WailsActivity:* | grep -E "renderer (crashed|was killed)|Recreating the WebView" || failed "recovery was logged"
    fi
  else
    echo "SKIP renderer kill (adb root is not available on this device)"
  fi
fi

# ---- 5. クラッシュが無い ----------------------------------------------------
adb logcat -d -b crash >"$out/crash.txt" 2>/dev/null || true
if grep -q "$app_id" "$out/crash.txt"; then failed "no crash in the crash log"; sed -n 1,40p "$out/crash.txt" >&2; else pass "no crash in the crash log"; fi
adb logcat -d >"$out/logcat.txt" 2>/dev/null || true
if grep -q "Render process crash wasn't handled" "$out/logcat.txt"; then failed "renderer crash was handled"; else pass "renderer crash was handled (or did not happen)"; fi
[ -n "$(app_pid)" ] && pass "process is still running at the end" || failed "process is still running at the end"

log "outputs: $out"
if [ "$fail" = 0 ]; then echo "E2E RESULT: PASS"; else echo "E2E RESULT: FAIL" >&2; fi
exit "$fail"
