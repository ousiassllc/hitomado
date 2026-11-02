package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ousiassllc/hitomado/internal/store/db"
)

func (s *sqlStore) GetSyncState(ctx context.Context, accountID string) (SyncState, error) {
	r, err := s.q.GetSyncState(ctx, accountID)
	if err != nil {
		return SyncState{}, fmt.Errorf("store: get sync state: %w", notFoundIfNoRows(err))
	}
	st := SyncState{
		AccountID:       r.AccountID,
		HistoryID:       r.HistoryID.String,
		InboxPageToken:  r.InboxPageToken.String,
		InboxExhausted:  r.InboxExhausted == 1,
		InitialSyncDone: r.InitialSyncDone == 1,
		LastError:       r.LastError.String,
	}
	if r.LastSyncedAt.Valid {
		st.LastSyncedAt = fromMillis(r.LastSyncedAt.Int64)
	}
	return st, nil
}

func (s *sqlStore) SaveSyncState(ctx context.Context, st SyncState) error {
	var syncedAt sql.NullInt64
	if !st.LastSyncedAt.IsZero() {
		syncedAt = sql.NullInt64{Int64: toMillis(st.LastSyncedAt), Valid: true}
	}
	err := requireRows(s.q.SaveSyncState(ctx, db.SaveSyncStateParams{
		AccountID:       st.AccountID,
		HistoryID:       nullString(st.HistoryID),
		InboxPageToken:  nullString(st.InboxPageToken),
		InboxExhausted:  boolInt(st.InboxExhausted),
		InitialSyncDone: boolInt(st.InitialSyncDone),
		LastSyncedAt:    syncedAt,
		LastError:       nullString(st.LastError),
	}))
	if err != nil {
		return fmt.Errorf("store: save sync state: %w", err)
	}
	return nil
}
