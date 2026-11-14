package apptests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
)

// 利用者の操作の失敗で再接続待ちになるときも、理由と契機をログに残す（アカウントは ID だけ）。invalid_grant は同期周期の経路と同じ
// 状態メッセージにして、画面が原因の案内（OAuth 同意画面が「テスト」だと 7 日で失効する、など）を出せるようにする（#369）。
func TestGetThreadNeedsReauthLogsReason(t *testing.T) {
	for _, tc := range []struct {
		name, reason, wantMsg string
	}{
		{"invalid_grant", gmail.ReasonInvalidGrant, syncer.MsgTokenRevoked},
		{"api 401", gmail.ReasonAPIUnauthorized, "アカウントの接続が切れました。再接続してください。"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, e := newThreadSvc(t)
			a := e.AddAccount(t, "me@a.example")
			var buf bytes.Buffer
			e.Deps.Logger = logging.NewWriter(&buf, slog.LevelInfo, logging.Meta{Version: "test"})
			e.Gmail.Err["GetThreadFull"] = gmail.WithReason(fmt.Errorf("x: %w", gmail.ErrNeedsReauth), tc.reason)

			_, err := svc.GetThread(t.Context(), ref(a.ID, "t1"))
			_ = requireCode(t, apperr.NeedsReauth, err)

			got, _ := e.Store.GetAccount(t.Context(), a.ID)
			assert.Equal(t, store.StatusNeedsReauth, got.Status)
			assert.Equal(t, tc.wantMsg, got.StatusMessage)
			var rec map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes()[:bytes.IndexByte(buf.Bytes(), '\n')], &rec))
			assert.Equal(t, "account needs reauth", rec["msg"])
			assert.Equal(t, a.ID, rec["account"])
			assert.Equal(t, tc.reason, rec["reason"])
			assert.Equal(t, "action", rec["trigger"])
			assert.Equal(t, "app", rec["origin"])
			assert.NotContains(t, buf.String(), "me@a.example")
		})
	}
}
