package composecore

import (
	"context"
	"mime"
	"os"
	"path/filepath"
	"slices"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/store"
)

// attachmentLimitBytes は添付合計の上限（Gmail の 25MB。転送元の添付を含む）。
const attachmentLimitBytes int64 = 25 * 1024 * 1024

func errAttachmentTooLarge() error {
	return apperr.New(apperr.AttachmentTooLarge, "添付ファイルの合計が 25MB を超えています。").
		WithDetail("limitBytes", attachmentLimitBytes)
}

// errAttachmentIO のメッセージにファイル名は含めない（ログ・エラー経由の漏えい防止）。ファイル名は details に入れる。
func errAttachmentIO(filename string) error {
	return apperr.New(apperr.AttachmentIO, "添付ファイルを読み込めません。ファイルを確認してください。").
		WithDetail("filename", filename)
}

// AddAttachments はローカルファイルを添付に追加する（パスと名前・サイズのみ保持し、内容は送信時に読む）。
// 合計（転送の元添付を含む）が 25MB を超えるときは追加分をすべて不採用にする。
func (s *Service) AddAttachments(ctx context.Context, sessionID string, paths []string) ([]dto.ComposeAttachment, error) {
	if _, _, err := s.snapshot(sessionID); err != nil {
		return nil, err
	}
	added := make([]sessionAttachment, 0, len(paths))
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, apperr.FromInfra(err, "")
		}
		info, err := os.Stat(p)
		name := filepath.Base(p)
		if err != nil || !info.Mode().IsRegular() {
			return nil, errAttachmentIO(name)
		}
		mt := mime.TypeByExtension(filepath.Ext(name))
		if mt == "" {
			mt = "application/octet-stream"
		}
		added = append(added, sessionAttachment{
			ID: store.NewID(), Filename: name, MimeType: mt, Size: info.Size(), Source: attachmentLocal, Path: p,
			ModTime: info.ModTime(),
		})
	}
	var out []dto.ComposeAttachment
	err := s.mutate(sessionID, func(cs *composeSession) error {
		total := cs.attachmentsTotal()
		for _, a := range added {
			total += a.Size
		}
		if total > attachmentLimitBytes {
			return errAttachmentTooLarge()
		}
		cs.attachments = append(cs.attachments, added...)
		out = cs.attachmentDTOs()
		return nil
	})
	return out, err
}

// RemoveAttachment は添付を 1 件削除する（転送の元添付も削除できる）。
func (s *Service) RemoveAttachment(_ context.Context, sessionID, attachmentID string) ([]dto.ComposeAttachment, error) {
	var out []dto.ComposeAttachment
	err := s.mutate(sessionID, func(cs *composeSession) error {
		i := slices.IndexFunc(cs.attachments, func(a sessionAttachment) bool { return a.ID == attachmentID })
		if i < 0 {
			return apperr.New(apperr.NotFound, "添付ファイルが見つかりません。")
		}
		cs.attachments = slices.Delete(cs.attachments, i, i+1)
		out = cs.attachmentDTOs()
		return nil
	})
	return out, err
}

// outgoingAttachments は送信用に添付の内容を読む（ローカルはファイル、転送の元添付は Gmail から）。
// 確認時（追加時）のサイズ・更新時刻と異なるローカルファイル、サイズの異なる転送元添付、合計が上限を超える
// 添付は送信しない（確認した内容と違うものを送らない）。
func (s *Service) outgoingAttachments(ctx context.Context, cs composeSession) ([]model.OutgoingAttachment, error) {
	out := make([]model.OutgoingAttachment, 0, len(cs.attachments))
	var total int64
	for _, a := range cs.attachments {
		var (
			data []byte
			err  error
		)
		if a.Source == attachmentForwarded {
			data, err = s.d.Gmail.GetAttachment(ctx, cs.accountID, a.MessageID, a.GmailAtt)
			if err != nil {
				return nil, apperr.FromInfra(err, cs.accountID)
			}
			if int64(len(data)) != a.Size {
				return nil, errAttachmentIO(a.Filename)
			}
		} else if data, err = readUnchanged(a); err != nil {
			return nil, err
		}
		total += int64(len(data))
		if total > attachmentLimitBytes {
			return nil, errAttachmentTooLarge()
		}
		out = append(out, model.OutgoingAttachment{Filename: a.Filename, MimeType: a.MimeType, Data: data})
	}
	return out, nil
}

// readUnchanged はローカル添付を読む。追加時からサイズ・更新時刻が変わっていれば ATTACHMENT_IO。
func readUnchanged(a sessionAttachment) ([]byte, error) {
	info, err := os.Stat(a.Path)
	if err != nil || info.Size() != a.Size || !info.ModTime().Equal(a.ModTime) {
		return nil, errAttachmentIO(a.Filename)
	}
	data, err := os.ReadFile(a.Path) //nolint:gosec // 利用者がダイアログ・ドロップで選んだパス（NF-S09）
	if err != nil || int64(len(data)) != a.Size {
		return nil, errAttachmentIO(a.Filename)
	}
	return data, nil
}
