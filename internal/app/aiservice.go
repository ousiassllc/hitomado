package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/aicore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// AIService は AI 設定・同意・下書き生成（バインディング仕様 §6）。
// Wails のバインディング面で、実装は aicore に委譲する。
type AIService struct{ core *aicore.Service }

// NewAIService は AIService を作る。返信系セッションの参照に compose を使う。
func NewAIService(d *Deps, compose *ComposeService) *AIService {
	return &AIService{core: aicore.New((*deps.Deps)(d), compose.core)}
}

func (s *AIService) GetAIConfig(ctx context.Context) (dto.AIConfig, error) {
	return s.core.GetAIConfig(ctx)
}

func (s *AIService) SetAIConfig(ctx context.Context, provider, model, baseURL string) (dto.AIConfig, error) {
	return s.core.SetAIConfig(ctx, provider, model, baseURL)
}

func (s *AIService) SetAIApiKey(ctx context.Context, provider, apiKey string) (dto.AIConfig, error) {
	return s.core.SetAIApiKey(ctx, provider, apiKey)
}

func (s *AIService) ClearAIApiKey(ctx context.Context, provider string) (dto.AIConfig, error) {
	return s.core.ClearAIApiKey(ctx, provider)
}

func (s *AIService) RecordAIConsent(ctx context.Context, provider, baseURL string) (dto.AIConfig, error) {
	return s.core.RecordAIConsent(ctx, provider, baseURL)
}

// SignInChatGPT は OpenAI の公式画面で ChatGPT にログインする（完了・中止・時間切れまでブロック）。accountId が空なら別のアカウントを追加し、指定したときはその登録で再ログインする。
func (s *AIService) SignInChatGPT(ctx context.Context, accountID string) (dto.AIConfig, error) {
	return s.core.SignInChatGPT(ctx, accountID)
}

// CancelSignInChatGPT は進行中の ChatGPT ログインを中止する（SignInChatGPT は OAUTH_CANCELLED で終わる）。
func (s *AIService) CancelSignInChatGPT() error { return s.core.CancelSignInChatGPT() }

// SelectChatGPTAccount は下書きに使う ChatGPT アカウントを切り替える（ログイン中のアカウントだけ）。
func (s *AIService) SelectChatGPTAccount(ctx context.Context, accountID string) (dto.AIConfig, error) {
	return s.core.SelectChatGPTAccount(ctx, accountID)
}

// SignOutChatGPT は指定した ChatGPT アカウントだけログアウトする（OpenAI 側の失効を試み、そのトークンを消す）。
func (s *AIService) SignOutChatGPT(ctx context.Context, accountID string) (dto.ChatGPTSignOutResult, error) {
	return s.core.SignOutChatGPT(ctx, accountID)
}

// AckChatGPTPlanNotice は「ChatGPT プランを使っています」の確認を済みにする（そのアカウントでは以後出さない）。
func (s *AIService) AckChatGPTPlanNotice(ctx context.Context, accountID string) (dto.AIConfig, error) {
	return s.core.AckChatGPTPlanNotice(ctx, accountID)
}

// RemoveChatGPTAccount はログアウト済みの ChatGPT アカウントの登録を忘れる。
func (s *AIService) RemoveChatGPTAccount(ctx context.Context, accountID string) (dto.AIConfig, error) {
	return s.core.RemoveChatGPTAccount(ctx, accountID)
}

// ListAIModels は渡された送信先（openai は baseURL）のモデル一覧を、保存前でも取得する。取得できないときは Reason つきの空の一覧を返す。
func (s *AIService) ListAIModels(ctx context.Context, provider, baseURL string) (dto.AIModelList, error) {
	return s.core.ListAIModels(ctx, provider, baseURL)
}

// GenerateReplyDraft は reply／reply_all セッションのスレッドから下書きを生成する。ctx キャンセルは CANCELLED。
// instruction は任意の「AI への指示」（空・空白だけは指示なし。2,000 文字まで。保存しない）。
func (s *AIService) GenerateReplyDraft(ctx context.Context, sessionID, requestID, instruction string) (dto.ReplyDraft, error) {
	return s.core.GenerateReplyDraft(ctx, sessionID, requestID, instruction)
}

// CancelGenerate は実行中の生成を中止する（GenerateReplyDraft は CANCELLED で終わる）。
func (s *AIService) CancelGenerate(requestID string) error { return s.core.CancelGenerate(requestID) }
