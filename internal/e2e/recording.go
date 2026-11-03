//go:build e2e && !production

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

// DraftsDir は RecordingGmail が下書きの内容を書き出すディレクトリ（dir 配下）。
// E2E スクリプトが `adb shell run-as <app> cat files/Hitomado/e2e/drafts/<id>.txt` で読み、返信下書きが Gmail へ渡ったことを確かめる。
const DraftsDir = "e2e/drafts"

// RecordingGmail は FakeGmail の下書きの作成・更新を、解析した内容（宛先・件名・本文）でファイルにも残す。
type RecordingGmail struct {
	*apptest.FakeGmail
	dir string
}

// NewRecordingGmail は dir 配下の DraftsDir に下書きを書き出す gmail.Client を返す。
func NewRecordingGmail(f *apptest.FakeGmail, dir string) *RecordingGmail {
	return &RecordingGmail{FakeGmail: f, dir: filepath.Join(dir, DraftsDir)}
}

var _ gmail.Client = (*RecordingGmail)(nil)

func (r *RecordingGmail) CreateDraft(ctx context.Context, accountID string, raw []byte, threadID string) (string, error) {
	id, err := r.FakeGmail.CreateDraft(ctx, accountID, raw, threadID)
	if err != nil {
		return "", err
	}
	return id, r.record(ctx, accountID, id)
}

func (r *RecordingGmail) UpdateDraft(ctx context.Context, accountID, draftID string, raw []byte) error {
	if err := r.FakeGmail.UpdateDraft(ctx, accountID, draftID, raw); err != nil {
		return err
	}
	return r.record(ctx, accountID, draftID)
}

func (r *RecordingGmail) record(ctx context.Context, accountID, draftID string) error {
	d, err := r.GetDraft(ctx, accountID, draftID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return fmt.Errorf("e2e: create drafts dir: %w", err)
	}
	body := fmt.Sprintf("thread: %s\nsubject: %s\nto: %d\n\n%s\n", d.ThreadID, d.Message.Subject, len(d.Message.To), d.Message.BodyText)
	return os.WriteFile(filepath.Join(r.dir, draftID+".txt"), []byte(body), 0o600)
}
