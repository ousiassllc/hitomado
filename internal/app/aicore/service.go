// Package aicore は AI 設定・同意・返信下書き生成の実装（Service の本体）。
package aicore

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

// AI プロバイダ名（API §6 の AIProvider）。
const (
	providerAnthropic = "anthropic"
	providerOpenAI    = "openai"
	providerChatGPT   = "chatgpt" // Sign in with ChatGPT（利用者自身の ChatGPT プラン。Responses API）
	// providerTypeSafe は TypeSafe AI（Jev。#314）。文章を書かない分類用のモデルで、Inbox Zero の判定だけに使える（モデルは選ばない）。
	// 「AI で下書き」・要約・質問には使えない（validProvider の外）。API キーと同意（件名・差出人・本文の先頭の送信）は持つ（validSendProvider）。
	providerTypeSafe = "typesafe"
)

// aiGenerateTimeout はプロバイダ呼び出しのタイムアウト（API §6）。
const aiGenerateTimeout = 60 * time.Second

// cancelRecordTTL は、登録前に届いた中止要求を覚えておく既定の時間。requestID は呼び出しごとに一意なので、
// 生成の呼び出しより少し先に中止が届く窓（と、完了後に届いた中止を忘れるまで）を覆えば足りる。
const cancelRecordTTL = time.Minute

// SessionLookup は作成セッションの種別・元スレッド・送信元の参照（composecore.Service が実装する）。
type SessionLookup interface {
	ThreadRef(sessionID string) (model.Mode, dto.ThreadRef, error)
	// SenderOf は作成セッションの種別と送信元アカウントの ID（未選択は ""）を返す（新規作成の AI 下書き。#365）。
	SenderOf(sessionID string) (model.Mode, string, error)
}

// Service は AI 設定・同意・下書き生成（バインディング仕様 §6）。
type Service struct {
	d        *deps.Deps
	sessions SessionLookup

	timeout time.Duration

	cancelTTL time.Duration

	mu       sync.Mutex
	inflight map[string]context.CancelCauseFunc // requestID → cancel
	// cancelled は、実行の登録より前に CancelGenerate が届いた requestID → 中止要求の時刻。
	// begin が見つけたら消費して CANCELLED にする。cancelTTL を過ぎた記録は捨てる（begin・CancelGenerate のたびに掃除する）。
	cancelled map[string]time.Time
}

// Option は Service の任意の設定。
type Option func(*Service)

// WithTimeout はプロバイダ呼び出しのタイムアウトを変える（既定は 60 秒。テストで短縮するのに使う）。
func WithTimeout(d time.Duration) Option { return func(s *Service) { s.timeout = d } }

// WithCancelRecordTTL は、登録前に届いた中止要求を覚えておく時間を変える（既定は 1 分。テストで短縮するのに使う）。
func WithCancelRecordTTL(d time.Duration) Option { return func(s *Service) { s.cancelTTL = d } }

// New は Service を作る。返信系セッションの参照に sessions を使う。
func New(d *deps.Deps, sessions SessionLookup, opts ...Option) *Service {
	s := &Service{d: d, sessions: sessions, timeout: aiGenerateTimeout, cancelTTL: cancelRecordTTL,
		inflight: map[string]context.CancelCauseFunc{}, cancelled: map[string]time.Time{}}
	for _, o := range opts {
		o(s)
	}
	return s
}

// validProvider は文章を書く AI（「AI で下書き」・要約・質問）に使えるプロバイダか（モデル一覧もこの範囲）。
func validProvider(p string) bool {
	return p == providerAnthropic || p == providerOpenAI || p == providerChatGPT
}

// validSendProvider はメールの内容の送信先になり得るプロバイダ（API キーの登録・同意の対象）か。validProvider に TypeSafe AI を足したもの。
func validSendProvider(p string) bool {
	return validProvider(p) || p == providerTypeSafe
}

// apiKey は資格情報ストアの API キー。未設定は ok=false。
func (s *Service) apiKey(provider string) (key string, ok bool, err error) {
	key, err = s.d.Secrets.Get(secrets.AIAPIKeyKey(provider))
	if errors.Is(err, secrets.ErrNotFound) || (err == nil && key == "") {
		return "", false, nil
	}
	if err != nil {
		return "", false, apperr.Wrap(apperr.Internal, "資格情報ストアから API キーを読み込めません。", err)
	}
	return key, true, nil
}

func (s *Service) config(ctx context.Context) (dto.AIConfig, error) {
	conn, err := s.loadConnection(ctx)
	if err != nil {
		return dto.AIConfig{}, err
	}
	features, err := s.loadFeatures(ctx)
	if err != nil {
		return dto.AIConfig{}, err
	}
	consented, err := s.loadConsent(ctx, conn.OpenAIBaseURL)
	if err != nil {
		return dto.AIConfig{}, err
	}
	gpt, err := s.chatGPTStatus(ctx)
	if err != nil {
		return dto.AIConfig{}, err
	}
	keyed := []string{}
	for _, p := range []string{providerAnthropic, providerOpenAI, providerTypeSafe} {
		_, ok, err := s.apiKey(p)
		if err != nil {
			return dto.AIConfig{}, err
		}
		if ok {
			keyed = append(keyed, p)
		}
	}
	return dto.AIConfig{
		BaseURL: conn.OpenAIBaseURL, Features: features.dto(),
		APIKeyProviders: keyed, ConsentedProviders: consented,
		ChatGPTSignedIn: gpt.SignedIn, ChatGPTEmail: gpt.Email, ChatGPTAccounts: chatGPTAccounts(gpt),
	}, nil
}

// GetAIConfig は AI 設定（つなぎ先の状態と、機能ごとに使う AI）を返す（API キー自体は返さない）。
func (s *Service) GetAIConfig(ctx context.Context) (dto.AIConfig, error) { return s.config(ctx) }

func errBadProvider() error {
	return apperr.New(apperr.Validation, "AI プロバイダが正しくありません（anthropic・openai・chatgpt・typesafe のいずれか）。")
}

// errChatGPTNoKey は chatgpt に API キー操作が来たときのエラー（ChatGPT ではログインでトークンを得る）。
func errChatGPTNoKey() error {
	return apperr.New(apperr.Validation, "ChatGPT のプランは API キーを使いません。「Continue with ChatGPT」でログインしてください。")
}

// SetAIApiKey は API キーを資格情報ストアへ保存する（NF-S01）。キーはログに出さない。
func (s *Service) SetAIApiKey(ctx context.Context, provider, apiKey string) (dto.AIConfig, error) {
	if !validSendProvider(provider) {
		return dto.AIConfig{}, errBadProvider()
	}
	if provider == providerChatGPT {
		return dto.AIConfig{}, errChatGPTNoKey()
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return dto.AIConfig{}, apperr.New(apperr.Validation, "API キーを入力してください。")
	}
	if err := s.d.Secrets.Set(secrets.AIAPIKeyKey(provider), apiKey); err != nil {
		return dto.AIConfig{}, apperr.Wrap(apperr.Internal, "API キーを資格情報ストアへ保存できません。", err)
	}
	return s.config(ctx)
}

// ClearAIApiKey は API キーを削除する。
func (s *Service) ClearAIApiKey(ctx context.Context, provider string) (dto.AIConfig, error) {
	if !validSendProvider(provider) {
		return dto.AIConfig{}, errBadProvider()
	}
	if provider == providerChatGPT {
		return dto.AIConfig{}, errChatGPTNoKey()
	}
	if err := s.d.Secrets.Delete(secrets.AIAPIKeyKey(provider)); err != nil {
		return dto.AIConfig{}, apperr.Wrap(apperr.Internal, "API キーを削除できません。", err)
	}
	return s.config(ctx)
}
