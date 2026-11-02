package appcore_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/store"
)

func TestInboxCursorRoundTrip(t *testing.T) {
	c := &store.InboxCursor{LastMessageAt: time.UnixMilli(1_700_000_000_123), AccountID: "acc", ThreadID: "thr"}
	got, err := appcore.DecodeInboxCursor(appcore.EncodeInboxCursor(c))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, c.LastMessageAt.UnixMilli(), got.LastMessageAt.UnixMilli())
	assert.Equal(t, "acc", got.AccountID)
	assert.Equal(t, "thr", got.ThreadID)
}

func TestInboxCursorStartAndInvalid(t *testing.T) {
	start := appcore.EncodeInboxCursor(nil)
	assert.NotEmpty(t, start, "先頭を指すカーソルも空文字ではない（続きがあることを伝える）")
	got, err := appcore.DecodeInboxCursor(start)
	require.NoError(t, err)
	assert.Nil(t, got)
	got, err = appcore.DecodeInboxCursor("")
	require.NoError(t, err)
	assert.Nil(t, got)
	for _, bad := range []string{"!!!", "e30", "bm90LWpzb24"} {
		_, err = appcore.DecodeInboxCursor(bad)
		assert.Error(t, err, bad)
	}
}

func TestSearchCursorRoundTrip(t *testing.T) {
	states := map[string]appcore.SearchState{
		"a": {PT: "tok", Rest: []appcore.RestEntry{{I: "t1", T: 5}}},
		"b": {},
		"c": {Rest: []appcore.RestEntry{{I: "t2", T: 7}}},
	}
	enc := appcore.EncodeSearchCursor(states)
	got, err := appcore.DecodeSearchCursor(enc)
	require.NoError(t, err)
	assert.Equal(t, map[string]appcore.SearchState{"a": states["a"], "c": states["c"]}, got, "出し尽くしたアカウントは含めない")
	assert.Empty(t, appcore.EncodeSearchCursor(map[string]appcore.SearchState{"b": {}}))
	_, err = appcore.DecodeSearchCursor("e30")
	assert.Error(t, err)
}

func TestSearchCursorKeepsStartState(t *testing.T) {
	enc := appcore.EncodeSearchCursor(map[string]appcore.SearchState{"a": {Start: true}, "b": {}})
	require.NotEmpty(t, enc, "先頭からの再試行は尽きたアカウントと区別して残す")
	got, err := appcore.DecodeSearchCursor(enc)
	require.NoError(t, err)
	assert.Equal(t, map[string]appcore.SearchState{"a": {Start: true}}, got)
}
