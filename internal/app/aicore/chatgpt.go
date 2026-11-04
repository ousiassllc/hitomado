package aicore

import (
	"context"
	"errors"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/siwc"
)

// chatGPTStatus は ChatGPT のログイン状態（通信しない）。使用中のアカウントの状態と、保存済みのアカウントの一覧。
func (s *Service) chatGPTStatus(ctx context.Context) (siwc.Status, error) {
	st, err := s.d.ChatGPT.Status(ctx)
	if err != nil {
		return siwc.Status{}, mapChatGPTError(err)
	}
	return st, nil
}

// chatGPTAccounts は siwc のアカウントを画面向けの DTO にする（常に非 nil の配列）。
func chatGPTAccounts(st siwc.Status) []dto.ChatGPTAccount {
	out := make([]dto.ChatGPTAccount, 0, len(st.Accounts))
	for _, a := range st.Accounts {
		out = append(out, dto.ChatGPTAccount{
			ID: a.ID, Email: a.Email, SignedIn: a.SignedIn, Active: a.Active, PlanNoticePending: a.PlanNoticePending,
		})
	}
	return out
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
// accountID が空なら、新しく登録する（別の ChatGPT アカウントの追加）。指定したときは、その登録で再ログインする。
// ログインしたアカウントが使用中になる。ID トークンを検証して保存を終えるまで、使用中のアカウントとほかのアカウントは変わらない。
// パスワード・Cookie は受け取らない。トークンは資格情報ストアにだけ保存し、ここからは返さない。
func (s *Service) SignInChatGPT(ctx context.Context, accountID string) (dto.AIConfig, error) {
	if _, err := s.d.ChatGPT.SignIn(ctx, siwc.SignInOptions{AccountID: accountID}); err != nil {
		return dto.AIConfig{}, mapChatGPTError(err)
	}
	return s.config(ctx)
}

// CancelSignInChatGPT は進行中のログインを中止する（SignInChatGPT は OAUTH_CANCELLED で終わる）。無ければ何もしない。
func (s *Service) CancelSignInChatGPT() error {
	s.d.ChatGPT.CancelSignIn()
	return nil
}

// SelectChatGPTAccount は下書きの生成に使う ChatGPT アカウントを切り替える（ログイン中のアカウントだけ）。
// ログアウト済みのアカウントは AI_NOT_CONFIGURED（details.reason = "account_signed_out"。画面が「再ログイン」を案内する文面に置き換える）。
func (s *Service) SelectChatGPTAccount(ctx context.Context, accountID string) (dto.AIConfig, error) {
	if err := s.d.ChatGPT.SelectAccount(ctx, accountID); err != nil {
		if errors.Is(err, siwc.ErrNotSignedIn) {
			// 生成の前提の案内（mapChatGPTError）ではなく、選んだアカウントについて次の操作を伝える。
			return dto.AIConfig{}, apperr.Wrap(apperr.AINotConfigured, "このアカウントはログアウト済みです。「再ログイン」からもう一度ログインしてください。", err).
				WithDetail(apperr.DetailReason, apperr.ReasonAccountSignedOut)
		}
		return dto.AIConfig{}, mapChatGPTError(err)
	}
	return s.config(ctx)
}

// SignOutChatGPT は指定した ChatGPT アカウントだけ、OpenAI 側のセッションを失効させてから、ローカルのトークンを消す。
// ほかのアカウントのトークンと登録は残る。失効を確認できなくてもローカルのトークンは消え、RevocationConfirmed=false で返す。
func (s *Service) SignOutChatGPT(ctx context.Context, accountID string) (dto.ChatGPTSignOutResult, error) {
	revoked, err := s.d.ChatGPT.SignOut(ctx, accountID)
	if err != nil {
		return dto.ChatGPTSignOutResult{}, mapChatGPTError(err)
	}
	cfg, err := s.config(ctx)
	if err != nil {
		return dto.ChatGPTSignOutResult{}, err
	}
	return dto.ChatGPTSignOutResult{Config: cfg, RevocationConfirmed: revoked}, nil
}

// AckChatGPTPlanNotice は「ChatGPT プランを使っています」の確認を済みにする。以後、そのアカウントでは出さない。
func (s *Service) AckChatGPTPlanNotice(ctx context.Context, accountID string) (dto.AIConfig, error) {
	if err := s.d.ChatGPT.AckPlanNotice(ctx, accountID); err != nil {
		return dto.AIConfig{}, mapChatGPTError(err)
	}
	return s.config(ctx)
}

// RemoveChatGPTAccount はログアウト済みの ChatGPT アカウントの登録を忘れる（ログイン中のアカウントは VALIDATION）。
func (s *Service) RemoveChatGPTAccount(ctx context.Context, accountID string) (dto.AIConfig, error) {
	if err := s.d.ChatGPT.RemoveAccount(ctx, accountID); err != nil {
		return dto.AIConfig{}, mapChatGPTError(err)
	}
	return s.config(ctx)
}

// mapChatGPTError は siwc のエラーを AppError にする。メッセージにトークン・応答本文は含まない（siwc のエラー自体が含まない）。
func mapChatGPTError(err error) error {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return err
	}
	switch {
	case errors.Is(err, siwc.ErrCancelled):
		return apperr.Wrap(apperr.OAuthCancelled, "ChatGPT のログインをキャンセルしました。", err)
	case errors.Is(err, siwc.ErrDenied):
		return apperr.Wrap(apperr.OAuthDenied, "ChatGPT のログインが許可されませんでした。", err)
	case errors.Is(err, siwc.ErrPlanNotGranted):
		return apperr.Wrap(apperr.OAuthDenied, "ChatGPT プランの利用が許可されませんでした。「Continue with ChatGPT」（登録済みのアカウントは「再ログイン」）をやり直し、プランの利用を許可してください。", err)
	case errors.Is(err, siwc.ErrTimeout):
		return apperr.Wrap(apperr.OAuthTimeout, "ChatGPT のログインが時間内に完了しませんでした。", err)
	case errors.Is(err, siwc.ErrBusy):
		return apperr.Wrap(apperr.Validation, "別の ChatGPT ログインが進行中です。", err)
	case errors.Is(err, siwc.ErrClientIDCollision): // ErrAccountMismatch より先に判定する
		return apperr.Wrap(apperr.Validation, "OpenAI が発行した登録が、既存の ChatGPT アカウントの登録と重なり、同じアカウントか確認できなかったため、保存しませんでした。しばらくしてからもう一度お試しください。登録済みのアカウントは「再ログイン」からログインできます。", err)
	case errors.Is(err, siwc.ErrAccountMismatch):
		return apperr.Wrap(apperr.Validation, "選んだ登録とは別の ChatGPT アカウントでログインされました。同じアカウントでログインするか、「アカウントを追加」から別の登録として追加してください。", err)
	case errors.Is(err, siwc.ErrUnknownAccount):
		return apperr.Wrap(apperr.NotFound, "その ChatGPT アカウントは登録されていません。", err)
	case errors.Is(err, siwc.ErrAccountInUse):
		return apperr.Wrap(apperr.Validation, "ログイン中の ChatGPT アカウントは削除できません。先にログアウトしてください。", err)
	case errors.Is(err, siwc.ErrNotSignedIn):
		return apperr.Wrap(apperr.AINotConfigured, "使用できる ChatGPT アカウントがありません（使用中のアカウントが未選択か、ログアウト済みです）。設定画面でログイン中のアカウントを選ぶか、「再ログイン」（アカウントがないときは「Continue with ChatGPT」）でログインしてください。", err)
	case errors.Is(err, siwc.ErrReauthRequired):
		return apperr.Wrap(apperr.AINotConfigured, "この ChatGPT アカウントはログインが切れています。設定画面の「再ログイン」からもう一度ログインしてください。", err)
	case errors.Is(err, siwc.ErrInvalidResponse):
		return apperr.Wrap(apperr.AIFailed, "OpenAI からの応答を確認できませんでした。もう一度お試しください。", err)
	case errors.Is(err, siwc.ErrFailed):
		return apperr.Wrap(apperr.AIFailed, "ChatGPT のログイン処理に失敗しました。", err)
	}
	return apperr.FromInfra(err, "")
}
