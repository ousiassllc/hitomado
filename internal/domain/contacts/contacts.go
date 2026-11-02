// Package contacts は宛先候補（F-13）のアドレス正規化・形式検証・順位付けを提供する（純粋ロジック）。
package contacts

import (
	"net/mail"
	"sort"
	"strings"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// maxEmailLen は RFC 5321 のパス長上限に基づくアドレス長の上限。
const maxEmailLen = 254

// Normalize はアドレスの前後空白を除いて小文字にする。
func Normalize(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// IsValid は email が単純なアドレス（user@domain）の形式かを返す。
// 表示名付き（"Name <a@b>"）・山括弧付き・前後の空白付きは不可。
func IsValid(email string) bool {
	if email == "" || len(email) > maxEmailLen {
		return false
	}
	a, err := mail.ParseAddress(email)
	return err == nil && a.Name == "" && a.Address == email
}

// Score は候補の順位用スコア（送信先として使った回数を 3 倍で重み付けする）。
func Score(sendCount, receiveCount int) int {
	return sendCount*3 + receiveCount
}

// Candidate は contacts テーブルの 1 行に相当する順位付けの入力。LastUsedAt は Unix ms。
type Candidate struct {
	Address      model.Address
	SendCount    int
	ReceiveCount int
	LastUsedAt   int64
}

// Rank は候補を Score 降順（同点は LastUsedAt 降順、さらに同点は入力順）に並べ、
// self（受信アカウント自身のアドレス）を除いて limit 件までの Address を返す。
// limit が 0 以下なら全件。メールアドレスは Normalize して重複は先頭を残す。
func Rank(cs []Candidate, self string, limit int) []model.Address {
	self = Normalize(self)
	seen := map[string]bool{self: true}
	kept := make([]Candidate, 0, len(cs))
	for _, c := range cs {
		e := Normalize(c.Address.Email)
		if e == "" || seen[e] {
			continue
		}
		seen[e] = true
		c.Address.Email = e
		kept = append(kept, c)
	}
	sort.SliceStable(kept, func(i, j int) bool {
		si, sj := Score(kept[i].SendCount, kept[i].ReceiveCount), Score(kept[j].SendCount, kept[j].ReceiveCount)
		if si != sj {
			return si > sj
		}
		return kept[i].LastUsedAt > kept[j].LastUsedAt
	})
	if limit > 0 && len(kept) > limit {
		kept = kept[:limit]
	}
	out := make([]model.Address, len(kept))
	for i, c := range kept {
		out[i] = c.Address
	}
	return out
}
