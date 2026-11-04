package dto

// AIConfig は AI 設定。API キー・トークン自体は返さない。BaseURL は openai のみ。
type AIConfig struct {
	Provider string `json:"provider"` // anthropic / openai / chatgpt
	Model    string `json:"model"`
	BaseURL  string `json:"baseUrl,omitempty"`
	// HasAPIKey は現在のプロバイダの資格情報が設定済みか（anthropic／openai は API キー、chatgpt は ChatGPT へのログイン）。
	HasAPIKey          bool     `json:"hasApiKey"`
	ConsentedProviders []string `json:"consentedProviders"`
	// ChatGPTSignedIn は選択中のプロバイダに関わらず、使用中の ChatGPT アカウントがログイン済みか。ChatGPTEmail はそのメールアドレス（表示用）。
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

// ReplyDraft は AI が生成した返信本文（署名・引用を含まない）。
type ReplyDraft struct {
	BodyHTML string `json:"bodyHtml"`
}
