// Package reply は返信・全員に返信・転送の宛先と件名を決定する（純粋ロジック）。
package reply

import (
	"regexp"
	"strings"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

var (
	rePrefix  = regexp.MustCompile(`(?i)^\s*re\s*:`)
	fwdPrefix = regexp.MustCompile(`(?i)^\s*(fwd|fw)\s*:`)
)

// Resolve は mode に応じた返信宛先を返す。self は受信アカウント自身のアドレス（小文字）。
//   - reply: Reply-To（あれば）、なければ From。元が自分の送信なら元の To
//   - reply_all: 上記の返信宛先を To、元の To／Cc から自アドレスと To に居る人を除いたものを Cc
//   - forward／その他: 空
//
// メールアドレスは小文字化して比較・出力し、重複は先頭を残して除く（順序保持）。
func Resolve(mode model.Mode, src model.SourceMessage, self string) model.Recipients {
	self = normalize(self)
	switch mode {
	case model.ModeReply:
		return model.Recipients{To: primary(src, self), Cc: []model.Address{}}
	case model.ModeReplyAll:
		to := primary(src, self)
		seen := map[string]bool{}
		for _, a := range to {
			seen[a.Email] = true
		}
		seen[self] = true
		others := append(append([]model.Address{}, src.To...), src.Cc...)
		return model.Recipients{To: to, Cc: dedupe(others, seen)}
	default:
		return model.Recipients{To: []model.Address{}, Cc: []model.Address{}}
	}
}

// primary は返信の主宛先を返す。自アドレス以外が残る限り自アドレスは除く
// （自分だけに送ったメッセージへの返信のように、除くと空になる場合は自分を残す）。
func primary(src model.SourceMessage, self string) []model.Address {
	var base []model.Address
	switch {
	case normalize(src.From.Email) == self:
		base = src.To
	case len(src.ReplyTo) > 0:
		base = src.ReplyTo
	default:
		base = []model.Address{src.From}
	}
	if got := dedupe(base, map[string]bool{self: true}); len(got) > 0 {
		return got
	}
	return dedupe(base, map[string]bool{})
}

// dedupe は as のうち seen に無いアドレスを、重複を除き順序を保って返す。seen は更新される。
func dedupe(as []model.Address, seen map[string]bool) []model.Address {
	out := []model.Address{}
	for _, a := range as {
		e := normalize(a.Email)
		if e == "" || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, model.Address{Name: strings.TrimSpace(a.Name), Email: e})
	}
	return out
}

func normalize(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// Subject は mode に応じて "Re: "／"Fwd: " を付けた件名を返す。
// すでに同種の接頭辞（大文字小文字を区別しない。転送は "Fw:" も既存扱い）があれば重複させない。
func Subject(mode model.Mode, orig string) string {
	switch mode {
	case model.ModeReply, model.ModeReplyAll:
		return withPrefix("Re: ", rePrefix, orig)
	case model.ModeForward:
		return withPrefix("Fwd: ", fwdPrefix, orig)
	default:
		return orig
	}
}

func withPrefix(prefix string, re *regexp.Regexp, orig string) string {
	if re.MatchString(orig) {
		return orig
	}
	return strings.TrimRight(prefix+strings.TrimSpace(orig), " ")
}
