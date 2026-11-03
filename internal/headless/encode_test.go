package headless

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/notify"
)

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m))
	return m
}

func TestEncodeIsASCIIAndRoundTrips(t *testing.T) {
	s := Encode(Result{
		NewMail: []notify.NewMailNotice{{
			AccountID: "a", AccountEmail: "a@example.com", AccountName: "山田 😀", ThreadID: "t1", FromName: "差出人", Subject: "件名 🎉",
		}},
		Accounts: []AccountResult{{ID: "a", Email: "a@example.com", Status: StatusNeedsReauth}},
	}, nil)
	for _, r := range s {
		require.Less(t, r, rune(0x80), "JNI の NewStringUTF のため ASCII だけにする: %q", s)
	}
	m := decode(t, s)
	assert.Equal(t, true, m["ok"])
	mail := m["newMail"].([]any)[0].(map[string]any)
	assert.Equal(t, "山田 😀", mail["accountName"])
	assert.Equal(t, "件名 🎉", mail["subject"])
	assert.Equal(t, "差出人", mail["from"])
	assert.Equal(t, "a@example.com", mail["accountEmail"])
	acct := m["accounts"].([]any)[0].(map[string]any)
	assert.Equal(t, "needs_reauth", acct["status"])
}

func TestEncodeEmptyResultHasEmptyArrays(t *testing.T) {
	m := decode(t, Encode(Result{}, nil))
	assert.Equal(t, []any{}, m["newMail"])
	assert.Equal(t, []any{}, m["accounts"])
}

func TestEncodeFailureCarriesOnlyACode(t *testing.T) {
	m := decode(t, Encode(Result{}, errors.New("boom: secret@example.com")))
	assert.Equal(t, false, m["ok"])
	assert.Equal(t, "failed", m["error"])
	assert.NotContains(t, Encode(Result{}, errors.New("boom: secret@example.com")), "secret")
	assert.Equal(t, "busy", decode(t, Encode(Result{}, ErrBusy))["error"])
}

func TestEncodeAppRunning(t *testing.T) {
	m := decode(t, Encode(Result{AppRunning: true}, nil))
	assert.Equal(t, true, m["appRunning"])
}
