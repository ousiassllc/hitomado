package aicore

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/store"
)

// 機能ごとに使う AI（#376）。選んだ AI・モデルは app_settings の ai.features に機能ごとに持ち、
// その機能の実行（下書き・要約・質問は draft、Inbox Zero は inboxZero）は、ここで選んだものだけを使う。
// つないでいない・外された AI を選んでいても、ほかの AI へ黙って切り替えない（実行は AI_NOT_CONFIGURED で止まる）。

// storedFeature は機能 1 つの選択。Provider が空なら未選択。
type storedFeature struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	// Effort はモデルのエフォート（#111。draft だけ）。「モデルの既定」はキー自体を書かない。範囲外の保存値は読むときに空にする。
	Effort string `json:"effort,omitempty"`
}

// storedFeatures は app_settings の ai.features（ER §3.9）。
type storedFeatures struct {
	Draft     storedFeature `json:"draft"`
	InboxZero storedFeature `json:"inboxZero"`
}

func (f storedFeatures) dto() dto.AIFeatures {
	return dto.AIFeatures{Draft: f.Draft.dto(), InboxZero: f.InboxZero.dto()}
}

func (f storedFeature) dto() dto.AIFeatureSetting {
	return dto.AIFeatureSetting{Provider: f.Provider, Model: f.Model, Effort: f.Effort}
}

// storedConnection は app_settings の ai.connection。つなぎ先の設定で、API キーは持たない（資格情報ストア）。
type storedConnection struct {
	// OpenAIBaseURL は OpenAI 互換の Base URL。空は OpenAI の既定の送信先。openai を選んだ機能すべてがこれを使う。
	OpenAIBaseURL string `json:"openaiBaseUrl,omitempty"`
}

// SelectedAI は 1 回の実行に使う、選んだ AI（プロバイダ・モデル・送信先・エフォート）。
type SelectedAI struct {
	Provider string
	Model    string
	BaseURL  string // openai のみ
	Effort   string
}

// validFeature は機能 ID か。
func validFeature(feature string) bool {
	return feature == dto.AIFeatureDraft || feature == dto.AIFeatureInboxZero
}

// validFeatureProvider は feature で選べるプロバイダか。draft は文章を書く AI だけ、Inbox Zero は TypeSafe AI（Jev）も選べる。
func validFeatureProvider(feature, provider string) bool {
	if feature == dto.AIFeatureInboxZero {
		return validSendProvider(provider)
	}
	return validProvider(provider)
}

func (f *storedFeatures) of(feature string) *storedFeature {
	if feature == dto.AIFeatureInboxZero {
		return &f.InboxZero
	}
	return &f.Draft
}

// loadFeatures は保存済みの選択を返す。未保存・壊れた値は全部未選択。その機能で選べないプロバイダ（手で書き換えられた値）は未選択、
// 範囲外のエフォートは「モデルの既定」として読む。
func (s *Service) loadFeatures(ctx context.Context) (storedFeatures, error) {
	var f storedFeatures
	v, ok, err := s.d.Store.GetSetting(ctx, store.SettingAIFeatures)
	if err != nil {
		return f, apperr.FromInfra(err, "")
	}
	if !ok {
		return f, nil
	}
	if err := json.Unmarshal([]byte(v), &f); err != nil {
		return storedFeatures{}, apperr.Wrap(apperr.Internal, "AI 設定を読み込めません。", err)
	}
	for _, feature := range []string{dto.AIFeatureDraft, dto.AIFeatureInboxZero} {
		sel := f.of(feature)
		if !validFeatureProvider(feature, sel.Provider) {
			*sel = storedFeature{}
			continue
		}
		if feature != dto.AIFeatureDraft || !ai.ValidEffort(sel.Provider, sel.Effort) {
			sel.Effort = ""
		}
	}
	return f, nil
}

// loadConnection はつなぎ先の設定を返す。未保存・壊れた値は既定（OpenAI の既定の送信先）。
func (s *Service) loadConnection(ctx context.Context) (storedConnection, error) {
	var c storedConnection
	v, ok, err := s.d.Store.GetSetting(ctx, store.SettingAIConnection)
	if err != nil {
		return c, apperr.FromInfra(err, "")
	}
	if !ok {
		return c, nil
	}
	if err := json.Unmarshal([]byte(v), &c); err != nil {
		return storedConnection{}, apperr.Wrap(apperr.Internal, "AI のつなぎ先の設定を読み込めません。", err)
	}
	return c, nil
}

// FeatureAI は feature（dto.AIFeatureDraft／dto.AIFeatureInboxZero）で選んだ AI を、送信先（openai は保存済みの Base URL）つきで返す。
// saved は選択があるか（false なら sel は空）。つながっているか・同意済みかは確かめない（確かめて実行するのは precheck）。
// Inbox Zero（#377）の判定は、ここで選ばれた AI・モデルを使う。
func (s *Service) FeatureAI(ctx context.Context, feature string) (sel SelectedAI, saved bool, err error) {
	f, err := s.loadFeatures(ctx)
	if err != nil {
		return SelectedAI{}, false, err
	}
	conn, err := s.loadConnection(ctx)
	if err != nil {
		return SelectedAI{}, false, err
	}
	chosen := *f.of(feature)
	sel = SelectedAI{Provider: chosen.Provider, Model: chosen.Model, Effort: chosen.Effort}
	if sel.Provider == providerOpenAI {
		sel.BaseURL = conn.OpenAIBaseURL
	}
	return sel, chosen.Provider != "", nil
}

// FeaturesAPI は機能ごとに使う AI・つなぎ先の設定（#376）のバインディング面（AIService が埋め込んで公開する。実装は Service）。
type FeaturesAPI interface {
	// SetAIFeature は機能（draft／inboxZero）で使う AI・モデル・エフォートを保存する。
	SetAIFeature(ctx context.Context, feature, provider, model, effort string) (dto.AIConfig, error)
	// SetAIBaseURL は OpenAI 互換の Base URL（つなぎ先の設定）を保存する。
	SetAIBaseURL(ctx context.Context, baseURL string) (dto.AIConfig, error)
}

var _ FeaturesAPI = (*Service)(nil)

// SetAIFeature は feature（draft／inboxZero）で使う AI・モデルを保存し、保存後の設定を返す。
// 選べるプロバイダは draft が anthropic／openai／chatgpt、inboxZero はそれに typesafe を足したもの（範囲外は VALIDATION）。
// typesafe はモデルを持たない（model は無視して空で保存する）。それ以外は model が必須。
// effort（#111）は draft だけで、省略・空は「モデルの既定」（キーを書かない）。指定できるのはプロバイダごとの段階だけで、範囲外は VALIDATION。
// 渡さなければ前のエフォートは引き継がず「既定」に戻る。モデルとエフォートの組み合わせは確かめない（確かめるのは画面。
// 生成時にプロバイダが拒否したら AI_FAILED）。つないでいない AI も保存できる（実行時に AI_NOT_CONFIGURED で止まり、別の AI へは切り替えない）。
func (s *Service) SetAIFeature(ctx context.Context, feature, provider, model, effort string) (dto.AIConfig, error) {
	if !validFeature(feature) {
		return dto.AIConfig{}, apperr.New(apperr.Validation, "AI を使う機能が正しくありません（draft・inboxZero のいずれか）。")
	}
	if !validFeatureProvider(feature, provider) {
		return dto.AIConfig{}, errBadProvider()
	}
	model, effort = strings.TrimSpace(model), strings.TrimSpace(effort)
	switch {
	case provider == providerTypeSafe:
		model = ""
	case model == "":
		return dto.AIConfig{}, apperr.New(apperr.Validation, "モデルを選んでください。")
	}
	if effort != "" && (feature != dto.AIFeatureDraft || !ai.ValidEffort(provider, effort)) {
		return dto.AIConfig{}, apperr.New(apperr.Validation, "エフォートが正しくありません。")
	}
	f, err := s.loadFeatures(ctx)
	if err != nil {
		return dto.AIConfig{}, err
	}
	*f.of(feature) = storedFeature{Provider: provider, Model: model, Effort: effort}
	b, err := json.Marshal(f)
	if err != nil {
		return dto.AIConfig{}, apperr.Wrap(apperr.Internal, "AI 設定を保存できません。", err)
	}
	if err := s.d.Store.SetSetting(ctx, store.SettingAIFeatures, string(b)); err != nil {
		return dto.AIConfig{}, apperr.FromInfra(err, "")
	}
	return s.config(ctx)
}

// SetAIBaseURL は OpenAI 互換の Base URL（つなぎ先の設定）を保存する。空は OpenAI の既定の送信先。http(s) の URL を要する
// （userinfo・query・fragment は拒否）。送信先（scheme＋ホスト）が変わると、そのプロバイダの同意は無効になる。API キーは維持する。
func (s *Service) SetAIBaseURL(ctx context.Context, baseURL string) (dto.AIConfig, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL != "" {
		if err := validateBaseURL(baseURL); err != nil {
			return dto.AIConfig{}, err
		}
	}
	b, err := json.Marshal(storedConnection{OpenAIBaseURL: baseURL})
	if err != nil {
		return dto.AIConfig{}, apperr.Wrap(apperr.Internal, "AI のつなぎ先の設定を保存できません。", err)
	}
	if err := s.d.Store.SetSetting(ctx, store.SettingAIConnection, string(b)); err != nil {
		return dto.AIConfig{}, apperr.FromInfra(err, "")
	}
	if err := s.pruneConsent(ctx, baseURL); err != nil {
		return dto.AIConfig{}, err
	}
	return s.config(ctx)
}
