package agentcore

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
)

// 下書きのプレビュー（本文の最初の数行。プレーンテキスト）の上限（保存先はメモリだけ。previews.go）。
const (
	previewLines = 3
	previewRunes = 140
)

// 画面に出す失敗の理由（固定の文面。エラーの内容は載せない）。
const (
	reasonJudgeFailed = "返信が要るか判断できませんでした"
	reasonDraftFailed = "返信の下書きを作れませんでした"
)

// OnNewMail は同期の新着の観測口（syncer.NewMailObserver）。エージェントがオフ・AI が使えないときは何もしない
// （スレッド取得も AI 呼び出しもしない）。オンなら待ち行列に積むだけで、待たない（同期のゴルーチンから呼ばれるため）。
func (s *Service) OnNewMail(ctx context.Context, accountID string, cands []syncer.NewMailCandidate) {
	if !s.active(ctx) {
		return
	}
	dropped := 0
	for _, c := range cands {
		select {
		case s.queue <- work{accountID: accountID, threadID: c.ThreadID, messageID: c.MessageID}:
		default:
			dropped++
		}
	}
	if dropped > 0 {
		s.log.Warn("agent: queue full; dropped new mail", "account", accountID, "count", dropped)
	}
}

// active はエージェントがオンで、AI が使える状態か。確かめられないときはログだけ残して偽。
func (s *Service) active(ctx context.Context) bool {
	cfg, err := s.loadSettings(ctx)
	if err != nil {
		s.log.Warn("agent: load settings failed", "kind", errKind(err))
		return false
	}
	if !cfg.Enabled {
		return false
	}
	ready, err := s.d.AI.Ready(ctx)
	if err != nil {
		s.log.Warn("agent: ai readiness check failed", "kind", errKind(err))
		return false
	}
	return ready
}

// Start は古い項目の削除・マスコットの表示の反映をして、待ち行列を 1 件ずつ処理するワーカーを起動する（冪等）。
// ワーカーは ctx の終了で止まる。完了待ちは Wait。
func (s *Service) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		if n, err := s.d.Store.DeleteFinishedAgentItemsBefore(ctx, s.d.Clock.Now().Add(-retention)); err != nil {
			s.log.Warn("agent: purge old items failed", "kind", errKind(err))
		} else if n > 0 {
			s.log.Info("agent: purged old items", "count", n)
		}
		if cfg, err := s.loadSettings(ctx); err != nil {
			s.log.Warn("agent: load settings failed", "kind", errKind(err))
		} else {
			s.applyMascot(cfg)
		}
		s.wg.Add(1)
		go s.runWorker(ctx)
	})
}

// Wait はワーカーの終了を待つ（ctx のキャンセルのあとに呼ぶ）。
func (s *Service) Wait() { s.wg.Wait() }

func (s *Service) runWorker(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case w := <-s.queue:
			s.processSafe(ctx, w)
		}
	}
}

func (s *Service) processSafe(ctx context.Context, w work) {
	defer logging.Recover(s.log, "agent process")
	s.process(ctx, w)
}

// process は新着 1 通を、判断→（返信が要るなら）質問または下書きづくりまで進めて、結果を保存する。
func (s *Service) process(ctx context.Context, w work) {
	if !s.active(ctx) { // 待っている間にオフにされた・AI が使えなくなった
		return
	}
	exists, err := s.d.Store.AgentItemExists(ctx, w.accountID, w.threadID, w.messageID)
	if err != nil {
		s.log.Warn("agent: lookup failed", "account", w.accountID, "kind", errKind(err))
		return
	}
	if exists {
		return // 冪等
	}
	row, err := s.d.Store.GetThread(ctx, w.accountID, w.threadID)
	if err != nil || !row.InInbox {
		return // 消えた・受信箱から外された（アーカイブ済みなど）
	}
	ref := dto.ThreadRef{AccountID: w.accountID, ThreadID: w.threadID}
	if s.hasUserDraft(ctx, ref) {
		return
	}
	item := store.AgentItem{AccountID: w.accountID, ThreadID: w.threadID, MessageID: w.messageID, Subject: row.Subject, FromLabel: addressLabel(row.LastFrom.Name, row.LastFrom.Email)}

	j, err := s.d.AI.JudgeThread(ctx, ref)
	if err != nil {
		if ctx.Err() != nil {
			return // 終了による中断。行は作らず、次の新着に影響しない
		}
		s.log.Warn("agent: judge failed", "account", w.accountID, "thread", w.threadID, "kind", errKind(err))
		s.createFailed(ctx, item, errKind(err), reasonJudgeFailed)
		return
	}
	s.log.Info("agent: judged", "account", w.accountID, "thread", w.threadID, "needsReply", j.NeedsReply, "reasonKind", j.ReasonKind, "questions", len(j.Questions))
	item.ReasonKind, item.Reason = j.ReasonKind, j.Reason
	switch {
	case !j.NeedsReply:
		item.Status = store.AgentStatusSkipped
		s.create(ctx, item)
	case len(j.Questions) > 0:
		item.Status, item.Questions = store.AgentStatusAsking, j.Questions
		s.create(ctx, item)
	default:
		s.createDrafted(ctx, item)
	}
}

// hasUserDraft は元のスレッドに下書きがすでにあるか（あれば利用者が自分で返信を書き始めているので、エージェントは触らない）。
// 確かめられないときは偽（判断・下書きづくりを続ける）。
func (s *Service) hasUserDraft(ctx context.Context, ref dto.ThreadRef) bool {
	thread, err := s.d.Threads.Get(ctx, ref.AccountID, ref.ThreadID)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(thread.Messages, func(m gmail.MessageFull) bool { return slices.Contains(m.LabelIDs, "DRAFT") })
}

// buildDraft は返信本文を AI で作って Gmail の下書きにし、item を ready にする（保存はしない）。失敗したときは item を変えない。
func (s *Service) buildDraft(ctx context.Context, item *store.AgentItem, instruction string) error {
	ref := dto.ThreadRef{AccountID: item.AccountID, ThreadID: item.ThreadID}
	text, err := s.d.AI.DraftReplyText(ctx, ref, instruction)
	if err != nil {
		return err
	}
	draftID, to, err := s.d.Drafts.CreateReplyDraft(ctx, ref, text)
	if err != nil {
		return err
	}
	item.Status, item.DraftID, item.To, item.Questions = store.AgentStatusReady, draftID, to, nil
	s.previews.put(item.ID, draftID, preview(text))
	s.log.Info("agent: draft created", "account", item.AccountID, "thread", item.ThreadID)
	return nil
}

// createDrafted は新着の返信下書きを作り、ready として保存する。失敗したときは failed として保存する。
func (s *Service) createDrafted(ctx context.Context, item store.AgentItem) {
	item.ID = store.NewID() // プレビューのキャッシュのキー（下書きを作る前に決めておく）
	if err := s.buildDraft(ctx, &item, ""); err != nil {
		if ctx.Err() != nil {
			return // 終了による中断
		}
		s.log.Warn("agent: draft failed", "account", item.AccountID, "thread", item.ThreadID, "kind", errKind(err))
		s.createFailed(ctx, item, errKind(err), reasonDraftFailed)
		return
	}
	s.create(ctx, item)
}

// createFailed は item を failed として保存する。
func (s *Service) createFailed(ctx context.Context, item store.AgentItem, kind, reason string) {
	item.Status, item.ReasonKind, item.Reason = store.AgentStatusFailed, kind, reason
	s.create(ctx, item)
}

// create は行を作って agent:updated を発行する。同じ新着がすでにあれば（冪等）何もしない。
func (s *Service) create(ctx context.Context, item store.AgentItem) {
	_, err := s.d.Store.CreateAgentItem(context.WithoutCancel(ctx), item)
	switch {
	case err == nil:
		s.emitUpdatedNow(ctx)
	case errors.Is(err, store.ErrDuplicate), errors.Is(err, store.ErrNotFound):
		// 二重に扱った・アカウントが消えた。何もしない。
	default:
		s.log.Warn("agent: save failed", "account", item.AccountID, "kind", errKind(err))
	}
}

// addressLabel は差出人の表示用（表示名。無ければアドレス）。
func addressLabel(name, email string) string {
	if name != "" {
		return name
	}
	return email
}

// preview は返信本文の最初の数行（空行を除く）を、約 140 文字のプレーンテキストにする。
func preview(text string) string {
	var lines []string
	for l := range strings.SplitSeq(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
			if len(lines) == previewLines {
				break
			}
		}
	}
	out := strings.Join(lines, "\n")
	if utf8.RuneCountInString(out) > previewRunes {
		out = string([]rune(out)[:previewRunes-1]) + "…"
	}
	return out
}

// errKind はエラーを、ログ・画面に出せる固定の種類（エラーコードの小文字。例: ai_failed。AppError でなければ internal）にする。内容は含めない。
func errKind(err error) string {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return strings.ToLower(ae.Code)
	}
	return "internal"
}
