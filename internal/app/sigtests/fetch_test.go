// Package sigtests は AccountService.FetchGmailSignature（Gmail の署名の取り込み。#110）の外部テスト。
package sigtests

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

func newAccountSvc(t *testing.T) (*app.AccountService, *apptest.Env) {
	t.Helper()
	e := apptest.NewEnv(t)
	return app.NewAccountService(e.Deps), e
}

func requireCode(t *testing.T, want string, err error) {
	t.Helper()
	var ae *apperr.Error
	require.ErrorAs(t, err, &ae)
	require.Equal(t, want, ae.Code)
}

func TestFetchGmailSignatureFormatsAndDoesNotSave(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	e.Gmail.Signatures[a.ID] = `<div dir="ltr"><div>山田 太郎</div><div><a href="https://example.com">Example</a></div></div>`

	got, err := svc.FetchGmailSignature(t.Context(), a.ID)
	require.NoError(t, err)
	assert.Equal(t, dto.GmailSignature{
		SignatureHTML: `<p>山田 太郎</p><p><a href="https://example.com">Example</a></p>`, Found: true,
	}, got)
	assert.Equal(t, []string{"me@a.example"}, e.Gmail.SignatureRequests, "sendAsEmail はアカウント本人のアドレス")

	saved, err := e.Store.GetAccount(t.Context(), a.ID)
	require.NoError(t, err)
	assert.Empty(t, saved.SignatureHTML, "取得しただけでは保存しない")
}

func TestFetchGmailSignatureReportsDroppedContent(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	e.Gmail.Signatures[a.ID] = `<div>Taro<img src="https://example.com/logo.png"></div>`

	got, err := svc.FetchGmailSignature(t.Context(), a.ID)
	require.NoError(t, err)
	assert.Equal(t, dto.GmailSignature{SignatureHTML: "<p>Taro</p>", Found: true, Sanitized: true}, got)
}

func TestFetchGmailSignatureEmpty(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	for _, raw := range []string{"", "  \n"} {
		e.Gmail.Signatures[a.ID] = raw
		got, err := svc.FetchGmailSignature(t.Context(), a.ID)
		require.NoError(t, err)
		assert.Equal(t, dto.GmailSignature{}, got)
	}
}

func TestFetchGmailSignatureNothingImportable(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	e.Gmail.Signatures[a.ID] = `<img src="https://example.com/logo.png">`

	got, err := svc.FetchGmailSignature(t.Context(), a.ID)
	require.NoError(t, err)
	assert.Equal(t, dto.GmailSignature{Sanitized: true}, got, "取り込める内容が残らなければ found は偽")
}

func TestFetchGmailSignatureErrors(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	e.Gmail.Signatures[a.ID] = "<p>x</p>"

	_, err := svc.FetchGmailSignature(t.Context(), "missing")
	requireCode(t, apperr.NotFound, err)
	assert.Zero(t, e.Gmail.CallCount("GetSignature"), "未登録のアカウントでは Gmail を呼ばない")

	for _, tc := range []struct {
		err  error
		code string
	}{
		{gmail.ErrOffline, apperr.Offline},
		{gmail.ErrRateLimited, apperr.RateLimited},
		{gmail.ErrNeedsReauth, apperr.NeedsReauth},
	} {
		e.Gmail.Err["GetSignature"] = tc.err
		_, err := svc.FetchGmailSignature(t.Context(), a.ID)
		requireCode(t, tc.code, err)
	}
	got, _ := e.Store.GetAccount(t.Context(), a.ID)
	assert.Equal(t, store.StatusNeedsReauth, got.Status, "権限エラーはアカウントを要再接続にする")
	assert.Empty(t, got.SignatureHTML)
}

func TestFetchGmailSignatureNotInLogs(t *testing.T) {
	svc, e := newAccountSvc(t)
	var buf bytes.Buffer
	e.Deps.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	a := e.AddAccount(t, "me@a.example")
	e.Gmail.Signatures[a.ID] = "<p>SECRET-SIGNATURE-BODY</p>"

	_, err := svc.FetchGmailSignature(t.Context(), a.ID)
	require.NoError(t, err)
	e.Gmail.Err["GetSignature"] = gmail.ErrOffline
	_, err = svc.FetchGmailSignature(t.Context(), a.ID)
	require.Error(t, err)
	assert.NotContains(t, buf.String(), "SECRET-SIGNATURE-BODY")
}
