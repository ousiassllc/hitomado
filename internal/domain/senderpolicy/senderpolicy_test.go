package senderpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name            string
		session, thread string
		wantErr         error
	}{
		{"一致", "a1", "a1", nil},
		{"不一致", "a1", "a2", ErrSenderMismatch},
		{"セッション側が空", "", "a2", ErrSenderMismatch},
		{"スレッド側が空", "a1", "", ErrSenderMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, Check(tt.session, tt.thread), tt.wantErr)
		})
	}
}

func TestCanChangeSender(t *testing.T) {
	assert.NoError(t, CanChangeSender(model.ModeNew))
	for _, m := range []model.Mode{model.ModeReply, model.ModeReplyAll, model.ModeForward} {
		assert.ErrorIs(t, CanChangeSender(m), ErrSenderLocked, m)
	}
}
