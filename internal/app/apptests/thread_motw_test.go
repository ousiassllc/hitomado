package apptests

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// 差し替え用: MarkInternetZone を一時的に置き換える（このパッケージのテストは Parallel を使わない）。
func stubMarkInternetZone(t *testing.T, fn func(string) error) {
	t.Helper()
	orig := appcore.MarkInternetZone
	appcore.MarkInternetZone = fn
	t.Cleanup(func() { appcore.MarkInternetZone = orig })
}

func TestSaveAttachmentMarksSavedFile(t *testing.T) {
	svc, e, a := saveSetup(t)
	e.Files.Path = filepath.Join(t.TempDir(), "out.pdf")
	require.NoError(t, os.WriteFile(e.Files.Path, []byte("OLD"), 0o600)) // 置換する場合も置換後に付ける
	var marked []string
	var contentAtMark string
	stubMarkInternetZone(t, func(p string) error {
		marked = append(marked, p)
		b, _ := os.ReadFile(p) //nolint:gosec // テストの一時ファイル
		contentAtMark = string(b)
		return nil
	})

	_, err := svc.SaveAttachment(t.Context(), dto.MessageRef{AccountID: a.ID, ThreadID: "t1", MessageID: "m1"}, "att1", "a.pdf")
	require.NoError(t, err)
	assert.Equal(t, []string{e.Files.Path}, marked)
	assert.Equal(t, "PDFDATA", contentAtMark, "rename 後（置換後のファイル）に付ける")
}

func TestSaveAttachmentSucceedsWhenMarkFails(t *testing.T) {
	svc, e, a := saveSetup(t)
	e.Files.Path = filepath.Join(t.TempDir(), "out.pdf")
	stubMarkInternetZone(t, func(string) error { return errors.New("ADS unsupported on this volume") })

	res, err := svc.SaveAttachment(t.Context(), dto.MessageRef{AccountID: a.ID, ThreadID: "t1", MessageID: "m1"}, "att1", "a.pdf")
	require.NoError(t, err, "マークの失敗は保存の失敗にしない")
	assert.True(t, res.Saved)
	b, err := os.ReadFile(e.Files.Path)
	require.NoError(t, err)
	assert.Equal(t, "PDFDATA", string(b))
}

func TestSaveAttachmentDoesNotMarkOnFailureOrCancel(t *testing.T) {
	svc, e, a := saveSetup(t)
	called := 0
	stubMarkInternetZone(t, func(string) error { called++; return nil })
	mref := dto.MessageRef{AccountID: a.ID, ThreadID: "t1", MessageID: "m1"}

	e.Files.Cancel = true
	_, err := svc.SaveAttachment(t.Context(), mref, "att1", "a.pdf")
	require.NoError(t, err)
	e.Files.Cancel = false
	e.Files.Path = filepath.Join(t.TempDir(), "no-such-dir", "out.pdf")
	_, err = svc.SaveAttachment(t.Context(), mref, "att1", "a.pdf")
	require.Error(t, err)
	assert.Zero(t, called)
}
