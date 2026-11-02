package app

import (
	"context"
	"net/url"
	"os"
	"strings"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// SaveAttachment はネイティブの保存ダイアログで選ばれた場所へ添付を書き込む。キャンセルは Saved=false。
// 一時ファイルは作らず、保存先へ直接書く（失敗時は書きかけを削除する）。
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
	if err := os.WriteFile(path, data, 0o600); err != nil {
		_ = os.Remove(path) // 書きかけを残さない（失敗は無視。元のエラーを返す）
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
