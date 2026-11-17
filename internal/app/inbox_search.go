package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/app/searchcore"
)

// Search は Gmail 検索クエリを各アカウントで並列実行し、最終メッセージ日時の降順で統合する（結果は保存しない）。
// 入力の検証と対象アカウントの解決だけここで行い、検索の本体は searchcore.Search。
func (s *InboxService) Search(ctx context.Context, q dto.SearchQuery) (dto.ThreadPage, error) {
	query := trimmed(q.Q)
	if query == "" {
		return dto.ThreadPage{}, validation("検索クエリを入力してください。")
	}
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return dto.ThreadPage{}, err
	}
	states, err := appcore.DecodeSearchCursor(q.Cursor)
	if err != nil {
		return dto.ThreadPage{}, err
	}
	list, _, err := accountsByID(s.d, ctx)
	if err != nil {
		return dto.ThreadPage{}, err
	}
	return searchcore.Search(ctx, searchcore.Params{
		Deps: (*deps.Deps)(s.d), Accounts: targetAccounts(list, q.AccountIDs), States: states,
		Query: query, Limit: limit, MaxFetchRounds: maxFetchRounds, Meta: s.meta,
	}), nil
}
