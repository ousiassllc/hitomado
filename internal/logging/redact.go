package logging

import (
	"context"
	"fmt"
	"log/slog"
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
	"token", "secret", "password", "passwd", "apikey", "authorization", "credential", "cookie", "privatekey", "bearer",
	"oauthcode", "authcode",
	// メールの内容（本文・件名・スニペット・宛先・添付）。
	"body", "subject", "snippet", "recipient", "attachment", "prompt",
}

// 完全一致で伏せるキー（短く、部分一致にすると関係のないキーまで巻き込むもの）。
var sensitiveKeys = map[string]bool{
	"to": true, "cc": true, "bcc": true, "from": true, "sender": true, "replyto": true,
	"email": true, "address": true, "addresses": true, "filename": true,
	"raw": true, "html": true, "text": true, "content": true,
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
	{regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`), "Bearer " + Masked},
	{regexp.MustCompile(`ya29\.[A-Za-z0-9._-]+`), Masked},                               // Google のアクセストークン
	{regexp.MustCompile(`\b1//[A-Za-z0-9._-]{20,}`), Masked},                            // Google のリフレッシュトークン
	{regexp.MustCompile(`GOCSPX-[A-Za-z0-9_-]+`), Masked},                               // Google の Client Secret
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}`), Masked},                             // AI の API キー（OpenAI・Anthropic）
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`), Masked}, // JWT（id_token）
	{regexp.MustCompile(`\bhmcp_[A-Za-z0-9_-]+`), Masked},                               // Hitomado の MCP トークン
	{regexp.MustCompile(`(?i)\b((?:access_|refresh_|id_)?token|client_secret|password|passwd|api[_-]?key|code|secret|authorization)=[^&\s"']+`), "${1}=" + Masked},
	{regexp.MustCompile(`(?i)("(?:access_token|refresh_token|id_token|client_secret|password|api_?key|authorization)"\s*:\s*)"[^"]*"`), `${1}"` + Masked + `"`},
	{regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`), maskedEmail},
}

// Scrub は s の中の秘密の形（トークン・キー・メールアドレス）を伏せて返す。
func Scrub(s string) string {
	for _, r := range scrubRules {
		s = r.re.ReplaceAllString(s, r.with)
	}
	return s
}

// Redact は h に渡るレコードの値を伏せる Handler を返す。
// 秘密・メール内容のキーの値は丸ごと伏せ、それ以外の文字列も Scrub を通す。
func Redact(h slog.Handler) slog.Handler { return &redactHandler{inner: h} }

type redactHandler struct{ inner slog.Handler }

func (r *redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return r.inner.Enabled(ctx, l)
}

func (r *redactHandler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, Scrub(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return r.inner.Handle(ctx, out)
}

func (r *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	red := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		red[i] = redactAttr(a)
	}
	return &redactHandler{inner: r.inner.WithAttrs(red)}
}

func (r *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{inner: r.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		red := make([]slog.Attr, len(group))
		for i, g := range group {
			red[i] = redactAttr(g)
		}
		a.Value = slog.GroupValue(red...)
		return a
	}
	if isSensitiveKey(a.Key) {
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
func scrubAny(v any) string {
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
