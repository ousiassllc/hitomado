package aicore

import (
	"context"
	"errors"
	"net"
	"slices"
	"time"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// aiModelsTimeout はモデル一覧の取得のタイムアウト。
const aiModelsTimeout = 20 * time.Second

// ListAIModels は provider のモデル一覧を、保存済みの設定（Base URL）の送信先から取得する（#76）。
// キーは資格情報ストアから読み、呼び出し元（フロント）には渡さない。呼ぶのは API キーの登録・変更後、Base URL の変更後、
// 利用者が「更新」を押したときだけで、定期的には呼ばない（呼び出し側の責務）。
//
// 取得できないとき（キー未登録・同意前・不正な Base URL・Models API が無い・通信できない・権限がない）は、エラーではなく
// Models が空で Reason と短い Message を持つ結果を返し、画面は手入力へ切り替える。
// 同意前の送信先・使えない Base URL には、一覧の取得も送らない。ログには件数と結果だけを残す（キー・応答の中身は残さない）。
func (s *Service) ListAIModels(ctx context.Context, provider string) (dto.AIModelList, error) {
	if !validProvider(provider) {
		return dto.AIModelList{}, errBadProvider()
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return dto.AIModelList{}, err
	}
	baseURL := currentBaseURL(cfg, provider)
	if baseURL != "" {
		if err := validateBaseURL(baseURL); err != nil {
			return s.noModels(provider, dto.AIModelsInvalidBaseURL, "Base URL が使えないため、モデル一覧を取得できません。モデル名を入力してください。"), nil
		}
	}
	consented, err := s.loadConsent(ctx, cfg)
	if err != nil {
		return dto.AIModelList{}, err
	}
	if !slices.Contains(consented, provider) {
		return s.noModels(provider, dto.AIModelsConsentRequired, "送信先への同意前は、モデル一覧を取得しません。モデル名を入力してください。"), nil
	}
	cred, ok, err := s.credential(ctx, provider)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) && ae.Code == apperr.AINotConfigured {
			return s.noModels(provider, dto.AIModelsNoCredentials, noCredentialsMessage(provider)), nil
		}
		if errors.As(err, &ae) && ae.Code == apperr.Offline {
			reason, msg := modelsFailure(err)
			return s.noModels(provider, reason, msg), nil
		}
		return dto.AIModelList{}, err
	}
	if !ok {
		return s.noModels(provider, dto.AIModelsNoCredentials, noCredentialsMessage(provider)), nil
	}

	lctx, cancel := context.WithTimeout(ctx, aiModelsTimeout)
	defer cancel()
	list, err := s.d.AIModels(lctx, provider, baseURL, cred)
	if err != nil {
		if ctx.Err() != nil {
			return dto.AIModelList{}, apperr.Wrap(apperr.Cancelled, "モデル一覧の取得を中止しました。", err)
		}
		reason, msg := modelsFailure(err)
		s.d.Logger.Warn("ai models list failed", "provider", provider, "reason", reason)
		return dto.AIModelList{Provider: provider, Models: []dto.AIModel{}, Reason: reason, Message: msg}, nil
	}
	out := make([]dto.AIModel, len(list))
	for i, m := range list {
		out[i] = dto.AIModel{ID: m.ID, DisplayName: m.DisplayName}
	}
	s.d.Logger.Info("ai models listed", "provider", provider, "count", len(out))
	return dto.AIModelList{Provider: provider, Models: out}, nil
}

func (s *Service) noModels(provider, reason, msg string) dto.AIModelList {
	return dto.AIModelList{Provider: provider, Models: []dto.AIModel{}, Reason: reason, Message: msg}
}

func noCredentialsMessage(provider string) string {
	if provider == providerChatGPT {
		return "ChatGPT にログインすると、使えるモデルを取得できます。モデル名を入力することもできます。"
	}
	return "API キーを登録すると、使えるモデルを取得できます。モデル名を入力することもできます。"
}

// modelsFailure は一覧の取得エラーを理由コードと短い表示にする。メッセージにキー・応答本文・URL は含めない。
func modelsFailure(err error) (reason, msg string) {
	var netErr net.Error
	switch {
	case errors.Is(err, ai.ErrInvalidBaseURL):
		return dto.AIModelsInvalidBaseURL, "Base URL が使えないため、モデル一覧を取得できません。モデル名を入力してください。"
	case errors.Is(err, ai.ErrModelsUnsupported):
		return dto.AIModelsUnsupported, "このサービスにはモデル一覧の取得機能がないようです。モデル名を入力してください。"
	case errors.Is(err, ai.ErrUnauthorized):
		return dto.AIModelsUnauthorized, "認証または権限がないため、モデル一覧を取得できません。キー（またはログイン）を確認するか、モデル名を入力してください。"
	case errors.Is(err, ai.ErrRateLimited):
		return dto.AIModelsRateLimited, "利用制限のため、モデル一覧を取得できません。しばらくしてから更新するか、モデル名を入力してください。"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr):
		return dto.AIModelsNetwork, "通信できないため、モデル一覧を取得できません。モデル名を入力してください。"
	}
	return dto.AIModelsFailed, "モデル一覧を取得できませんでした。モデル名を入力してください。"
}
