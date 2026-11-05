package flowtest_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/composecore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

// draftBlockingGmail は CreateDraft を呼び出し側が解放するまで止める gmail.Client。
type draftBlockingGmail struct {
	gmail.Client
	entered chan struct{}
	release chan struct{}
}

func (b *draftBlockingGmail) CreateDraft(ctx context.Context, accountID string, raw []byte, threadID string) (string, error) {
	close(b.entered)
	<-b.release
	return b.Client.CreateDraft(ctx, accountID, raw, threadID)
}

// 送信中・下書きの保存中は、自動アップデートの入れ替えをしない（Deps.Busy が進行中を示す）。
func TestSendAndSaveDraftMarkBusyForAutoUpdate(t *testing.T) {
	t.Run("送信", func(t *testing.T) {
		f := newComposeFixture(t)
		bg := &blockingGmail{Client: f.Gmail, entered: make(chan struct{}), release: make(chan struct{})}
		f.Deps.Gmail = bg
		f.Deps.Busy = &deps.Busy{}
		svc := composecore.New((*deps.Deps)(f.Deps))
		s, err := svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: f.A.ID})
		require.NoError(t, err)
		c := toContent("s", "<p>b</p>", "dave@x.example")
		p, err := svc.PrepareSend(t.Context(), s.SessionID, c, dto.SendOptions{})
		require.NoError(t, err)
		assert.False(t, f.Deps.Busy.Active(), "確認の準備だけでは進行中にならない")

		done := make(chan error, 1)
		go func() {
			_, err := svc.Send(t.Context(), s.SessionID, p.ConfirmToken, c)
			done <- err
		}()
		<-bg.entered
		assert.True(t, f.Deps.Busy.Active(), "送信中は進行中")
		close(bg.release)
		require.NoError(t, <-done)
		assert.False(t, f.Deps.Busy.Active(), "送信が終われば空く")
	})
	t.Run("下書きの保存", func(t *testing.T) {
		f := newComposeFixture(t)
		bg := &draftBlockingGmail{Client: f.Gmail, entered: make(chan struct{}), release: make(chan struct{})}
		f.Deps.Gmail = bg
		f.Deps.Busy = &deps.Busy{}
		svc := composecore.New((*deps.Deps)(f.Deps))
		s, err := svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "new", AccountID: f.A.ID})
		require.NoError(t, err)

		done := make(chan error, 1)
		go func() {
			_, err := svc.SaveDraft(t.Context(), s.SessionID, toContent("s", "<p>b</p>"))
			done <- err
		}()
		<-bg.entered
		assert.True(t, f.Deps.Busy.Active(), "下書きの保存中は進行中")
		close(bg.release)
		require.NoError(t, <-done)
		assert.False(t, f.Deps.Busy.Active(), "保存が終われば空く")
	})
}
