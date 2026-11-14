package dto

import "github.com/ousiassllc/hitomado/internal/domain/model"

// AgentItem.Status の値。
const (
	AgentStatusReady   = "ready"   // 返信が要ると判断し、下書きを作った。利用者の確認待ち
	AgentStatusAsking  = "asking"  // 返信が要るが、決められないことがある。質問への答え待ち
	AgentStatusSkipped = "skipped" // 返信は要らないと判断した（「やっぱり返信する」で直せる）
	AgentStatusLater   = "later"   // 「あとで」にした（確認待ち・質問待ちだったもの）
	AgentStatusDone    = "done"    // 利用者が下書きを開いて確認した
	AgentStatusFailed  = "failed"  // 判断または下書き作りに失敗した（「やっぱり返信する」でやり直せる）
)

// AgentSettings はエージェントモードの設定。既定はすべてオフ。
type AgentSettings struct {
	// Enabled はエージェントモード。オンのときだけ、新着メールを AI が自動で判断して下書きを作る。
	Enabled bool `json:"enabled"`
	// MascotEnabled は画面の端のマスコット（Windows のみ）。Enabled がオンのときだけ表示する。
	MascotEnabled bool `json:"mascotEnabled"`
	// AIReady は AI（プロバイダ・モデル・キー／ログイン・同意）が設定済みで、エージェントを動かせるか。偽なら設定の AI へ案内する。
	AIReady bool `json:"aiReady"`
}

// AgentItem はエージェントが扱った新着 1 件（パネルの 1 行）。
type AgentItem struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	AccountName  string `json:"accountName"`  // 受信アカウントの表示名（空ならアドレス）
	AccountEmail string `json:"accountEmail"` // 受信アカウントのアドレス
	ThreadID     string `json:"threadId"`
	Status       string `json:"status"` // AgentStatus*
	Subject      string `json:"subject"`
	From         string `json:"from"` // 新着の差出人（表示名。無ければアドレス）
	// ReasonKind は判断の理由の種類（log にも出す固定値）。Reason は理由の一言（画面用の日本語）。
	ReasonKind string `json:"reasonKind"`
	Reason     string `json:"reason"`
	// DraftID・DraftPreview・To は Status が ready／later／done で下書きがあるときだけ。DraftPreview は本文の最初の数行（プレーンテキスト）。
	DraftID      string          `json:"draftId,omitempty"`
	DraftPreview string          `json:"draftPreview,omitempty"`
	To           []model.Address `json:"to"`
	// Questions は Status が asking のときの、利用者に聞くこと（1〜3 件）。
	Questions []string `json:"questions"`
	// UpdatedAt は Unix ミリ秒。
	UpdatedAt int64 `json:"updatedAt"`
}

// AgentOverview は ListAgentItems の結果。
type AgentOverview struct {
	// PendingCount は確認待ち（ready）と質問待ち（asking）の合計。ナビ・マスコットのバッジに使う。
	PendingCount int `json:"pendingCount"`
	// Items は新しい順（UpdatedAt 降順）。null にしない。
	Items []AgentItem `json:"items"`
}
