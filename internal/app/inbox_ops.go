package app

import (
	"context"
	"errors"
	"sync"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

// threadOp は 1 スレッドへの Gmail 操作。
type threadOp func(ctx context.Context, accountID, threadID string) error

// opOutcome は 1 ref の結果。sum は更新後のサマリ（nil で err も nil なら削除済み）。
type opOutcome struct {
	sum *dto.ThreadSummary
	err error
	ref dto.ThreadRef
}

// MarkRead は UNREAD ラベルを付与・除去する。
func (s *InboxService) MarkRead(ctx context.Context, refs []dto.ThreadRef, read bool) ([]dto.ThreadSummary, error) {
	return s.apply(ctx, refs, func(ctx context.Context, accountID, threadID string) error {
		if read {
			return s.d.Gmail.ModifyThread(ctx, accountID, threadID, nil, []string{"UNREAD"})
		}
		return s.d.Gmail.ModifyThread(ctx, accountID, threadID, []string{"UNREAD"}, nil)
	})
}

// Archive は INBOX ラベルを除去する。
func (s *InboxService) Archive(ctx context.Context, refs []dto.ThreadRef) ([]dto.ThreadSummary, error) {
	return s.apply(ctx, refs, func(ctx context.Context, accountID, threadID string) error {
		return s.d.Gmail.ModifyThread(ctx, accountID, threadID, nil, []string{"INBOX"})
	})
}

// Trash はスレッドをゴミ箱へ移す。
func (s *InboxService) Trash(ctx context.Context, refs []dto.ThreadRef) ([]dto.ThreadSummary, error) {
	return s.apply(ctx, refs, s.d.Gmail.TrashThread)
}

// Unarchive は Archive の取り消し。INBOX ラベルを付け直す。
func (s *InboxService) Unarchive(ctx context.Context, refs []dto.ThreadRef) ([]dto.ThreadSummary, error) {
	return s.apply(ctx, refs, func(ctx context.Context, accountID, threadID string) error {
		return s.d.Gmail.ModifyThread(ctx, accountID, threadID, []string{"INBOX"}, nil)
	})
}

// Untrash は Trash の取り消し。スレッドをゴミ箱から戻す。backToInbox が true なら INBOX ラベルも付け直す
// （ゴミ箱へ移す前に受信箱にあったスレッド用。Trash は INBOX を除去し、Gmail の untrash が INBOX を戻すとは限らない）。
func (s *InboxService) Untrash(ctx context.Context, refs []dto.ThreadRef, backToInbox bool) ([]dto.ThreadSummary, error) {
	return s.apply(ctx, refs, func(ctx context.Context, accountID, threadID string) error {
		if err := s.d.Gmail.UntrashThread(ctx, accountID, threadID); err != nil {
			return err
		}
		if backToInbox {
			return s.d.Gmail.ModifyThread(ctx, accountID, threadID, []string{"INBOX"}, nil)
		}
		return nil
	})
}

// apply は refs をアカウントごとにまとめて op を実行し、Sync.RefreshThread で DB を更新して更新後サマリを返す。
// Gmail で 404 のスレッドは DB から削除し、NOT_FOUND として failedRefs に含める。
// 1 件でも失敗したら、失敗した ref を details.failedRefs に載せた AppError で reject する（成功分は反映済み）。
func (s *InboxService) apply(ctx context.Context, refs []dto.ThreadRef, op threadOp) ([]dto.ThreadSummary, error) {
	out := make([]dto.ThreadSummary, 0, len(refs))
	if len(refs) == 0 {
		return out, nil
	}
	var accountOrder []string
	byAccount := map[string][]int{}
	for i, r := range refs {
		if r.AccountID == "" || r.ThreadID == "" {
			return nil, validation("スレッドの指定が正しくありません。")
		}
		if _, ok := byAccount[r.AccountID]; !ok {
			accountOrder = append(accountOrder, r.AccountID)
		}
		byAccount[r.AccountID] = append(byAccount[r.AccountID], i)
	}
	_, known, err := accountsByID(s.d, ctx)
	if err != nil {
		return nil, err
	}

	outcomes := make([]opOutcome, len(refs))
	var mu sync.Mutex
	touched := map[string][]string{}
	appcore.ParallelDo(len(accountOrder), len(accountOrder), func(n int) {
		accountID := accountOrder[n]
		for _, i := range byAccount[accountID] {
			o := opOutcome{ref: refs[i]}
			if _, ok := known[accountID]; !ok {
				o.err = apperr.New(apperr.NotFound, "アカウントが見つかりません。")
			} else {
				o.sum, o.err = s.applyOne(ctx, accountID, refs[i].ThreadID, op)
				mu.Lock()
				touched[accountID] = append(touched[accountID], refs[i].ThreadID)
				mu.Unlock()
			}
			outcomes[i] = o
		}
	})
	for accountID, ids := range touched {
		s.d.Threads.Invalidate(accountID, ids...)
	}

	var failed []map[string]string
	var first *opOutcome // 代表エラー: NOT_FOUND 以外を優先する（NOT_FOUND は DB から除去済みで再試行不要）
	for i, o := range outcomes {
		switch {
		case o.err != nil:
			if first == nil || (isNotFound(first.err) && !isNotFound(o.err)) {
				first = &outcomes[i]
			}
			failed = append(failed, map[string]string{"accountId": o.ref.AccountID, "threadId": o.ref.ThreadID})
		case o.sum != nil:
			out = append(out, *o.sum)
		}
	}
	if first != nil {
		return nil, gmailErr(s.d, ctx, first.ref.AccountID, first.err).WithDetail("failedRefs", failed)
	}
	return out, nil
}

func isNotFound(err error) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Code == apperr.NotFound
}

// applyOne は 1 スレッドに op を実行して DB を更新する。スレッドが Gmail に無ければ（404）DB から削除したうえで
// NOT_FOUND を返す（F-04: 操作時の 404 は「スレッドが見つかりません」としてリストから除去する）。
func (s *InboxService) applyOne(ctx context.Context, accountID, threadID string, op threadOp) (*dto.ThreadSummary, error) {
	if err := op(ctx, accountID, threadID); err != nil {
		if errors.Is(err, gmail.ErrNotFound) {
			return nil, s.dropGone(ctx, accountID, threadID)
		}
		return nil, err
	}
	row, found, err := s.d.Sync.RefreshThread(ctx, accountID, threadID)
	if err != nil {
		return nil, err
	}
	if !found { // 同期エンジンが DB からも削除済み
		return nil, threadGone(accountID)
	}
	sum := summaryFromRow(row)
	return &sum, nil
}

// dropGone は Gmail に無くなったスレッドを DB から削除し、NOT_FOUND を返す（削除に失敗したらそのエラーを返す）。
func (s *InboxService) dropGone(ctx context.Context, accountID, threadID string) error {
	if err := s.d.Store.DeleteThread(ctx, accountID, threadID); err != nil {
		return apperr.FromInfra(err, accountID)
	}
	return threadGone(accountID)
}
