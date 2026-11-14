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
	if !saved {
		return sel, "", errNotSelected(feature)
	}
	return s.precheckSelected(ctx, feature, sel)
}

// errNotSelected は feature で使う AI が未選択のときのエラー（選ぶ場所の案内）。
func errNotSelected(feature string) error {
	return apperr.New(apperr.AINotConfigured, featureLabels[feature]+"で使う AI とモデルを、設定画面の「機能ごとに使う AI」で選んでください。")
}

// precheckInboxZero は Inbox Zero で使う AI の前提条件を検査する（precheck と同じ）。未選択のときだけ、TypeSafe AI のキーがあれば
// TypeSafe AI を使う（#314 の利用者は、Inbox Zero の AI を選ぶ前から TypeSafe AI で動いていたため。選んでいる AI は、つながっていなくても切り替えない）。
func (s *Service) precheckInboxZero(ctx context.Context) (SelectedAI, string, error) {
	sel, saved, err := s.FeatureAI(ctx, dto.AIFeatureInboxZero)
	if err != nil {
		return sel, "", err
	}
	if !saved {
		_, ok, err := s.apiKey(providerTypeSafe)
		if err != nil {
			return sel, "", err
		}
		if !ok {
			return sel, "", errNotSelected(dto.AIFeatureInboxZero)
		}
		sel = SelectedAI{Provider: providerTypeSafe}
	}
	return s.precheckSelected(ctx, dto.AIFeatureInboxZero, sel)
}

// precheckSelected は選んだ AI（sel）がつながっていて、モデルがあり、同意済みかを検査し、使うキーを返す。
func (s *Service) precheckSelected(ctx context.Context, feature string, sel SelectedAI) (SelectedAI, string, error) {
	label := featureLabels[feature]
	// ChatGPT のトークンは、同意の確認が済むまで取得（更新）しない。まずはログイン済みかだけを見る。
	var (
		key string
		ok  bool
		err error
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
