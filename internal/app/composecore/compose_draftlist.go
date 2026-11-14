package composecore

import (
	"context"
	"errors"
	"sort"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// draftListLimit は ListDrafts が 1 アカウントから読む下書きの上限（先頭から。Gmail は更新の新しい順に返す）。
const draftListLimit = 100

// ListDrafts は accountIDs（空は全アカウント）の Gmail の下書きを、更新の新しい順の 1 つの一覧にして返す（F-12。下書きの一覧）。
// アカウントごとに並列で Gmail を呼び、失敗したアカウントは Failures に入れて、ほかの結果は返す。
// 下書きの内容（宛先・件名・本文）はこの一覧のためだけに読み、保存もログへの出力もしない。
func (s *Service) ListDrafts(ctx context.Context, accountIDs []string) (dto.DraftList, error) {
	accounts, err := s.d.Store.ListAccounts(ctx)
	if err != nil {
		return dto.DraftList{}, apperr.FromInfra(err, "")
	}
	accounts = pickAccounts(accounts, accountIDs)
	lists := make([]gmail.DraftList, len(accounts))
	errs := make([]error, len(accounts))
	appcore.ParallelDo(s.log(), len(accounts), appcore.SearchParallelism, func(i int) {
		lists[i], errs[i] = s.d.Gmail.ListDrafts(ctx, accounts[i].ID, draftListLimit)
	})

	out := dto.DraftList{Items: []dto.DraftSummary{}, Failures: []dto.AccountFailure{}}
	for i, a := range accounts {
		if errs[i] != nil {
			out.Failures = append(out.Failures, dto.AccountFailure{AccountID: a.ID, Error: apperr.ToMap(apperr.FromInfra(errs[i], a.ID))})
			continue
		}
		out.Truncated = out.Truncated || lists[i].Truncated
		for _, d := range lists[i].Items {
			out.Items = append(out.Items, draftSummaryOf(a.ID, d))
		}
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.UpdatedAt != b.UpdatedAt {
			return a.UpdatedAt > b.UpdatedAt
		}
		if a.AccountID != b.AccountID {
			return a.AccountID < b.AccountID
		}
		return a.DraftID < b.DraftID
	})
	return out, nil
}

// pickAccounts は ids（空は全アカウント）に該当するアカウントを、一覧の順で返す。未知の ID は無視する。
func pickAccounts(all []store.Account, ids []string) []store.Account {
	if len(ids) == 0 {
		return all
	}
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := make([]store.Account, 0, len(ids))
	for _, a := range all {
		if _, ok := want[a.ID]; ok {
			out = append(out, a)
		}
	}
	return out
}

func draftSummaryOf(accountID string, d gmail.DraftSummary) dto.DraftSummary {
	to := make([]model.Address, 0, len(d.To))
	for _, a := range d.To {
		to = append(to, model.Address{Name: foldLine(a.Name), Email: foldLine(a.Email)})
	}
	return dto.DraftSummary{
		AccountID: accountID, DraftID: d.ID, ThreadID: d.ThreadID, UpdatedAt: d.UpdatedAt.UnixMilli(),
		To: to, Subject: foldLine(d.Subject), Snippet: d.Snippet, IsReply: d.IsReply,
	}
}

// DiscardDraft は一覧から選んだ Gmail の下書きを削除する（作成セッションを開かずに破棄する）。
// 既に無い（Gmail 側で削除済み）ときは成功として扱う。削除したらスレッド表示のキャッシュも捨てる。
func (s *Service) DiscardDraft(ctx context.Context, ref dto.DraftRef) error {
	defer s.d.Busy.Begin()() // 下書きの削除中は自動アップデートの入れ替えをしない
	if ref.AccountID == "" || ref.DraftID == "" {
		return apperr.New(apperr.Validation, "破棄する下書きを指定してください。")
	}
	if _, err := s.account(ctx, ref.AccountID); err != nil {
		return err
	}
	if err := s.d.Gmail.DeleteDraft(ctx, ref.AccountID, ref.DraftID); err != nil && !errors.Is(err, gmail.ErrNotFound) {
		return apperr.FromInfra(err, ref.AccountID)
	}
	s.invalidateThread(ref.AccountID, ref.ThreadID)
	return nil
}
