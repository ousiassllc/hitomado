// Package callbackpage は、ブラウザの認可（OAuth・Sign in with ChatGPT）の 127.0.0.1 待受が返す完了ページを提供する。
// 外部リソースを使えない（CSP が default-src 'none'）ため、スタイルはページに直接埋め込む。
package callbackpage

import (
	"fmt"
	"html"
	"net/http"
)

// page は完了ページの HTML。色・フォント・サイズ・行間・角丸・余白は Ousiass デザインガイドのトークン（frontend/src/styles/tokens.css）の値の複製で、
// 同期は callbackpage_test.go の TestPageValuesMatchTokens が tokens.css を読んで検証する（トークンを変えたらここも直す）。
// 色は :root の変数に置き、ダークは prefers-color-scheme で切り替える。フォントは --font-sans の Sora + Noto Sans JP を端末のフォント名
// （"Sora Variable" は同梱した @font-face の名前なので使えない）で指定する。CSP が外部リソースを禁じるため同梱はしない。
// サイズ・行間はガイドのスケール（見出し 18px/1.7・本文 16px/1.8）。
const page = `<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><title>Hitomado</title>
<style>
:root{--ink:#050505;--bone:#f5f5f4;--surface:#ffffff;--border:#e7e7e5}
@media (prefers-color-scheme:dark){:root{--ink:#ffffff;--bone:#050505;--surface:#141414;--border:#2a2a2a}}
body{font-family:Sora,"Noto Sans JP",sans-serif;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0;background:var(--bone);color:var(--ink)}
main{background:var(--surface);padding:24px;border-radius:12px;border:1px solid var(--border);max-width:28rem}
h1{font-size:18px;line-height:1.7;font-weight:600;margin:0 0 12px}
p{margin:0;font-size:16px;line-height:1.8}
</style>
</head><body><main><h1>Hitomado</h1><p>%s</p></main></body></html>
`

// Write は msg を載せた完了ページを w に書く。msg は HTML エスケープする。
func Write(w http.ResponseWriter, msg string) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Connection", "close")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	// 書き込み失敗（クライアントの切断）は認可結果の受け取りに影響しない。
	_, _ = fmt.Fprintf(w, page, html.EscapeString(msg))
}
