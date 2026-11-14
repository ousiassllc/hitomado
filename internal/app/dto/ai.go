package dto

import "github.com/ousiassllc/hitomado/internal/domain/model"

// AI を使う機能の ID（SetAIFeature の feature。AIFeatures のキーと同じ）。
const (
	AIFeatureDraft     = "draft"     // 「AI で下書き」（返信・新規作成。要約・質問も同じ設定を使う）
	AIFeatureInboxZero = "inboxZero" // Inbox Zero の判定（#377）
)

// AIFeatureSetting は機能 1 つで使う AI・モデルの選択（#376）。保存した値をそのまま返し、つないでいるかは確かめない
// （つないでいない AI を選んでいても、選択は残す。画面が「つなぎ直しが必要」を出す）。
type AIFeatureSetting struct {
	// Provider は選んだプロバイダ（anthropic／openai／chatgpt。Inbox Zero は typesafe も）。空は未選択。
	Provider string `json:"provider"`
	// Model は選んだモデル。typesafe（モデルを選ばない）と未選択は空。
	Model string `json:"model"`
	// Effort は生成に渡すモデルのエフォート（reasoning effort。#111。「AI で下書き」だけ）。空は「モデルの既定」（プロバイダへ送らない）。
	// anthropic は low／medium／high／xhigh／max、openai／chatgpt は low／medium／high。範囲外の保存値は空として返す。
	Effort string `json:"effort,omitempty"`
}

// AIFeatures は機能ごとに使う AI・モデル。
type AIFeatures struct {
	Draft     AIFeatureSetting `json:"draft"`
	InboxZero AIFeatureSetting `json:"inboxZero"`
}

// AIConfig は AI 設定。「つなぎ先」（ログイン・API キー・同意・Base URL）と「機能ごとに使う AI」（#376）を 1 つにまとめて返す。
// API キー・トークン自体は返さない。
type AIConfig struct {
	// BaseURL は OpenAI 互換の Base URL（つなぎ先の設定）。空は OpenAI の既定の送信先。
	BaseURL string `json:"baseUrl,omitempty"`
	// Features は機能ごとに使う AI・モデル。
	Features AIFeatures `json:"features"`
	// APIKeyProviders は API キーを登録済みのプロバイダ（anthropic／openai／typesafe。chatgpt は ChatGPTSignedIn）。null にしない。
	APIKeyProviders []string `json:"apiKeyProviders"`
	// ConsentedProviders は送信先への同意済みのプロバイダ（anthropic／openai／chatgpt／typesafe）。
	// openai は現在の Base URL の送信先への同意だけが有効。null にしない。
	ConsentedProviders []string `json:"consentedProviders"`
	// ChatGPTSignedIn は使用中の ChatGPT アカウントがログイン済みか。ChatGPTEmail はそのメールアドレス（表示用）。
	ChatGPTSignedIn bool   `json:"chatgptSignedIn"`
	ChatGPTEmail    string `json:"chatgptEmail,omitempty"`
	// ChatGPTAccounts は保存済みの ChatGPT アカウント（登録順。トークンは含まない）。
	ChatGPTAccounts []ChatGPTAccount `json:"chatgptAccounts"`
}

// ChatGPTAccount は保存済みの ChatGPT アカウント（クライアント登録）1 件。
type ChatGPTAccount struct {
	// ID は発行された client_id（登録の識別子）。同じメールアドレスでも、別の登録なら別の ID になる。
	ID       string `json:"id"`
	Email    string `json:"email,omitempty"`
	SignedIn bool   `json:"signedIn"` // ログイン中のセッションがあるか
	Active   bool   `json:"active"`   // 下書きの生成に使うアカウントか
	// PlanNoticePending は、初めてログインして ChatGPT プランの利用が有効になったのに、「You're using your ChatGPT plan」の確認を
	// まだ済ませていないか。画面が確認を出し、「Got it」で AckChatGPTPlanNotice を呼ぶ（2 回目以降のログインでは立たない）。
	PlanNoticePending bool `json:"planNoticePending"`
}

// ChatGPTSignOutResult は ChatGPT ログアウトの結果。ローカルのトークンは常に消えている。
type ChatGPTSignOutResult struct {
	Config AIConfig `json:"config"`
	// RevocationConfirmed は OpenAI 側でセッションの失効を確認できたか。false のときは ChatGPT の設定で連携を解除できる旨を伝える。
	RevocationConfirmed bool `json:"revocationConfirmed"`
}

// AIModel はモデル一覧の 1 件。
type AIModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	// Efforts はそのモデルが受け付けるエフォートの段階（弱い順。#111）。anthropic は Models API の capabilities.effort から作る
	// （対応しない・取れないモデルは省略＝画面はエフォートの選択を出さない）。openai・chatgpt は一覧が段階を返さないので常に省略する
	// （画面は共通の low／medium／high を出す）。
	Efforts []string `json:"efforts,omitempty"`
}

// 一覧を取得できなかった理由（AIModelList.Reason）。
const (
	AIModelsNoCredentials   = "no_credentials"   // API キー未登録・ChatGPT 未ログイン
	AIModelsConsentRequired = "consent_required" // 送信先への同意前（取得も送らない）
	AIModelsInvalidBaseURL  = "invalid_base_url" // Base URL が使えない（送らない）
	AIModelsUnsupported     = "unsupported"      // Models API が無い
	AIModelsUnauthorized    = "unauthorized"     // キー不正・権限なし
	AIModelsRateLimited     = "rate_limited"
	AIModelsNetwork         = "network" // 通信できない・時間切れ
	AIModelsFailed          = "failed"  // その他（不正な応答など）
)

// AIModelList はモデル一覧の取得結果。取得できないときは Models が空で Reason と Message を持ち、手入力に切り替える（エラーではない）。
type AIModelList struct {
	Provider string    `json:"provider"`
	Models   []AIModel `json:"models"`
	Reason   string    `json:"reason,omitempty"`
	Message  string    `json:"message,omitempty"`
}

// ReplyDraft は AI が生成した下書きの本文（返信・新規作成。署名・引用を含まない）。
type ReplyDraft struct {
	BodyHTML string `json:"bodyHtml"`
}

// NewDraftContext は新規作成の AI 下書き（GenerateNewDraft）に渡す、作成画面に入力済みの件名と宛先（To のみ。どちらも空でよい）。
type NewDraftContext struct {
	Subject string          `json:"subject"`
	To      []model.Address `json:"to"`
}

// ThreadAIResult はスレッドの AI 要約・質問の結果（F-17・F-18）。Text は日本語のプレーンテキスト。
// IncludedCount は入力に含めたメッセージ数、TotalCount はスレッドの（下書きを除く）メッセージ数（長いスレッドで古いものを外すと IncludedCount < TotalCount）。
type ThreadAIResult struct {
	Text          string `json:"text"`
	IncludedCount int    `json:"includedCount"`
	TotalCount    int    `json:"totalCount"`
}

// ThreadAITurn はスレッドへの AI 質問の会話 1 件（F-18）。Role は user（利用者の質問）か assistant（AI の回答）。
type ThreadAITurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}
