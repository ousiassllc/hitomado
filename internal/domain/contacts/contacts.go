// Package contacts は宛先候補（F-13）のアドレス正規化・形式検証を提供する（順位付けは store の SuggestContacts の SQL）（純粋ロジック）。
package contacts

import (
	"net/mail"
	"strings"
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
