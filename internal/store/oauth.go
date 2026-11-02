package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ousiassllc/hitomado/internal/store/db"
)

func (s *sqlStore) ListOAuthClients(ctx context.Context) ([]OAuthClient, error) {
	rows, err := s.q.ListOAuthClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list oauth clients: %w", err)
	}
	out := make([]OAuthClient, 0, len(rows))
	for _, r := range rows {
		out = append(out, OAuthClient{
			ID:           r.ID,
			Name:         r.Name,
			ClientID:     r.ClientID,
			AccountCount: int(r.AccountCount),
			CreatedAt:    fromMillis(r.CreatedAt),
			UpdatedAt:    fromMillis(r.UpdatedAt),
		})
	}
	return out, nil
}

func (s *sqlStore) GetOAuthClient(ctx context.Context, id string) (OAuthClient, error) {
	return getOAuthClient(ctx, s.q, id)
}

func getOAuthClient(ctx context.Context, q *db.Queries, id string) (OAuthClient, error) {
	r, err := q.GetOAuthClient(ctx, id)
	if err != nil {
		return OAuthClient{}, fmt.Errorf("store: get oauth client: %w", notFoundIfNoRows(err))
	}
	return OAuthClient{
		ID:        r.ID,
		Name:      r.Name,
		ClientID:  r.ClientID,
		CreatedAt: fromMillis(r.CreatedAt),
		UpdatedAt: fromMillis(r.UpdatedAt),
	}, nil
}

func (s *sqlStore) CreateOAuthClient(ctx context.Context, c OAuthClient) error {
	created := nowOr(c.CreatedAt, s.now())
	updated := nowOr(c.UpdatedAt, created)
	err := s.q.CreateOAuthClient(ctx, db.CreateOAuthClientParams{
		ID:        c.ID,
		Name:      c.Name,
		ClientID:  c.ClientID,
		CreatedAt: toMillis(created),
		UpdatedAt: toMillis(updated),
	})
	if isUnique(err) {
		return fmt.Errorf("store: create oauth client: %w", ErrDuplicate)
	}
	if err != nil {
		return fmt.Errorf("store: create oauth client: %w", err)
	}
	return nil
}

func (s *sqlStore) UpdateOAuthClient(ctx context.Context, id, name, clientID string) (OAuthClient, error) {
	var out OAuthClient
	err := s.withTx(ctx, func(q *db.Queries) error {
		n, err := q.UpdateOAuthClient(ctx, db.UpdateOAuthClientParams{
			ID: id, Name: name, ClientID: clientID, UpdatedAt: toMillis(s.now()),
		})
		if isUnique(err) {
			return fmt.Errorf("store: update oauth client: %w", ErrDuplicate)
		}
		if err := requireRows(n, err); err != nil {
			return fmt.Errorf("store: update oauth client: %w", err)
		}
		out, err = getOAuthClient(ctx, q, id)
		return err
	})
	return out, err
}

func (s *sqlStore) DeleteOAuthClient(ctx context.Context, id string) error {
	n, err := s.q.DeleteOAuthClient(ctx, id)
	if isForeignKey(err) {
		return fmt.Errorf("store: delete oauth client: %w", ErrOAuthClientUse)
	}
	if err := requireRows(n, err); err != nil {
		return fmt.Errorf("store: delete oauth client: %w", err)
	}
	return nil
}

// nowOr は t がゼロ値のとき now を返す。
func nowOr(t, now time.Time) time.Time {
	if t.IsZero() {
		return now
	}
	return t
}
