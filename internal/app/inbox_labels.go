package app

import (
	"context"
	"errors"
	"slices"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// refreshLabels は labels.list の結果でラベルのキャッシュを更新し、保存した全ラベルを返す。
func (s *InboxService) refreshLabels(ctx context.Context, accountID string) ([]store.Label, error) {
	remote, err := s.d.Gmail.ListLabels(ctx, accountID)
	if err != nil {
		return nil, err
	}
	labels := make([]store.Label, len(remote))
	for i, l := range remote {
		labels[i] = store.Label{ID: l.ID, Name: l.Name, Type: l.Type, ColorBg: l.ColorBg, ColorFg: l.ColorFg}
	}
	if err := s.d.Store.ReplaceLabels(ctx, accountID, labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// ListLabels はユーザー作成ラベルを返す（labels.list で更新。オフライン時はキャッシュ）。
func (s *InboxService) ListLabels(ctx context.Context, accountID string) ([]dto.Label, error) {
	if _, err := s.d.Store.GetAccount(ctx, accountID); err != nil {
		return nil, apperr.FromInfra(err, accountID)
	}
	labels, err := s.refreshLabels(ctx, accountID)
	if err != nil {
		ae := gmailErr(s.d, ctx, accountID, err)
		if ae.Code != apperr.Offline {
			return nil, ae
		}
		if labels, err = s.d.Store.ListLabels(ctx, accountID); err != nil {
			return nil, apperr.FromInfra(err, accountID)
		}
	}
	return userLabelsDTO(labels), nil
}

// ModifyLabels はユーザー作成ラベルの付与・除去。システムラベル・未知のラベルは VALIDATION。
func (s *InboxService) ModifyLabels(ctx context.Context, ref dto.ThreadRef, addLabelIDs, removeLabelIDs []string) (dto.ThreadSummary, error) {
	if len(addLabelIDs)+len(removeLabelIDs) == 0 {
		return dto.ThreadSummary{}, validation("ラベルを指定してください。")
	}
	if _, err := s.d.Store.GetAccount(ctx, ref.AccountID); err != nil {
		return dto.ThreadSummary{}, apperr.FromInfra(err, ref.AccountID)
	}
	wanted := slices.Concat(addLabelIDs, removeLabelIDs)
	labels, err := s.d.Store.ListLabels(ctx, ref.AccountID)
	if err != nil {
		return dto.ThreadSummary{}, apperr.FromInfra(err, ref.AccountID)
	}
	if !allUserLabels(labels, wanted) {
		// キャッシュに無い新しいラベルかもしれないので 1 回だけ更新して再判定する。
		if labels, err = s.refreshLabels(ctx, ref.AccountID); err != nil {
			return dto.ThreadSummary{}, gmailErr(s.d, ctx, ref.AccountID, err)
		}
		if !allUserLabels(labels, wanted) {
			return dto.ThreadSummary{}, validation("指定できるのはユーザー作成ラベルのみです。")
		}
	}
	if err := s.d.Gmail.ModifyThread(ctx, ref.AccountID, ref.ThreadID, addLabelIDs, removeLabelIDs); err != nil {
		if errors.Is(err, gmail.ErrNotFound) {
			return dto.ThreadSummary{}, s.dropGone(ctx, ref.AccountID, ref.ThreadID)
		}
		return dto.ThreadSummary{}, gmailErr(s.d, ctx, ref.AccountID, err)
	}
	s.d.Threads.Invalidate(ref.AccountID, ref.ThreadID)
	row, found, err := s.d.Sync.RefreshThread(ctx, ref.AccountID, ref.ThreadID)
	if err != nil {
		return dto.ThreadSummary{}, gmailErr(s.d, ctx, ref.AccountID, err)
	}
	if !found {
		return dto.ThreadSummary{}, threadGone(ref.AccountID)
	}
	return summaryFromRow(row), nil
}

func allUserLabels(labels []store.Label, ids []string) bool {
	return !slices.ContainsFunc(ids, func(id string) bool {
		return !slices.ContainsFunc(labels, func(l store.Label) bool { return l.ID == id && l.Type == "user" })
	})
}
