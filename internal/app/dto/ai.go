package dto

// AIConfig は AI 設定。API キー自体は返さない。BaseURL は openai のみ。
type AIConfig struct {
	Provider           string   `json:"provider"` // anthropic / openai
	Model              string   `json:"model"`
	BaseURL            string   `json:"baseUrl,omitempty"`
	HasAPIKey          bool     `json:"hasApiKey"`
	ConsentedProviders []string `json:"consentedProviders"`
}

// ReplyDraft は AI が生成した返信本文（署名・引用を含まない）。
type ReplyDraft struct {
	BodyHTML string `json:"bodyHtml"`
}
