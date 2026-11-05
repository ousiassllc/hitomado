// Package callbackpage は、ブラウザの認可（OAuth・Sign in with ChatGPT）の 127.0.0.1 待受が返す完了ページを提供する。
// 外部リソースを使えない（CSP が default-src 'none'）ため、スタイルはページに直接埋め込む。
package callbackpage

import (
	"fmt"
	"html"
	"net/http"
)

// page は完了ページの HTML。色は Ousiass デザインガイドのコアトークン（frontend/src/styles/tokens.css と同じ値）を
// :root の変数に置き、ダークは prefers-color-scheme で切り替える。
const page = `<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><title>Hitomado</title>
<style>
:root{--ink:#050505;--bone:#f5f5f4;--surface:#fff;--border:#e7e7e5}
@media (prefers-color-scheme:dark){:root{--ink:#fff;--bone:#050505;--surface:#141414;--border:#2a2a2a}}
body{font-family:sans-serif;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0;background:var(--bone);color:var(--ink)}
main{background:var(--surface);padding:2rem 2.5rem;border-radius:8px;border:1px solid var(--border);box-shadow:0 1px 4px rgba(0,0,0,.2);max-width:28rem}
h1{font-size:1.2rem;margin:0 0 .75rem}
p{margin:0;line-height:1.6}
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
