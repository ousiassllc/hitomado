package composecore

import (
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/exportfile"
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
		added = append(added, sessionAttachment{
			ID: store.NewID(), Filename: name, MimeType: exportfile.MIMEForName(name), Size: info.Size(), Source: attachmentLocal, Path: p,
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

// outgoingAttachments は MIME 用に添付の内容を揃える（送信・下書き保存の両方で使う）。
//   - local: ファイルを読む。追加時からサイズ・更新時刻が変わっていれば ATTACHMENT_IO。
//   - forwarded: 保持済みならそれを、無ければ Gmail から取得する（取得できたものは fetched に返す。
//     呼び出し側がセッションに保持して、保存の度の再取得を避ける）。サイズが違えば ATTACHMENT_IO。
//   - draft: 復元時に保持したバイト列。
//
// 合計が上限を超える場合は ATTACHMENT_TOO_LARGE（確認した内容と違うものを送らない）。
func (s *Service) outgoingAttachments(ctx context.Context, cs composeSession) (out []model.OutgoingAttachment, fetched map[string][]byte, err error) {
	out = make([]model.OutgoingAttachment, 0, len(cs.attachments))
	fetched = map[string][]byte{}
	var total int64
	for _, a := range cs.attachments {
		data := a.Data
		switch {
		case a.Source == attachmentLocal:
			if data, err = readUnchanged(a); err != nil {
				return nil, nil, err
			}
		case data == nil && a.Source == attachmentForwarded:
			if data, err = s.d.Gmail.GetAttachment(ctx, cs.accountID, a.MessageID, a.GmailAtt); err != nil {
				return nil, nil, apperr.FromInfra(err, cs.accountID)
			}
			if int64(len(data)) != a.Size {
				return nil, nil, errAttachmentIO(a.Filename)
			}
			fetched[a.ID] = data
		}
		total += int64(len(data))
		if total > attachmentLimitBytes {
			return nil, nil, errAttachmentTooLarge()
		}
		out = append(out, model.OutgoingAttachment{Filename: a.Filename, MimeType: a.MimeType, Data: data})
	}
	return out, fetched, nil
}

// keepFetched は取得済みの転送元添付をセッションに保持する（同じセッション内の再保存・送信で再取得しない）。
func (s *Service) keepFetched(sessionID string, fetched map[string][]byte) {
	if len(fetched) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	for i := range cs.attachments {
		if data, ok := fetched[cs.attachments[i].ID]; ok {
			cs.attachments[i].Data = data
		}
	}
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
