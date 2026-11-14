package platform

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/capability"
)

// マスコットに対応しない OS（Linux のデスクトップ・Android）では、出す指示でもウィンドウに触らない
// （app が nil のままでも落ちず、panic の記録も出ない）。
func TestMascotSetVisibleDoesNothingWhenUnsupported(t *testing.T) {
	var buf bytes.Buffer
	m := NewMascot(nil, capability.Desktop("linux"), nil, slog.New(slog.NewTextHandler(&buf, nil)))

	m.SetVisible(true)
	m.SetVisible(false)

	assert.Nil(t, m.win)
	assert.Empty(t, buf.String())
}
