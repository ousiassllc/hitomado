package composecore

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/confirm"
	"github.com/ousiassllc/hitomado/internal/domain/contacts"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/senderpolicy"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// confirmTTL は送信確認トークンの有効期間（API §5.2）。
const confirmTTL = 5 * time.Minute

// 宛先候補の件数の範囲（API §5 SuggestContacts）。
const (
	minSuggestLimit = 1
	maxSuggestLimit = 20
)

// Service は作成セッション・下書き・送信前確認・送信・宛先候補（バインディング仕様 §5）。
// セッションはメモリのみで、最終操作から 24 時間で操作時に破棄する。
type Service struct {
	d       *deps.Deps
	confirm *confirm.Manager

	mu       sync.Mutex
	sessions map[string]*composeSession
}

// New は Service を作る。
func New(d *deps.Deps) *Service {
	return &Service{
		d:        d,
		confirm:  confirm.NewManager(d.Clock, confirmTTL),
		sessions: map[string]*composeSession{},
	}
}

// account は accounts から送信元アカウントを取得する。
func (s *Service) account(ctx context.Context, id string) (store.Account, error) {
	a, err := s.d.Store.GetAccount(ctx, id)
	if err != nil {
		return store.Account{}, apperr.FromInfra(err, id)
	}
	return a, nil
}

// SetComposeSender は新規作成の送信元を設定する。返信・転送は SENDER_LOCKED（NF-S11）。
// 応答の content は Go が最後に把握している入力内容で、署名の差し替えはフロントが行う。
func (s *Service) SetComposeSender(ctx context.Context, sessionID, accountID string) (dto.ComposeSession, error) {
	_, view, err := s.snapshot(sessionID)
	if err != nil {
		return dto.ComposeSession{}, err
	}
	if err := senderpolicy.CanChangeSender(view.mode); err != nil {
		return dto.ComposeSession{}, apperr.Wrap(apperr.SenderLocked, "返信・転送では送信元を変更できません。", err)
	}
	acct, err := s.account(ctx, accountID)
	if err != nil {
		return dto.ComposeSession{}, err
	}
	var out dto.ComposeSession
	err = s.mutate(sessionID, func(cs *composeSession) error {
		cs.accountID = acct.ID
		cs.from = model.Address{Name: foldLine(acct.DisplayName), Email: acct.Email}
		out = cs.toDTO(cs.lastContent)
		return nil
	})
	return out, err
}

// CloseCompose はセッションを破棄する（下書きは残す）。存在しないセッションは何もしない。
func (s *Service) CloseCompose(_ context.Context, sessionID string) error {
	s.mu.Lock()
	cs, ok := s.sessions[sessionID]
	if ok && cs.sending {
		s.mu.Unlock()
		return errSessionBusy()
	}
	s.expireLocked()
	s.mu.Unlock()
	s.drop(sessionID)
	return nil
}

// DiscardCompose はセッションを破棄し、Gmail の下書きがあれば削除する。
// 下書きの削除に失敗したときはセッションを残し、エラーを返す（再試行できる）。
func (s *Service) DiscardCompose(ctx context.Context, sessionID string) error {
	defer s.d.Busy.Begin()() // 下書きの削除中は自動アップデートの入れ替えをしない
	cs, view, err := s.snapshot(sessionID)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) && ae.Code == apperr.SessionNotFound {
			return nil
		}
		return err
	}
	if !cs.io.lock(ctx.Done()) {
		return apperr.FromInfra(ctx.Err(), "")
	}
	defer cs.io.unlock()
	// ロック待ちの間に SaveDraft が下書きを作った可能性があるので最新の値を読み直す。
	s.mu.Lock()
	draftID, draftAcct := cs.draftID, cs.draftAcctID
	s.mu.Unlock()
	if draftID != "" {
		if err := s.d.Gmail.DeleteDraft(ctx, draftAcct, draftID); err != nil && !errors.Is(err, gmail.ErrNotFound) {
			return apperr.FromInfra(err, draftAcct)
		}
		s.invalidateThread(draftAcct, view.sendThreadID) // 削除した下書きをスレッド表示に残さない
	}
	s.drop(sessionID)
	return nil
}

// SuggestContacts は送信元アカウントごとの宛先候補を返す（F-13）。prefix が空なら空配列。
func (s *Service) SuggestContacts(ctx context.Context, accountID, prefix string, limit int) ([]model.Address, error) {
	out := []model.Address{}
	p := contacts.Normalize(prefix)
	if p == "" {
		return out, nil
	}
	limit = min(max(limit, minSuggestLimit), maxSuggestLimit)
	found, err := s.d.Store.SuggestContacts(ctx, accountID, p, limit)
	if err != nil {
		return nil, apperr.FromInfra(err, accountID)
	}
	for _, c := range found {
		out = append(out, model.Address{Name: c.DisplayName, Email: c.Email})
	}
	return out, nil
}
