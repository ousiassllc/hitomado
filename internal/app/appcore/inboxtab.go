package appcore

import (
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/store"
)

// InboxFilterOf は InboxQuery のタブ・未読のみ・カーソルから store.InboxFilter を作る（AccountIDs・Limit は呼び出し側で設定）。
// タブ・カーソルが不正なら VALIDATION。タブ・未読のみの絞り込みはローカルのキャッシュだけに掛かり、Gmail からの追加取得は
// 絞り込まない INBOX の続きを進める。カーソルは絞り込みに依存しない位置なので、タブを切り替えたら呼び出し側が捨てて先頭から取り直す。
func InboxFilterOf(q dto.InboxQuery) (store.InboxFilter, error) {
	tab, err := ParseInboxTab(q.Tab)
	if err != nil {
		return store.InboxFilter{}, err
	}
	after, err := DecodeInboxCursor(q.Cursor)
	if err != nil {
		return store.InboxFilter{}, err
	}
	return store.InboxFilter{UnreadOnly: q.UnreadOnly, Tab: tab, After: after}, nil
}

// ParseInboxTab は InboxQuery.Tab を store のタブに直す。"all" は "" と同じ。未知の値は VALIDATION。
func ParseInboxTab(tab string) (store.InboxTab, error) {
	if tab == "all" {
		return store.InboxTabAll, nil
	}
	t := store.InboxTab(tab)
	if !t.Valid() {
		return "", apperr.New(apperr.Validation, "tab は all／primary／social／promotions／new のいずれかで指定してください。")
	}
	return t, nil
}
