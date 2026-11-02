package composecore

import (
	"context"
	"fmt"
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

func errAttachmentIO(filename string) error {
	return apperr.New(apperr.AttachmentIO, fmt.Sprintf("添付ファイル「%s」を読み込めません。", filename))
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
// 読み込んだ内容が確認時のサイズと違う、または合計が上限を超えるときは送信しない。
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
		} else if data, err = os.ReadFile(a.Path); err != nil { //nolint:gosec // 利用者がダイアログ・ドロップで選んだパス（NF-S09）
			return nil, errAttachmentIO(a.Filename)
		}
		if a.Source == attachmentLocal && int64(len(data)) != a.Size {
			return nil, errAttachmentIO(a.Filename) // 確認後にファイルが変更された
		}
		total += int64(len(data))
		if total > attachmentLimitBytes {
			return nil, errAttachmentTooLarge()
		}
		out = append(out, model.OutgoingAttachment{Filename: a.Filename, MimeType: a.MimeType, Data: data})
	}
	return out, nil
}
