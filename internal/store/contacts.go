package store

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/store/db"
)

// defaultSuggestLimit は SuggestContacts の limit が 0 以下のときの件数。
const defaultSuggestLimit = 10

func (s *sqlStore) AddReceivedContacts(ctx context.Context, accountID string, addrs []model.Address, at time.Time) error {
	return s.addContacts(ctx, accountID, addrs, at, func(q *db.Queries, a model.Address, ms int64) error {
		return q.UpsertReceivedContact(ctx, db.UpsertReceivedContactParams{
			AccountID: accountID, Email: a.Email, DisplayName: nullString(a.Name), LastUsedAt: ms,
		})
	})
}

func (s *sqlStore) AddSentContacts(ctx context.Context, accountID string, addrs []model.Address, at time.Time) error {
	return s.addContacts(ctx, accountID, addrs, at, func(q *db.Queries, a model.Address, ms int64) error {
		return q.UpsertSentContact(ctx, db.UpsertSentContactParams{
			AccountID: accountID, Email: a.Email, DisplayName: nullString(a.Name), LastUsedAt: ms,
		})
	})
}

// addContacts は addrs を正規化（trim＋小文字）・重複除去（1 回の呼び出しにつき 1 アドレス 1 回の観測）して
// 1 トランザクションで加算する。空のアドレスは無視する。
func (s *sqlStore) addContacts(ctx context.Context, accountID string, addrs []model.Address, at time.Time,
	upsert func(q *db.Queries, a model.Address, ms int64) error) error {
	seen := make(map[string]struct{}, len(addrs))
	list := make([]model.Address, 0, len(addrs))
	for _, a := range addrs {
		a.Email = strings.ToLower(strings.TrimSpace(a.Email))
		a.Name = strings.TrimSpace(a.Name)
		if _, dup := seen[a.Email]; a.Email == "" || dup {
			continue
		}
		seen[a.Email] = struct{}{}
		list = append(list, a)
	}
	if len(list) == 0 {
		return nil
	}
	return s.withTx(ctx, func(q *db.Queries) error {
		for _, a := range list {
			err := upsert(q, a, toMillis(at))
			if isForeignKey(err) {
				return fmt.Errorf("store: add contacts: account: %w", ErrNotFound)
			}
			if err != nil {
				return fmt.Errorf("store: add contacts: %w", err)
			}
		}
		return nil
	})
}

// SuggestContacts は prefix に前方一致するメールアドレス（PK インデックスの範囲検索）または表示名の
// 候補を、順位（send*3+receive 降順、同点は last_used_at 降順）で返す。自アカウントのアドレスは除く。
// prefix が空のときは順位の上位を返す。
func (s *sqlStore) SuggestContacts(ctx context.Context, accountID, prefix string, limit int) ([]Contact, error) {
	if limit <= 0 {
		limit = defaultSuggestLimit
	}
	q := strings.ToLower(strings.TrimSpace(prefix))
	from, to := emailRange(q)
	rows, err := s.q.SuggestContacts(ctx, db.SuggestContactsParams{
		AccountID:   accountID,
		EmailFrom:   from,
		EmailTo:     to,
		NamePattern: likePrefixPattern(q),
		RowLimit:    int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("store: suggest contacts: %w", err)
	}
	out := make([]Contact, 0, len(rows))
	for _, r := range rows {
		out = append(out, Contact{Email: r.Email, DisplayName: r.DisplayName.String})
	}
	return out, nil
}

// emailRange は前方一致 q の範囲検索 [from, to) を返す（to は q の最終文字を 1 つ進めたもの）。
// q が空のとき（および最終文字が進められないとき）は上限なし相当の番兵を返す。
func emailRange(q string) (from, to string) {
	const unbounded = "\U0010FFFF"
	if q == "" {
		return "", unbounded
	}
	runes := []rune(q)
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] < utf8.MaxRune {
			runes[i]++
			return q, string(runes[:i+1])
		}
	}
	return q, unbounded
}

// likePrefixPattern は LIKE の前方一致パターン（\ をエスケープ文字とする）を返す。
func likePrefixPattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(q) + "%"
}
