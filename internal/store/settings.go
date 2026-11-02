package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ousiassllc/hitomado/internal/store/db"
)

func (s *sqlStore) GetSetting(ctx context.Context, key string) (string, bool, error) {
	v, err := s.q.GetSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: get setting: %w", err)
	}
	return v, true, nil
}

func (s *sqlStore) SetSetting(ctx context.Context, key, value string) error {
	err := s.q.SetSetting(ctx, db.SetSettingParams{Key: key, Value: value, UpdatedAt: toMillis(s.now())})
	if err != nil {
		return fmt.Errorf("store: set setting: %w", err)
	}
	return nil
}
