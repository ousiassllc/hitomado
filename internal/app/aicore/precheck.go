package aicore

import (
	"context"
	"slices"
	"strings"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/siwc"
)

// precheck は前提条件（キー・モデル設定済み→同意済み）を検査する。キーはここ以外に渡さない。
func (s *Service) precheck(ctx context.Context) (storedAIConfig, string, error) {
	cfg, key, err := s.precheckReady(ctx)
	if err != nil {
		return cfg, "", err
	}
	if cfg.Provider == providerChatGPT {
		// ChatGPT のトークンは、同意の確認が済んでから取得（更新）する。
		var ok bool
		key, ok, err = s.credential(ctx, cfg.Provider)
		if err != nil {
			return cfg, "", err
		}
		if !ok {
			return cfg, "", errChatGPTNotConfigured()
		}
	}
	return cfg, key, nil
}

// precheckReady は precheck のうち、設定済みか（キー／ログイン・モデル・Base URL）と同意済みかの検査まで。ChatGPT のトークンは取得しない
// （キーは ChatGPT 以外のときだけ返す）。AI が使える状態か（Ready）の判定に使う。
func (s *Service) precheckReady(ctx context.Context) (storedAIConfig, string, error) {
	cfg, key, err := s.precheckConfigured(ctx)
	if err != nil {
		return cfg, "", err
	}
	consented, err := s.loadConsent(ctx, cfg)
	if err != nil {
		return cfg, "", err
	}
	if !slices.Contains(consented, cfg.Provider) {
		return cfg, "", apperr.New(apperr.AIConsentRequired, "スレッドの内容が AI プロバイダへ送信されることへの同意が必要です。")
	}
	return cfg, key, nil
}

// precheckConfigured は設定済みか（キー／ChatGPT のログイン・モデル・保存済みの Base URL）だけを検査する。同意は見ない。
func (s *Service) precheckConfigured(ctx context.Context) (storedAIConfig, string, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return cfg, "", err
	}
	// ChatGPT のトークンは、同意の確認が済むまで取得（更新）しない。まずはログイン済みかだけを見る。
	var (
		key string
		ok  bool
	)
	if cfg.Provider == providerChatGPT {
		var gpt siwc.Status
		gpt, err = s.chatGPTStatus(ctx)
		ok = gpt.SignedIn
	} else {
		key, ok, err = s.apiKey(cfg.Provider)
	}
	if err != nil {
		return cfg, "", err
	}
	if !ok || strings.TrimSpace(cfg.Model) == "" {
		if cfg.Provider == providerChatGPT {
			return cfg, "", errChatGPTNotConfigured()
		}
		return cfg, "", apperr.New(apperr.AINotConfigured, "AI の API キーとモデルを設定画面で設定してください。")
	}
	if cfg.BaseURL != "" && cfg.Provider == providerOpenAI {
		// 保存済みの旧設定（非 loopback の http:// など）は、接続前に分かりやすく拒否する。
		if err := validateBaseURL(cfg.BaseURL); err != nil {
			return cfg, "", apperr.Wrap(apperr.Validation, "保存済みの Base URL は使えません。設定画面で https:// の URL（または localhost の http://）に直してください。", err)
		}
	}
	return cfg, key, nil
}

func errChatGPTNotConfigured() error {
	return apperr.New(apperr.AINotConfigured, "ChatGPT にログインし、モデルを設定画面で設定してください。")
}

// precheckError は precheck の失敗を返す形にする。実行が中止されていれば（ChatGPT のトークン更新の途中を含む）CANCELLED にする。
func (s *Service) precheckError(runCtx context.Context, err error) error {
	if runCtx.Err() != nil {
		return s.mapError(runCtx, err, "")
	}
	return err
}
