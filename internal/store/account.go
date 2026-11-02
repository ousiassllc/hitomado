package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ousiassllc/hitomado/internal/store/db"
)

func accountFromRow(r db.Account) Account {
	return Account{
		ID:            r.ID,
		OAuthClientID: r.OauthClientID,
		Email:         r.Email,
		DisplayName:   r.DisplayName,
		Color:         r.Color,
		SignatureHTML: r.SignatureHtml,
		NotifyEnabled: r.NotifyEnabled == 1,
		Status:        r.Status,
		StatusMessage: r.StatusMessage.String,
		SortOrder:     int(r.SortOrder),
		CreatedAt:     fromMillis(r.CreatedAt),
		UpdatedAt:     fromMillis(r.UpdatedAt),
	}
}

func (s *sqlStore) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.q.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list accounts: %w", err)
	}
	out := make([]Account, 0, len(rows))
	for _, r := range rows {
		out = append(out, accountFromRow(r))
	}
	return out, nil
}

func (s *sqlStore) GetAccount(ctx context.Context, id string) (Account, error) {
	return getAccount(ctx, s.q, id)
}

func getAccount(ctx context.Context, q *db.Queries, id string) (Account, error) {
	r, err := q.GetAccount(ctx, id)
	if err != nil {
		return Account{}, fmt.Errorf("store: get account: %w", notFoundIfNoRows(err))
	}
	return accountFromRow(r), nil
}

// CreateAccount はアカウントと account_sync_state を 1 トランザクションで作成する。
// email は trim＋小文字化して保存する。sort_order は a.SortOrder によらず末尾（既存の最大値＋1）に採番する。
func (s *sqlStore) CreateAccount(ctx context.Context, a Account) error {
	email := strings.ToLower(strings.TrimSpace(a.Email))
	if email == "" {
		return errors.New("store: create account: email is empty")
	}
	status := a.Status
	if status == "" {
		status = StatusActive
	}
	created := nowOr(a.CreatedAt, s.now())
	updated := nowOr(a.UpdatedAt, created)
	return s.withTx(ctx, func(q *db.Queries) error {
		maxOrder, err := q.MaxAccountSortOrder(ctx)
		if err != nil {
			return fmt.Errorf("store: create account: %w", err)
		}
		err = q.CreateAccount(ctx, db.CreateAccountParams{
			ID:            a.ID,
			OauthClientID: a.OAuthClientID,
			Email:         email,
			DisplayName:   a.DisplayName,
			Color:         a.Color,
			SignatureHtml: a.SignatureHTML,
			NotifyEnabled: boolInt(a.NotifyEnabled),
			Status:        status,
			StatusMessage: nullString(a.StatusMessage),
			SortOrder:     maxOrder + 1,
			CreatedAt:     toMillis(created),
			UpdatedAt:     toMillis(updated),
		})
		switch {
		case isUnique(err):
			return fmt.Errorf("store: create account: %w", ErrDuplicate)
		case isForeignKey(err):
			return fmt.Errorf("store: create account: oauth client: %w", ErrNotFound)
		case err != nil:
			return fmt.Errorf("store: create account: %w", err)
		}
		if err := q.CreateSyncState(ctx, a.ID); err != nil {
			return fmt.Errorf("store: create account sync state: %w", err)
		}
		return nil
	})
}

func (s *sqlStore) UpdateAccount(ctx context.Context, id string, p AccountPatch) (Account, error) {
	var out Account
	err := s.withTx(ctx, func(q *db.Queries) error {
		cur, err := getAccount(ctx, q, id)
		if err != nil {
			return err
		}
		if p.DisplayName != nil {
			cur.DisplayName = *p.DisplayName
		}
		if p.Color != nil {
			cur.Color = *p.Color
		}
		if p.NotifyEnabled != nil {
			cur.NotifyEnabled = *p.NotifyEnabled
		}
		err = requireRows(q.UpdateAccount(ctx, db.UpdateAccountParams{
			ID:            id,
			DisplayName:   cur.DisplayName,
			Color:         cur.Color,
			NotifyEnabled: boolInt(cur.NotifyEnabled),
			UpdatedAt:     toMillis(s.now()),
		}))
		if err != nil {
			return fmt.Errorf("store: update account: %w", err)
		}
		out, err = getAccount(ctx, q, id)
		return err
	})
	return out, err
}

func (s *sqlStore) SetAccountStatus(ctx context.Context, id, status, message string) error {
	err := requireRows(s.q.SetAccountStatus(ctx, db.SetAccountStatusParams{
		ID:            id,
		Status:        status,
		StatusMessage: nullString(message),
		UpdatedAt:     toMillis(s.now()),
	}))
	if err != nil {
		return fmt.Errorf("store: set account status: %w", err)
	}
	return nil
}

func (s *sqlStore) SetSignature(ctx context.Context, id, signatureHTML string) (Account, error) {
	var out Account
	err := s.withTx(ctx, func(q *db.Queries) error {
		err := requireRows(q.SetAccountSignature(ctx, db.SetAccountSignatureParams{
			ID: id, SignatureHtml: signatureHTML, UpdatedAt: toMillis(s.now()),
		}))
		if err != nil {
			return fmt.Errorf("store: set signature: %w", err)
		}
		out, err = getAccount(ctx, q, id)
		return err
	})
	return out, err
}

// ReorderAccounts は ids の並びを sort_order に反映する。ids は全アカウント ID を過不足なく 1 回ずつ含むこと。
func (s *sqlStore) ReorderAccounts(ctx context.Context, ids []string) error {
	return s.withTx(ctx, func(q *db.Queries) error {
		existing, err := q.ListAccountIDs(ctx)
		if err != nil {
			return fmt.Errorf("store: reorder accounts: %w", err)
		}
		want := make(map[string]struct{}, len(existing))
		for _, id := range existing {
			want[id] = struct{}{}
		}
		if len(ids) != len(existing) {
			return fmt.Errorf("store: reorder accounts: ids must contain all %d accounts exactly once", len(existing))
		}
		for _, id := range ids {
			if _, ok := want[id]; !ok {
				return fmt.Errorf("store: reorder accounts: unknown or duplicate account id")
			}
			delete(want, id)
		}
		for i, id := range ids {
			if err := requireRows(q.SetAccountSortOrder(ctx, db.SetAccountSortOrderParams{ID: id, SortOrder: int64(i)})); err != nil {
				return fmt.Errorf("store: reorder accounts: %w", err)
			}
		}
		return nil
	})
}

func (s *sqlStore) DeleteAccount(ctx context.Context, id string) error {
	if err := requireRows(s.q.DeleteAccount(ctx, id)); err != nil {
		return fmt.Errorf("store: delete account: %w", err)
	}
	return nil
}
