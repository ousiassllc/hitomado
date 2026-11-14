package aicore

import (
	"context"
	"slices"
	"strings"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/siwc"
)

// featureLabels は機能の表示名（エラー文面用）。
var featureLabels = map[string]string{dto.AIFeatureDraft: "「AI で下書き」", dto.AIFeatureInboxZero: "Inbox Zero"}

// precheck は feature で選んだ AI の前提条件（選択済み→キー・ログイン済み→モデル→同意済み）を検査する。キーはここ以外に渡さない。
// 選んだ AI がつながっていないときも、ほかの AI へ切り替えず AI_NOT_CONFIGURED で止める。
// TypeSafe AI（モデルを持たない）はモデルの検査をしない。
func (s *Service) precheck(ctx context.Context, feature string) (SelectedAI, string, error) {
	sel, saved, err := s.FeatureAI(ctx, feature)
	if err != nil {
		return sel, "", err
	}
	label := featureLabels[feature]
	if !saved {
		return sel, "", apperr.New(apperr.AINotConfigured, label+"で使う AI とモデルを、設定画面の「機能ごとに使う AI」で選んでください。")
	}
	// ChatGPT のトークンは、同意の確認が済むまで取得（更新）しない。まずはログイン済みかだけを見る。
	var (
		key string
		ok  bool
	)
	if sel.Provider == providerChatGPT {
		var gpt siwc.Status
		gpt, err = s.chatGPTStatus(ctx)
		ok = gpt.SignedIn
	} else {
		key, ok, err = s.apiKey(sel.Provider)
	}
	if err != nil {
		return sel, "", err
	}
	if !ok {
		return sel, "", notConnectedError(label, sel.Provider)
	}
	if sel.Provider != providerTypeSafe && strings.TrimSpace(sel.Model) == "" {
		return sel, "", apperr.New(apperr.AINotConfigured, label+"で使うモデルを、設定画面の「機能ごとに使う AI」で選んでください。")
	}
	if sel.BaseURL != "" && sel.Provider == providerOpenAI {
		// 保存済みの旧設定（非 loopback の http:// など）は、接続前に分かりやすく拒否する。
		if err := validateBaseURL(sel.BaseURL); err != nil {
			return sel, "", apperr.Wrap(apperr.Validation, "保存済みの Base URL は使えません。設定画面の「AI のつなぎ先」で https:// の URL（または localhost の http://）に直してください。", err)
		}
	}
	consented, err := s.loadConsent(ctx, sel.BaseURL)
	if err != nil {
		return sel, "", err
	}
	if !slices.Contains(consented, sel.Provider) {
		return sel, "", apperr.New(apperr.AIConsentRequired, "スレッドの内容が AI プロバイダへ送信されることへの同意が必要です。")
	}
	if sel.Provider == providerChatGPT {
		key, ok, err = s.credential(ctx, sel.Provider)
		if err != nil {
			return sel, "", err
		}
		if !ok {
			return sel, "", notConnectedError(label, sel.Provider)
		}
	}
	return sel, key, nil
}

// notConnectedError は選んだ AI がつながっていないときのエラー（つなぎ直しの案内）。
func notConnectedError(label, provider string) error {
	if provider == providerChatGPT {
		return apperr.New(apperr.AINotConfigured, label+"で選んだ ChatGPT プランにログインし直してください（設定画面の「AI のつなぎ先」）。")
	}
	return apperr.New(apperr.AINotConfigured, label+"で選んだ AI の API キーを、設定画面の「AI のつなぎ先」で登録し直してください。")
}
