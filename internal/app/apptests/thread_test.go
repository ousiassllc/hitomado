package apptests

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

func newThreadSvc(t *testing.T) (*app.ThreadService, *apptest.Env) {
	t.Helper()
	e := apptest.NewEnv(t)
	return app.NewThreadService(e.Deps), e
}

func TestGetThread(t *testing.T) {
	svc, e := newThreadSvc(t)
	a := e.AddAccount(t, "me@a.example")
	require.NoError(t, e.Store.ReplaceLabels(t.Context(), a.ID, []store.Label{
		{ID: "Label_1", Name: "Work", Type: "user"}, {ID: "Label_2", Name: "Unused", Type: "user"}, {ID: "INBOX", Name: "INBOX", Type: "system"},
	}))
	m1 := apptest.Msg("m1", "t1", at(1), "INBOX", "Label_1")
	m2 := apptest.Msg("m2", "t1", at(3), "INBOX", "UNREAD")
	m2.BodyHTML = `<p>hello <img src="cid:logo@x"></p>`
	m2.InlineImages = map[string]gmail.InlinePart{"logo@x": {MimeType: "image/png", Data: []byte{1, 2, 3}}}
	m2.Attachments = []model.AttachmentInfo{{ID: "att1", Filename: "a.pdf", MimeType: "application/pdf", Size: 10}}
	draft := apptest.Msg("m-draft", "t1", at(2), "DRAFT")
	e.PutThread(t, a.ID, "t1", m2, draft, m1) // 順不同
	e.Gmail.Drafts[apptest.Key(a.ID, "d1")] = gmail.DraftFull{ID: "d1", ThreadID: "t1"}
	calls := len(e.Gmail.Modified)

	d, err := svc.GetThread(t.Context(), ref(a.ID, "t1"))
	require.NoError(t, err)
	require.Len(t, d.Messages, 2, "下書きは除く")
	assert.Equal(t, []string{"m1", "m2"}, []string{d.Messages[0].ID, d.Messages[1].ID}, "時系列昇順")
	assert.Equal(t, "subject t1", d.Subject)
	assert.Equal(t, "d1", d.DraftID)
	assert.Equal(t, a.ID, d.Account.ID)
	assert.Equal(t, "me@a.example", d.Account.Email)
	require.Len(t, d.Labels, 1)
	assert.Equal(t, "Work", d.Labels[0].Name)
	assert.True(t, d.Messages[1].IsUnread)
	assert.False(t, d.Messages[0].IsUnread)
	assert.Contains(t, d.Messages[1].BodyHTML, "data:image/png;base64,AQID")
	assert.NotContains(t, d.Messages[1].BodyHTML, "cid:")
	assert.False(t, d.Messages[1].HasRemoteImages, "data: URL はリモート画像ではない")
	assert.Equal(t, []model.AttachmentInfo{{ID: "att1", Filename: "a.pdf", MimeType: "application/pdf", Size: 10}}, d.Messages[1].Attachments)
	assert.NotNil(t, d.Messages[0].Attachments)
	assert.NotNil(t, d.Messages[0].Cc)

	assert.Len(t, e.Gmail.Modified, calls, "状態を変更しない")
	row, err := e.Store.GetThread(t.Context(), a.ID, "t1")
	require.NoError(t, err)
	assert.True(t, row.IsUnread, "既読化しない")

	_, err = svc.GetThread(t.Context(), ref(a.ID, "missing"))
	_ = requireCode(t, apperr.NotFound, err)
	_, err = svc.GetThread(t.Context(), ref("nope", "t1"))
	_ = requireCode(t, apperr.NotFound, err)
}

func TestGetThreadWithoutDraftAndTextOnly(t *testing.T) {
	svc, e := newThreadSvc(t)
	a := e.AddAccount(t, "me@a.example")
	m := apptest.Msg("m1", "t1", at(1), "INBOX")
	m.BodyHTML, m.BodyText = "", "plain text"
	e.PutThread(t, a.ID, "t1", m)
	d, err := svc.GetThread(t.Context(), ref(a.ID, "t1"))
	require.NoError(t, err)
	assert.Empty(t, d.DraftID)
	assert.Equal(t, "plain text", d.Messages[0].BodyText)
	assert.Empty(t, d.Messages[0].BodyHTML)
	assert.NotNil(t, d.Labels)
}

func TestGetThreadRemoteImages(t *testing.T) {
	svc, e := newThreadSvc(t)
	a := e.AddAccount(t, "me@a.example")
	cases := map[string]bool{
		`<p>no images</p>`:                                                   false,
		`<img src="https://t.example/p.gif">`:                                true,
		`<img src="HTTP://t.example/p.gif">`:                                 true,
		`<img src="//t.example/p.gif">`:                                      true,
		`<img srcset="a.png 1x, https://t.example/b.png 2x">`:                true,
		`<img srcset="a.png 1x, b.png 2x">`:                                  false,
		`<table background="https://t.example/bg.png"><tr></tr></table>`:     true,
		`<div style="background:url('https://t.example/x.png')"></div>`:      true,
		`<div style="background:url(data:image/png;base64,AA==)"></div>`:     false,
		`<style>.a{background:url(https://t.example/x.png)}</style><p>x</p>`: true,
		`<a href="https://t.example/">link only</a>`:                         false,
		`<img src="data:image/png;base64,AA==">`:                             false,
	}
	i := 0
	for body, want := range cases {
		i++
		id := "t" + string(rune('a'+i))
		m := apptest.Msg("m"+id, id, at(i), "INBOX")
		m.BodyHTML = body
		e.PutThread(t, a.ID, id, m)
		d, err := svc.GetThread(t.Context(), ref(a.ID, id))
		require.NoError(t, err)
		assert.Equal(t, want, d.Messages[0].HasRemoteImages, body)
	}
}

func TestGetThreadNeedsReauth(t *testing.T) {
	svc, e := newThreadSvc(t)
	a := e.AddAccount(t, "me@a.example")
	e.Gmail.Err["GetThreadFull"] = gmail.ErrNeedsReauth
	_, err := svc.GetThread(t.Context(), ref(a.ID, "t1"))
	_ = requireCode(t, apperr.NeedsReauth, err)
	got, _ := e.Store.GetAccount(t.Context(), a.ID)
	assert.Equal(t, store.StatusNeedsReauth, got.Status)
}

func TestOpenExternal(t *testing.T) {
	svc, e := newThreadSvc(t)
	for _, ok := range []string{"https://example.com/a?b=1", "http://example.com", "mailto:someone@example.com?subject=hi", " HTTPS://Example.com/ "} {
		require.NoError(t, svc.OpenExternal(t.Context(), ok), ok)
	}
	assert.Len(t, e.Browser.URLs, 4)
	for _, bad := range []string{"javascript:alert(1)", "file:///etc/passwd", "ftp://x.example", "data:text/html,x", "cmd:/c calc", "//example.com", "example.com", "", "https://", "mailto:", "http://[::1", "ms-msdt:/id x"} {
		_ = requireCode(t, apperr.Validation, svc.OpenExternal(t.Context(), bad))
	}
	assert.Len(t, e.Browser.URLs, 4, "拒否した URL は開かない")
}

func saveSetup(t *testing.T) (*app.ThreadService, *apptest.Env, store.Account) {
	svc, e := newThreadSvc(t)
	a := e.AddAccount(t, "me@a.example")
	e.Gmail.Attachments[apptest.Key(a.ID, "m1", "att1")] = []byte("PDFDATA")
	return svc, e, a
}

func TestSaveAttachment(t *testing.T) {
	svc, e, a := saveSetup(t)
	dir := t.TempDir()
	e.Files.Path = filepath.Join(dir, "out.pdf")

	res, err := svc.SaveAttachment(t.Context(), dto.MessageRef{AccountID: a.ID, ThreadID: "t1", MessageID: "m1"}, "att1", `re:port/2026?*.pdf`)
	require.NoError(t, err)
	assert.Equal(t, dto.SaveAttachmentResult{Saved: true, Path: e.Files.Path}, res)
	assert.Equal(t, []string{"re_port_2026__.pdf"}, e.Files.Suggested)
	b, err := os.ReadFile(e.Files.Path)
	require.NoError(t, err)
	assert.Equal(t, "PDFDATA", string(b))
	entries, _ := os.ReadDir(dir)
	assert.Len(t, entries, 1, "一時ファイルを残さない")
}

func TestSaveAttachmentCancelled(t *testing.T) {
	svc, e, a := saveSetup(t)
	e.Files.Cancel = true
	res, err := svc.SaveAttachment(t.Context(), dto.MessageRef{AccountID: a.ID, ThreadID: "t1", MessageID: "m1"}, "att1", "a.pdf")
	require.NoError(t, err)
	assert.False(t, res.Saved)
	assert.Empty(t, res.Path)
	assert.Zero(t, e.Gmail.CallCount("GetAttachment"), "キャンセルなら取得しない")
}

func TestSaveAttachmentErrors(t *testing.T) {
	svc, e, a := saveSetup(t)
	mref := dto.MessageRef{AccountID: a.ID, ThreadID: "t1", MessageID: "m1"}

	e.Files.Path = filepath.Join(t.TempDir(), "no-such-dir", "out.pdf")
	_, err := svc.SaveAttachment(t.Context(), mref, "att1", "a.pdf")
	_ = requireCode(t, apperr.AttachmentIO, err)

	e.Files.Path = filepath.Join(t.TempDir(), "ok.pdf")
	_, err = svc.SaveAttachment(t.Context(), mref, "missing", "a.pdf")
	_ = requireCode(t, apperr.AttachmentIO, err)
	_, statErr := os.Stat(e.Files.Path)
	assert.True(t, os.IsNotExist(statErr), "取得失敗ではファイルを作らない")

	e.Gmail.Err["GetAttachment"] = gmail.ErrOffline
	_, err = svc.SaveAttachment(t.Context(), mref, "att1", "a.pdf")
	_ = requireCode(t, apperr.Offline, err)

	e.Files.Err = os.ErrPermission
	_, err = svc.SaveAttachment(t.Context(), mref, "att1", "a.pdf")
	_ = requireCode(t, apperr.AttachmentIO, err)

	_, err = svc.SaveAttachment(t.Context(), mref, "", "a.pdf")
	_ = requireCode(t, apperr.Validation, err)
	_, err = svc.SaveAttachment(t.Context(), dto.MessageRef{AccountID: "nope", MessageID: "m1"}, "att1", "a.pdf")
	_ = requireCode(t, apperr.NotFound, err)
}

func TestSaveAttachmentFilenameSanitizing(t *testing.T) {
	svc, e, a := saveSetup(t)
	e.Files.Cancel = true
	mref := dto.MessageRef{AccountID: a.ID, ThreadID: "t1", MessageID: "m1"}
	cases := map[string]string{
		"normal.txt":      "normal.txt",
		`a<b>c:d"e|f.txt`: "a_b_c_d_e_f.txt",
		`..\..\evil.exe`:  `.._.._evil.exe`,
		"line\nbreak.txt": "line_break.txt",
		"trailing. . ":    "trailing",
		"":                "attachment",
		"   ":             "attachment",
		"...":             "attachment",
		"CON":             "_CON",
		"nul.txt":         "_nul.txt",
		"日本語の資料.pdf":      "日本語の資料.pdf",
	}
	for in, want := range cases {
		_, err := svc.SaveAttachment(t.Context(), mref, "att1", in)
		require.NoError(t, err)
		assert.Equal(t, want, e.Files.Suggested[len(e.Files.Suggested)-1], "入力 %q", in)
	}
}

func TestSaveAttachmentReplacesExistingFileAtomically(t *testing.T) {
	svc, e, a := saveSetup(t)
	dir := t.TempDir()
	e.Files.Path = filepath.Join(dir, "out.pdf")
	require.NoError(t, os.WriteFile(e.Files.Path, []byte("OLD"), 0o600)) // 上書き確認済みの既存ファイル
	mref := dto.MessageRef{AccountID: a.ID, ThreadID: "t1", MessageID: "m1"}

	// 取得に失敗しても、既存ファイルは壊さず・消さない。
	_, err := svc.SaveAttachment(t.Context(), mref, "missing", "a.pdf")
	_ = requireCode(t, apperr.AttachmentIO, err)
	b, err := os.ReadFile(e.Files.Path)
	require.NoError(t, err)
	assert.Equal(t, "OLD", string(b))

	_, err = svc.SaveAttachment(t.Context(), mref, "att1", "a.pdf")
	require.NoError(t, err)
	b, _ = os.ReadFile(e.Files.Path)
	assert.Equal(t, "PDFDATA", string(b))
	entries, _ := os.ReadDir(dir)
	assert.Len(t, entries, 1, "一時ファイルを残さない")
}

// 権限ビットの検証だけを Windows でスキップする（既存ファイルの置き換えで一時ファイルが残らないことは
// TestSaveAttachmentReplacesExistingFileAtomically が Windows でも検証する）。
func TestSaveAttachmentPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows のファイルモードは POSIX の権限ビットを持たない")
	}
	svc, e, a := saveSetup(t)
	mref := dto.MessageRef{AccountID: a.ID, ThreadID: "t1", MessageID: "m1"}
	dir := t.TempDir()

	e.Files.Path = filepath.Join(dir, "new.pdf")
	_, err := svc.SaveAttachment(t.Context(), mref, "att1", "a.pdf")
	require.NoError(t, err)
	fi, err := os.Stat(e.Files.Path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), fi.Mode().Perm(), "新規は 0644")

	e.Files.Path = filepath.Join(dir, "existing.pdf")
	require.NoError(t, os.WriteFile(e.Files.Path, []byte("OLD"), 0o600))
	_, err = svc.SaveAttachment(t.Context(), mref, "att1", "a.pdf")
	require.NoError(t, err)
	fi, err = os.Stat(e.Files.Path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm(), "既存ファイルの権限を保つ（新規の既定 0644 とは異なる値）")
}
