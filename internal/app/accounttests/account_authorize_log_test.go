package accounttests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/logging"
)

// 認可が分類できない理由で失敗して INTERNAL になるとき、画面には出ない元のエラーと型のつながりをログに残す（#184）。
func TestStartAddAccountLogsTheCauseOfInternal(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "x@a.example")
	buf := &bytes.Buffer{}
	e.Deps.Logger = logging.NewWriter(buf, slog.LevelInfo, logging.Meta{Version: "test"})
	e.Flow.Err = fmt.Errorf("oauth: ブラウザを開けませんでした: %w", errors.New("intent failed"))

	_, err := svc.StartAddAccount(t.Context(), a.OAuthClientID)
	ae := requireCode(t, apperr.Internal, err)

	assert.NotContains(t, ae.Message, "intent failed")
	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes()[:bytes.IndexByte(buf.Bytes(), '\n')], &rec))
	assert.Equal(t, "WARN", rec["level"])
	assert.Equal(t, "account authorize failed", rec["msg"])
	assert.Equal(t, "INTERNAL", rec["code"])
	assert.Equal(t, "oauth: ブラウザを開けませんでした: intent failed", rec["err"])
	assert.Equal(t, "*fmt.wrapError > *errors.errorString", rec["chain"])
}
