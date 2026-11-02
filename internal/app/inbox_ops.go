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

// apply は refs をアカウントごとにまとめて op を実行し、Sync.RefreshThread で DB を更新して更新後サマリを返す。
// Gmail で 404（すでに無い）のスレッドは DB から削除し、結果には含めない。
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
	var first error
	for _, o := range outcomes {
		switch {
		case o.err != nil:
			if first == nil {
				first = o.err
			}
			failed = append(failed, map[string]string{"accountId": o.ref.AccountID, "threadId": o.ref.ThreadID})
		case o.sum != nil:
			out = append(out, *o.sum)
		}
	}
	if first != nil {
		return nil, gmailErr(s.d, ctx, failedAccount(refs, outcomes), first).WithDetail("failedRefs", failed)
	}
	return out, nil
}

// failedAccount は最初に失敗した ref のアカウント ID。
func failedAccount(refs []dto.ThreadRef, outcomes []opOutcome) string {
	for i, o := range outcomes {
		if o.err != nil {
			return refs[i].AccountID
		}
	}
	return ""
}

// applyOne は 1 スレッドに op を実行して DB を更新する。スレッドが Gmail に無ければ DB から削除して nil を返す。
func (s *InboxService) applyOne(ctx context.Context, accountID, threadID string, op threadOp) (*dto.ThreadSummary, error) {
	if err := op(ctx, accountID, threadID); err != nil {
		if !errors.Is(err, gmail.ErrNotFound) {
			return nil, err
		}
		return nil, s.d.Store.DeleteThread(ctx, accountID, threadID)
	}
	row, found, err := s.d.Sync.RefreshThread(ctx, accountID, threadID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	sum := summaryFromRow(row)
	return &sum, nil
}
