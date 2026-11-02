package store

import (
	"context"
	"fmt"

	"github.com/ousiassllc/hitomado/internal/store/db"
)

// ReplaceLabels はアカウントのラベル一覧を labels で置き換える。
func (s *sqlStore) ReplaceLabels(ctx context.Context, accountID string, labels []Label) error {
	return s.withTx(ctx, func(q *db.Queries) error {
		if err := q.DeleteLabels(ctx, accountID); err != nil {
			return fmt.Errorf("store: replace labels: %w", err)
		}
		for _, l := range labels {
			err := q.InsertLabel(ctx, db.InsertLabelParams{
				AccountID: accountID,
				ID:        l.ID,
				Name:      l.Name,
				Type:      l.Type,
				ColorBg:   nullString(l.ColorBg),
				ColorFg:   nullString(l.ColorFg),
			})
			if isForeignKey(err) {
				return fmt.Errorf("store: replace labels: account: %w", ErrNotFound)
			}
			if err != nil {
				return fmt.Errorf("store: replace labels: %w", err)
			}
		}
		return nil
	})
}

func (s *sqlStore) ListLabels(ctx context.Context, accountID string) ([]Label, error) {
	rows, err := s.q.ListLabels(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: list labels: %w", err)
	}
	out := make([]Label, 0, len(rows))
	for _, r := range rows {
		out = append(out, Label{ID: r.ID, Name: r.Name, Type: r.Type, ColorBg: r.ColorBg.String, ColorFg: r.ColorFg.String})
	}
	return out, nil
}
