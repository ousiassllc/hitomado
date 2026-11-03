package aicore

import (
	"context"
	"errors"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/siwc"
)

// chatGPTStatus は ChatGPT のログイン状態（通信しない）。メールアドレスは表示用。
func (s *Service) chatGPTStatus(ctx context.Context) (signedIn bool, email string, err error) {
	st, err := s.d.ChatGPT.Status(ctx)
	if err != nil {
		return false, "", mapChatGPTError(err)
	}
	return st.SignedIn, st.Email, nil
}

// credential は provider の資格情報（API キー、chatgpt では有効なアクセストークン）。未設定は ok=false。
// ChatGPT のトークンは、期限が近ければここで更新される。
func (s *Service) credential(ctx context.Context, provider string) (string, bool, error) {
	if provider != providerChatGPT {
		return s.apiKey(provider)
	}
	tok, err := s.d.ChatGPT.AccessToken(ctx)
	switch {
	case err == nil:
		return tok, true, nil
	case errors.Is(err, siwc.ErrNotSignedIn):
		return "", false, nil
	}
	return "", false, mapChatGPTError(err)
}

// SignInChatGPT は OpenAI の公式画面（システムブラウザ）で ChatGPT にログインする。ログインの完了・中止・時間切れまでブロックする。
// differentAccount が true なら、保存済みの登録を使わず新しく登録する（別の ChatGPT アカウントへの切り替え）。
// パスワード・Cookie は受け取らない。トークンは資格情報ストアにだけ保存し、ここからは返さない。
func (s *Service) SignInChatGPT(ctx context.Context, differentAccount bool) (dto.AIConfig, error) {
	if _, err := s.d.ChatGPT.SignIn(ctx, siwc.SignInOptions{DifferentAccount: differentAccount}); err != nil {
		return dto.AIConfig{}, mapChatGPTError(err)
	}
	return s.config(ctx)
}

// CancelSignInChatGPT は進行中のログインを中止する（SignInChatGPT は OAUTH_CANCELLED で終わる）。無ければ何もしない。
func (s *Service) CancelSignInChatGPT() error {
	s.d.ChatGPT.CancelSignIn()
	return nil
}

// SignOutChatGPT は OpenAI 側のセッションを失効させてから、ローカルのトークンを消す。
// 失効を確認できなくてもローカルのトークンは消え、RevocationConfirmed=false で返す。
func (s *Service) SignOutChatGPT(ctx context.Context) (dto.ChatGPTSignOutResult, error) {
	revoked, err := s.d.ChatGPT.SignOut(ctx)
	if err != nil {
		return dto.ChatGPTSignOutResult{}, mapChatGPTError(err)
	}
	cfg, err := s.config(ctx)
	if err != nil {
		return dto.ChatGPTSignOutResult{}, err
	}
	return dto.ChatGPTSignOutResult{Config: cfg, RevocationConfirmed: revoked}, nil
}

// mapChatGPTError は siwc のエラーを AppError にする。メッセージにトークン・応答本文は含まない（siwc のエラー自体が含まない）。
func mapChatGPTError(err error) error {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return err
	}
	switch {
	case errors.Is(err, siwc.ErrCancelled), errors.Is(err, context.Canceled):
		return apperr.Wrap(apperr.OAuthCancelled, "ChatGPT のログインをキャンセルしました。", err)
	case errors.Is(err, siwc.ErrDenied):
		return apperr.Wrap(apperr.OAuthDenied, "ChatGPT のログインが許可されませんでした。", err)
	case errors.Is(err, siwc.ErrPlanNotGranted):
		return apperr.Wrap(apperr.OAuthDenied, "ChatGPT プランの利用が許可されませんでした。「ChatGPT でログイン」をやり直し、プランの利用を許可してください。", err)
	case errors.Is(err, siwc.ErrTimeout):
		return apperr.Wrap(apperr.OAuthTimeout, "ChatGPT のログインが時間内に完了しませんでした。", err)
	case errors.Is(err, siwc.ErrBusy):
		return apperr.Wrap(apperr.Validation, "別の ChatGPT ログインが進行中です。", err)
	case errors.Is(err, siwc.ErrAccountMismatch):
		return apperr.Wrap(apperr.Validation, "保存済みとは別の ChatGPT アカウントです。「別のアカウントでログイン」から登録し直してください。", err)
	case errors.Is(err, siwc.ErrNotSignedIn), errors.Is(err, siwc.ErrReauthRequired):
		return apperr.Wrap(apperr.AINotConfigured, "ChatGPT のログインが切れました。設定画面で「ChatGPT でログイン」をやり直してください。", err)
	case errors.Is(err, siwc.ErrInvalidResponse):
		return apperr.Wrap(apperr.AIFailed, "OpenAI からの応答を確認できませんでした。もう一度お試しください。", err)
	case errors.Is(err, siwc.ErrFailed):
		return apperr.Wrap(apperr.AIFailed, "ChatGPT のログイン処理に失敗しました。", err)
	}
	return apperr.FromInfra(err, "")
}
