package gmail

import (
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail/mimebuild"
)

// ErrInvalidMessage は BuildMIME／BuildDraftMIME の入力が不正（ヘッダインジェクション・不正なアドレス・宛先なし等）なことを表す。
var ErrInvalidMessage = mimebuild.ErrInvalid

// BuildMIME は送信メッセージを RFC 5322 の生 MIME にする（Gmail の raw にする前の形。Send/CreateDraft が base64url 化する）。
// multipart/alternative（text/plain＋text/html）で、添付があれば multipart/mixed。
// ヘッダ値に CR/LF が含まれる入力はヘッダインジェクションとして拒否する。
func BuildMIME(m model.OutgoingMessage) ([]byte, error) { return mimebuild.Build(m) }

// BuildDraftMIME は下書き用の BuildMIME。宛先が 1 件もなくてもよい（To ヘッダは出力しない）。
func BuildDraftMIME(m model.OutgoingMessage) ([]byte, error) { return mimebuild.BuildDraft(m) }
