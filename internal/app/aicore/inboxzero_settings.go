package aicore

import (
	"context"
	"encoding/json"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/store"
)

// defaultInboxZeroReasons は未設定（と壊れた保存値）のときの設定。4 つの理由すべてがオン。
func defaultInboxZeroReasons() dto.InboxZeroReasons {
	return dto.InboxZeroReasons{Ad: true, Notification: true, Newsletter: true, Replied: true}
}

// loadInboxZeroReasons は保存済みの理由ごとのオン・オフを返す。未設定・壊れた値は全部オン。
// 保存値に無いキーはオンのまま読む（理由を足しても、古い保存値でその理由が消えない）。
func (s *Service) loadInboxZeroReasons(ctx context.Context) (dto.InboxZeroReasons, error) {
	reasons := defaultInboxZeroReasons()
	v, ok, err := s.d.Store.GetSetting(ctx, store.SettingInboxZero)
	if err != nil {
		return reasons, apperr.FromInfra(err, "")
	}
	if !ok {
		return reasons, nil
	}
	if json.Unmarshal([]byte(v), &reasons) != nil {
		return defaultInboxZeroReasons(), nil
	}
	return reasons, nil
}

// GetInboxZeroSettings は Inbox Zero の設定を返す。未設定は全部オン。
func (s *Service) GetInboxZeroSettings(ctx context.Context) (dto.InboxZeroSettings, error) {
	reasons, err := s.loadInboxZeroReasons(ctx)
	return dto.InboxZeroSettings{Reasons: reasons}, err
}

// SetInboxZeroReasons は「不要なメール」の理由ごとのオン・オフを保存し、保存後の設定を返す（全部オフも保存できる。
// その場合 FindInboxZeroCandidates は VALIDATION になる）。
func (s *Service) SetInboxZeroReasons(ctx context.Context, reasons dto.InboxZeroReasons) (dto.InboxZeroSettings, error) {
	b, err := json.Marshal(reasons)
	if err != nil {
		return dto.InboxZeroSettings{}, apperr.Wrap(apperr.Internal, "Inbox Zero の設定を保存できません。", err)
	}
	if err := s.d.Store.SetSetting(ctx, store.SettingInboxZero, string(b)); err != nil {
		return dto.InboxZeroSettings{}, apperr.FromInfra(err, "")
	}
	return dto.InboxZeroSettings{Reasons: reasons}, nil
}

// InboxZeroAPI は Inbox Zero（#314）のバインディング面（AIService が埋め込んで公開する。実装は Service）。
type InboxZeroAPI interface {
	// GetInboxZeroSettings は Inbox Zero の設定（「不要なメール」の理由ごとのオン・オフ）を返す。未設定は全部オン（#314）。
	GetInboxZeroSettings(ctx context.Context) (dto.InboxZeroSettings, error)
	// SetInboxZeroReasons は「不要なメール」の理由ごとのオン・オフを保存する（広告・通知・ニュースレター・返信済み）。
	SetInboxZeroReasons(ctx context.Context, reasons dto.InboxZeroReasons) (dto.InboxZeroSettings, error)
	// FindInboxZeroCandidates は refs（読み込み済みの一覧。重複を除いて 100 件まで）から、オンの理由に当てはまるアーカイブ候補を返す。
	// 返信済みは端末内で判定し、広告・通知・ニュースレターは TypeSafe AI（件名・差出人・本文の先頭 500 文字を送る。キーと同意が前提）で判定する。
	// 中止は CancelGenerate（CANCELLED）。アーカイブは行わない（InboxService.Archive を使う）。
	FindInboxZeroCandidates(ctx context.Context, refs []dto.ThreadRef, requestID string) (dto.InboxZeroResult, error)
}

var _ InboxZeroAPI = (*Service)(nil)
