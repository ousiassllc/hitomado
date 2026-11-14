package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/store/db"
)

// AgentItem.Status の値（エージェントモード。#372）。dto.AgentStatus* と同じ文字列。
const (
	AgentStatusReady   = "ready"
	AgentStatusAsking  = "asking"
	AgentStatusSkipped = "skipped"
	AgentStatusLater   = "later"
	AgentStatusDone    = "done"
	AgentStatusFailed  = "failed"
)

// AgentItem はエージェントモードが扱った新着 1 件（agent_items の 1 行）。
// 本文・下書きの本文（冒頭のプレビューを含む）・利用者の回答は持たない。DraftID は Gmail の下書きの ID。
type AgentItem struct {
	ID         string
	AccountID  string
	ThreadID   string
	MessageID  string
	Status     string
	Subject    string
	FromLabel  string
	ReasonKind string
	Reason     string
	DraftID    string
	To         []model.Address
	Questions  []string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CreateAgentItem は行を作る。(AccountID, ThreadID, MessageID) の重複は ErrDuplicate、アカウントが無ければ ErrNotFound。
// ID が空なら採番する。CreatedAt・UpdatedAt が 0 なら現在時刻。
func (s *sqlStore) CreateAgentItem(ctx context.Context, it AgentItem) (AgentItem, error) {
	if it.ID == "" {
		it.ID = NewID()
	}
	it.CreatedAt = nowOr(it.CreatedAt, s.now())
	it.UpdatedAt = nowOr(it.UpdatedAt, it.CreatedAt)
	toJSON, qJSON, err := marshalAgentLists(it)
	if err != nil {
		return AgentItem{}, err
	}
	err = s.q.InsertAgentItem(ctx, db.InsertAgentItemParams{
		ID: it.ID, AccountID: it.AccountID, ThreadID: it.ThreadID, MessageID: it.MessageID, Status: it.Status,
		Subject: it.Subject, FromLabel: it.FromLabel, ReasonKind: it.ReasonKind, Reason: it.Reason,
		DraftID: it.DraftID, ToJson: toJSON, QuestionsJson: qJSON,
		CreatedAt: toMillis(it.CreatedAt), UpdatedAt: toMillis(it.UpdatedAt),
	})
	switch {
	case isUnique(err):
		return AgentItem{}, fmt.Errorf("store: create agent item: %w", ErrDuplicate)
	case isForeignKey(err):
		return AgentItem{}, fmt.Errorf("store: create agent item: account: %w", ErrNotFound)
	case err != nil:
		return AgentItem{}, fmt.Errorf("store: create agent item: %w", err)
	}
	return it, nil
}

// UpdateAgentItem は it.ID の行の状態（Status・理由・下書き・宛先・質問）を更新し、UpdatedAt を現在時刻にする。無ければ ErrNotFound。
func (s *sqlStore) UpdateAgentItem(ctx context.Context, it AgentItem) (AgentItem, error) {
	toJSON, qJSON, err := marshalAgentLists(it)
	if err != nil {
		return AgentItem{}, err
	}
	n, err := s.q.UpdateAgentItem(ctx, db.UpdateAgentItemParams{
		ID: it.ID, Status: it.Status, ReasonKind: it.ReasonKind, Reason: it.Reason,
		DraftID: it.DraftID, ToJson: toJSON, QuestionsJson: qJSON,
		UpdatedAt: toMillis(s.now()),
	})
	if err := requireRows(n, err); err != nil {
		return AgentItem{}, fmt.Errorf("store: update agent item: %w", err)
	}
	return s.GetAgentItem(ctx, it.ID)
}

func (s *sqlStore) GetAgentItem(ctx context.Context, id string) (AgentItem, error) {
	r, err := s.q.GetAgentItem(ctx, id)
	if err != nil {
		return AgentItem{}, fmt.Errorf("store: get agent item: %w", notFoundIfNoRows(err))
	}
	return agentItemOf(r)
}

// ListAgentItems は新しい順（UpdatedAt 降順）に返す。
func (s *sqlStore) ListAgentItems(ctx context.Context) ([]AgentItem, error) {
	rows, err := s.q.ListAgentItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list agent items: %w", err)
	}
	out := make([]AgentItem, 0, len(rows))
	for _, r := range rows {
		it, err := agentItemOf(r)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, nil
}

func (s *sqlStore) AgentItemExists(ctx context.Context, accountID, threadID, messageID string) (bool, error) {
	found, err := s.q.AgentItemExists(ctx, db.AgentItemExistsParams{AccountID: accountID, ThreadID: threadID, MessageID: messageID})
	if err != nil {
		return false, fmt.Errorf("store: agent item exists: %w", err)
	}
	return found, nil
}

// CountPendingAgentItems は確認待ち（ready）と質問待ち（asking）の合計。
func (s *sqlStore) CountPendingAgentItems(ctx context.Context) (int, error) {
	n, err := s.q.CountPendingAgentItems(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: count pending agent items: %w", err)
	}
	return int(n), nil
}

// DeleteFinishedAgentItemsBefore は UpdatedAt が cutoff より古い done／skipped／failed の行を削除し、件数を返す。
func (s *sqlStore) DeleteFinishedAgentItemsBefore(ctx context.Context, cutoff time.Time) (int, error) {
	n, err := s.q.DeleteFinishedAgentItemsBefore(ctx, toMillis(cutoff))
	if err != nil {
		return 0, fmt.Errorf("store: delete finished agent items: %w", err)
	}
	return int(n), nil
}

func marshalAgentLists(it AgentItem) (toJSON, questionsJSON string, err error) {
	to, questions := it.To, it.Questions
	if to == nil {
		to = []model.Address{}
	}
	if questions == nil {
		questions = []string{}
	}
	tb, err := json.Marshal(to)
	if err != nil {
		return "", "", fmt.Errorf("store: marshal agent item: %w", err)
	}
	qb, err := json.Marshal(questions)
	if err != nil {
		return "", "", fmt.Errorf("store: marshal agent item: %w", err)
	}
	return string(tb), string(qb), nil
}

func agentItemOf(r db.AgentItem) (AgentItem, error) {
	it := AgentItem{
		ID: r.ID, AccountID: r.AccountID, ThreadID: r.ThreadID, MessageID: r.MessageID, Status: r.Status,
		Subject: r.Subject, FromLabel: r.FromLabel, ReasonKind: r.ReasonKind, Reason: r.Reason,
		DraftID:   r.DraftID,
		CreatedAt: fromMillis(r.CreatedAt), UpdatedAt: fromMillis(r.UpdatedAt),
	}
	if err := json.Unmarshal([]byte(r.ToJson), &it.To); err != nil {
		return AgentItem{}, fmt.Errorf("store: read agent item: %w", err)
	}
	if err := json.Unmarshal([]byte(r.QuestionsJson), &it.Questions); err != nil {
		return AgentItem{}, fmt.Errorf("store: read agent item: %w", err)
	}
	return it, nil
}
