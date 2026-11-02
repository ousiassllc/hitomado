package app

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// SaveAttachment はネイティブの保存ダイアログで選ばれた場所へ添付を書き込む。キャンセルは Saved=false。
// 保存先と同じディレクトリの一時ファイルへ書いてから置き換える（失敗時は一時ファイルを削除し、既存ファイルは壊さない）。
func (s *ThreadService) SaveAttachment(ctx context.Context, ref dto.MessageRef, attachmentID, suggestedName string) (dto.SaveAttachmentResult, error) {
	if ref.MessageID == "" || attachmentID == "" {
		return dto.SaveAttachmentResult{}, validation("添付ファイルの指定が正しくありません。")
	}
	if _, err := s.d.Store.GetAccount(ctx, ref.AccountID); err != nil {
		return dto.SaveAttachmentResult{}, apperr.FromInfra(err, ref.AccountID)
	}
	path, ok, err := s.d.Files.SaveFile(ctx, appcore.SanitizeFilename(suggestedName))
	if err != nil {
		return dto.SaveAttachmentResult{}, apperr.Wrap(apperr.AttachmentIO, "保存先を選択できませんでした。", err)
	}
	if !ok {
		return dto.SaveAttachmentResult{}, nil
	}
	data, err := s.d.Gmail.GetAttachment(ctx, ref.AccountID, ref.MessageID, attachmentID)
	if err != nil {
		ae := gmailErr(s.d, ctx, ref.AccountID, err)
		if ae.Code == apperr.NotFound {
			ae = apperr.Wrap(apperr.AttachmentIO, "添付ファイルを取得できません。", err).ForAccount(ref.AccountID)
		}
		return dto.SaveAttachmentResult{}, ae
	}
	if err := writeFileReplacing(path, data); err != nil {
		return dto.SaveAttachmentResult{}, apperr.Wrap(apperr.AttachmentIO, "ファイルを保存できませんでした。", err)
	}
	return dto.SaveAttachmentResult{Saved: true, Path: path}, nil
}

// OpenExternal は http／https／mailto のみ許可する（NF-S07）。
func (s *ThreadService) OpenExternal(_ context.Context, rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return validation("URL が正しくありません。")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Hostname() == "" {
			return validation("URL が正しくありません。")
		}
	case "mailto":
		if u.Opaque == "" && u.Path == "" {
			return validation("URL が正しくありません。")
		}
	default:
		return validation("このリンクは開けません。http／https／mailto のみ対応しています。")
	}
	if err := s.d.Browser.Open(u.String()); err != nil {
		return apperr.Wrap(apperr.Internal, "リンクを開けませんでした。", err)
	}
	return nil
}

// writeFileReplacing は path と同じディレクトリの一時ファイルに書き、成功したら path へ置き換える。
// どの失敗経路でも一時ファイルは残さず、path に元からあったファイルを壊さない。
func writeFileReplacing(path string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hitomado-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
