package logging

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
)

// Masked は伏せた値の代わりに書く文字列。
const Masked = "[REDACTED]"

// maskedEmail はメールアドレスの代わりに書く文字列。
const maskedEmail = "[email]"

// 値を無条件に伏せるキー（小文字・英数字のみに正規化したあとの部分一致）。
// 過剰に伏せる側へ倒す（誤って残すより、読みにくい方がよい）。
var sensitiveKeyParts = []string{
	// 秘密情報。
	"token", "secret", "password", "passwd", "passphrase", "apikey", "authorization", "credential", "cookie", "privatekey",
	"bearer", "oauthcode", "authcode", "verifier", "jwt", "session", "otp",
	// メールの内容（本文・件名・スニペット・宛先・添付・署名・検索語）。
	"body", "subject", "snippet", "recipient", "attachment", "prompt", "html", "content", "preview", "signature", "email",
}

// 完全一致で伏せるキー（短く、部分一致にすると関係のないキーまで巻き込むもの）。
var sensitiveKeys = map[string]bool{
	"to": true, "cc": true, "bcc": true, "from": true, "sender": true, "replyto": true,
	"address": true, "addresses": true, "filename": true, "raw": true, "text": true, "messagetext": true,
	"pwd": true, "pass": true, "key": true, "q": true, "query": true, "title": true, "name": true,
	"displayname": true, "fromaddress": true, "toaddress": true,
}

func isSensitiveKey(key string) bool {
	var b strings.Builder
	for _, r := range strings.ToLower(key) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	k := b.String()
	if sensitiveKeys[k] {
		return true
	}
	for _, p := range sensitiveKeyParts {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}

// 文字列の中の秘密の形を伏せるパターン。キーでは判別できない場所（エラー文・URL・ログメッセージ）に混ざった値を拾う。
var scrubRules = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`(?i)\b(authorization\s*[:=]\s*)(?:bearer|basic|token|digest)\s+[A-Za-z0-9._~+/=%-]+`), "${1}" + Masked},
	{regexp.MustCompile(`(?i)\bbearer(?:\s+|%20)[A-Za-z0-9._~+/=%-]+`), "Bearer " + Masked},
	{regexp.MustCompile(`ya29\.[A-Za-z0-9._-]+`), Masked},                               // Google のアクセストークン
	{regexp.MustCompile(`\b1(?://|%2F%2F)[A-Za-z0-9._-]{20,}`), Masked},                 // Google のリフレッシュトークン
	{regexp.MustCompile(`GOCSPX-[A-Za-z0-9_-]+`), Masked},                               // Google の Client Secret
	{regexp.MustCompile(`AIza[A-Za-z0-9_-]{30,}`), Masked},                              // Google API キー
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}`), Masked},                             // AI の API キー（OpenAI・Anthropic）
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`), Masked}, // JWT（id_token）
	{regexp.MustCompile(`\bhmcp_[A-Za-z0-9_-]+`), Masked},                               // Hitomado の MCP トークン
	// key=value / key: value / key%3Dvalue。キーが「…_token」「x-auth-token」のように前に文字が付いていても拾う。
	{regexp.MustCompile(`(?i)([A-Za-z0-9_.-]*(?:token|secret|passw(?:or)?d|passphrase|api[_-]?key|access[_-]?key|secret[_-]?key|verifier|credential|session|otp|jwt)[A-Za-z0-9_-]*\s*(?:=|:|%3D|%3A)\s*)(?:"[^"]*"|'[^']*'|[^&\s"',;%]+)`), "${1}" + Masked},
	// 引用符つきの JSON のキー: "accessToken":"…"。
	{regexp.MustCompile(`(?i)("[A-Za-z0-9_.-]*(?:token|secret|passw(?:or)?d|passphrase|api[_-]?key|access[_-]?key|verifier|credential|session|authorization|jwt|otp)[A-Za-z0-9_.-]*"\s*:\s*)"[^"]*"`), "${1}\"" + Masked + "\""},
	// URL のクエリの code / state / key。
	{regexp.MustCompile(`(?i)([?&](?:code|state|key|code_verifier)(?:=|%3D))[^&\s"']+`), "${1}" + Masked},
	{regexp.MustCompile(`[\p{L}\p{N}._%+-]+@[\p{L}\p{N}-]+(?:\.[\p{L}\p{N}-]+)*\.\p{L}{2,}`), maskedEmail},
}

// Scrub は s の中の秘密の形（トークン・キー・メールアドレス）を伏せて返す。
func Scrub(s string) string {
	s = scrubOnce(s)
	// URL エンコードされた形（access_token%3D…・alice%40example.com）は、デコードした文字列でも調べる。
	// デコード後に何か伏せたときだけ、デコード後の文字列を採る（無関係な % を壊さない）。
	if strings.Contains(s, "%") {
		if dec, err := url.QueryUnescape(s); err == nil && dec != s {
			if scrubbed := scrubOnce(dec); scrubbed != dec {
				return scrubbed
			}
		}
	}
	return s
}

func scrubOnce(s string) string {
	for _, r := range scrubRules {
		s = r.re.ReplaceAllString(s, r.with)
	}
	return s
}

// Redact は h に渡るレコードの値を伏せる Handler を返す。
// 秘密・メール内容のキーの値は丸ごと伏せ、それ以外の文字列も Scrub を通す。
func Redact(h slog.Handler) slog.Handler { return &redactHandler{inner: h} }

type redactHandler struct {
	inner  slog.Handler
	masked bool // 秘密・メール内容の名前のグループの内側（WithGroup）
}

func (r *redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return r.inner.Enabled(ctx, l)
}

func (r *redactHandler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, Scrub(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a, r.masked))
		return true
	})
	return r.inner.Handle(ctx, out)
}

func (r *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	red := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		red[i] = redactAttr(a, r.masked)
	}
	return &redactHandler{inner: r.inner.WithAttrs(red), masked: r.masked}
}

func (r *redactHandler) WithGroup(name string) slog.Handler {
	if r.masked || isSensitiveKey(name) {
		// 秘密・メール内容の名前のグループは、以後の属性を丸ごと伏せる。
		return &redactHandler{inner: r.inner.WithGroup(name), masked: true}
	}
	return &redactHandler{inner: r.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr, masked bool) slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		childMasked := masked || isSensitiveKey(a.Key)
		red := make([]slog.Attr, len(group))
		for i, g := range group {
			red[i] = redactAttr(g, childMasked)
		}
		a.Value = slog.GroupValue(red...)
		return a
	}
	if masked || isSensitiveKey(a.Key) {
		a.Value = slog.StringValue(Masked)
		return a
	}
	switch a.Value.Kind() {
	case slog.KindString:
		a.Value = slog.StringValue(Scrub(a.Value.String()))
	case slog.KindAny:
		a.Value = slog.StringValue(scrubAny(a.Value.Any()))
	default: // 数値・真偽値・時間・時刻は秘密を運べない
	}
	return a
}

// scrubAny は error・Stringer だけ文字列にして Scrub する。構造体・マップ・スライスなどは中身を出さず型名だけにする。
// String／Error の panic（nil ポインタなど）は型名に落とす（ログ呼び出しで呼び出し元を落とさない）。
func scrubAny(v any) (out string) {
	defer func() {
		if recover() != nil {
			out = fmt.Sprintf("<%T>", v)
		}
	}()
	switch x := v.(type) {
	case nil:
		return "<nil>"
	case error:
		return Scrub(x.Error())
	case fmt.Stringer:
		return Scrub(x.String())
	default:
		return fmt.Sprintf("<%T>", v)
	}
}
