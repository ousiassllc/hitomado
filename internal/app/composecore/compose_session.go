package composecore

import (
	"log/slog"
	"time"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// composeSessionTTL は作成セッションの最終操作からの自動破棄までの時間（API §9）。
const composeSessionTTL = 24 * time.Hour

// sessionAttachment は作成セッションが保持する添付。ローカル添付は Path、転送の元添付は
// MessageID＋GmailAttID を持つ（内容は送信時に読む）。
type sessionAttachment struct {
	ID        string
	Filename  string
	MimeType  string
	Size      int64
	Source    string    // dto 側の "local" / "forwarded"
	Path      string    // local のみ
	ModTime   time.Time // local のみ（追加時の更新時刻。送信時に同一性を検査する）
	MessageID string    // forwarded のみ（元メッセージの ID）
	GmailAtt  string    // forwarded のみ（Gmail の attachmentId）
}

const (
	attachmentLocal     = "local"
	attachmentForwarded = "forwarded"
)

// composeSession は 1 つの作成セッション（メモリのみ）。フィールドは Service.mu で保護する。
type composeSession struct {
	id   string
	mode model.Mode

	accountID string        // 送信元（未選択は ""）
	from      model.Address // 送信元の表示用スナップショット（accounts.email 由来）

	thread          *dto.ThreadRef // reply 系・forward の元スレッド。new は nil
	threadAccountID string         // スレッドの所有アカウント（SENDER_MISMATCH 検査用。作成時に固定）
	inReplyTo       string         // reply 系のみ
	references      []string       // reply 系のみ
	sendThreadID    string         // Gmail へ渡す threadId（reply 系のみ）

	attachments []sessionAttachment
	draftID     string
	draftAcctID string // draftID が存在する Gmail アカウント

	lastContent dto.ComposeContent // 最後に分かっている入力内容（SetComposeSender の応答用）
	archive     bool               // 直近の PrepareSend の archiveAfterSend（Send のハッシュに含める）

	sending  bool
	lastUsed time.Time
	// io は SaveDraft／DiscardCompose／Send の Gmail 呼び出しを直列化する（下書きの二重作成を防ぐ）。
	io chanMutex
}

// chanMutex は ctx を待てるロック（チャネル版 Mutex）。Wails の呼び出し取消でブロックし続けないため。
type chanMutex chan struct{}

func newChanMutex() chanMutex { return make(chanMutex, 1) }

func (m chanMutex) lock(done <-chan struct{}) bool {
	select {
	case m <- struct{}{}:
		return true
	case <-done:
		return false
	}
}

func (m chanMutex) unlock() { <-m }

// clone は I/O をロック外で行うための値コピー（スライスも複製する）。
func (cs *composeSession) clone() composeSession {
	c := *cs
	c.references = append([]string(nil), cs.references...)
	c.attachments = append([]sessionAttachment(nil), cs.attachments...)
	return c
}

func (cs *composeSession) attachmentDTOs() []dto.ComposeAttachment {
	out := make([]dto.ComposeAttachment, 0, len(cs.attachments))
	for _, a := range cs.attachments {
		out = append(out, dto.ComposeAttachment{ID: a.ID, Filename: a.Filename, Size: a.Size, Source: a.Source})
	}
	return out
}

func (cs *composeSession) attachmentsTotal() int64 {
	var n int64
	for _, a := range cs.attachments {
		n += a.Size
	}
	return n
}

// toDTO はセッションを DTO にする（From は accounts.email 由来のスナップショットのみ）。
func (cs *composeSession) toDTO(content dto.ComposeContent) dto.ComposeSession {
	out := dto.ComposeSession{
		SessionID:    cs.id,
		Mode:         string(cs.mode),
		AccountID:    cs.accountID,
		SenderLocked: cs.mode != model.ModeNew,
		Content:      content,
		Attachments:  cs.attachmentDTOs(),
		DraftID:      cs.draftID,
	}
	if cs.accountID != "" {
		from := cs.from
		out.From = &from
	}
	return out
}

func (s *Service) log() *slog.Logger {
	if s.d.Logger != nil {
		return s.d.Logger
	}
	return slog.Default()
}

// expireLocked は最終操作から 24 時間を過ぎたセッションを破棄する（s.mu を保持して呼ぶ）。送信中は残す。
func (s *Service) expireLocked() {
	now := s.d.Clock.Now()
	for id, cs := range s.sessions {
		if !cs.sending && now.Sub(cs.lastUsed) >= composeSessionTTL {
			delete(s.sessions, id)
			s.confirm.Invalidate(id)
		}
	}
}

func errSessionNotFound() error {
	return apperr.New(apperr.SessionNotFound, "作成セッションが見つかりません。もう一度開いてください。")
}

func errSessionBusy() error {
	return apperr.New(apperr.Validation, "送信処理中のため操作できません。")
}

// getLocked は期限切れを破棄してセッションを返し、最終操作時刻を更新する（s.mu を保持して呼ぶ）。
func (s *Service) getLocked(id string) (*composeSession, error) {
	s.expireLocked()
	cs, ok := s.sessions[id]
	if !ok {
		return nil, errSessionNotFound()
	}
	cs.lastUsed = s.d.Clock.Now()
	return cs, nil
}

// mutate はセッションを排他的に更新する（Gmail などの I/O は fn 内で行わない）。送信中は拒否する。
func (s *Service) mutate(id string, fn func(*composeSession) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cs, err := s.getLocked(id)
	if err != nil {
		return err
	}
	if cs.sending {
		return errSessionBusy()
	}
	return fn(cs)
}

// snapshot はセッションの値コピーを返す（I/O をロック外で行うため）。送信中は拒否する。
func (s *Service) snapshot(id string) (*composeSession, composeSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cs, err := s.getLocked(id)
	if err != nil {
		return nil, composeSession{}, err
	}
	if cs.sending {
		return nil, composeSession{}, errSessionBusy()
	}
	return cs, cs.clone(), nil
}

// drop はセッションとその確認トークンを破棄する。
func (s *Service) drop(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
	s.confirm.Invalidate(id)
}

// ThreadRef は返信系セッションの種別と元スレッドを返す（AIService 用）。new は ref が nil。
func (s *Service) ThreadRef(id string) (model.Mode, dto.ThreadRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cs, err := s.getLocked(id)
	if err != nil {
		return "", dto.ThreadRef{}, err
	}
	if cs.thread == nil {
		return cs.mode, dto.ThreadRef{}, nil
	}
	return cs.mode, *cs.thread, nil
}
