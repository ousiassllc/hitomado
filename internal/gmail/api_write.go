package gmail

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/ousiassllc/hitomado/internal/gmail/mimeparse"
)

// ModifyThread はスレッドのラベルを付与・除去する（threads.modify）。
func (c *client) ModifyThread(ctx context.Context, accountID, threadID string, add, remove []string) error {
	req := &gmailapi.ModifyThreadRequest{AddLabelIds: add, RemoveLabelIds: remove}
	return c.do(ctx, accountID, callSpec{name: "threads.modify", units: unitsThread, retry5xx: true}, func(s *gmailapi.Service) error {
		_, err := s.Users.Threads.Modify(userID, threadID, req).Context(ctx).Do()
		return err
	})
}

// TrashThread はスレッドをゴミ箱へ移す（threads.trash）。
func (c *client) TrashThread(ctx context.Context, accountID, threadID string) error {
	return c.do(ctx, accountID, callSpec{name: "threads.trash", units: unitsThread, retry5xx: true}, func(s *gmailapi.Service) error {
		_, err := s.Users.Threads.Trash(userID, threadID).Context(ctx).Do()
		return err
	})
}

// GetAttachment は添付データを取得する。取得できなければ ErrAttachmentGone（404 は ErrNotFound も満たす）。
func (c *client) GetAttachment(ctx context.Context, accountID, messageID, attachmentID string) ([]byte, error) {
	var body *gmailapi.MessagePartBody
	err := c.do(ctx, accountID, callSpec{name: "attachments.get", units: unitsAttachment, retry5xx: true}, func(s *gmailapi.Service) error {
		var err error
		body, err = s.Users.Messages.Attachments.Get(userID, messageID, attachmentID).Context(ctx).Do()
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("%w: %w", ErrAttachmentGone, err)
	}
	if err != nil {
		return nil, err
	}
	data, err := mimeparse.DecodeBase64URL(body.Data)
	if err != nil {
		return nil, fmt.Errorf("gmail: 添付のデコードに失敗: %w", ErrAttachmentGone)
	}
	return data, nil
}

// encodeRaw は生 MIME を Gmail の raw（base64url）にする。
func encodeRaw(raw []byte) string { return base64.URLEncoding.EncodeToString(raw) }

// Send は生 MIME を送信する（messages.send）。5xx は二重送信を避けるため再試行しない。
func (c *client) Send(ctx context.Context, accountID string, raw []byte, threadID string) (SentMessage, error) {
	msg := &gmailapi.Message{Raw: encodeRaw(raw), ThreadId: threadID}
	var sent *gmailapi.Message
	err := c.do(ctx, accountID, callSpec{name: "messages.send", units: unitsMessageSend}, func(s *gmailapi.Service) error {
		var err error
		sent, err = s.Users.Messages.Send(userID, msg).Context(ctx).Do()
		return err
	})
	if err != nil {
		return SentMessage{}, err
	}
	return SentMessage{MessageID: sent.Id, ThreadID: sent.ThreadId}, nil
}

// CreateDraft は下書きを作成する（drafts.create）。5xx は重複作成を避けるため再試行しない。
func (c *client) CreateDraft(ctx context.Context, accountID string, raw []byte, threadID string) (string, error) {
	draft := &gmailapi.Draft{Message: &gmailapi.Message{Raw: encodeRaw(raw), ThreadId: threadID}}
	var created *gmailapi.Draft
	err := c.do(ctx, accountID, callSpec{name: "drafts.create", units: unitsDraft}, func(s *gmailapi.Service) error {
		var err error
		created, err = s.Users.Drafts.Create(userID, draft).Context(ctx).Do()
		return err
	})
	if err != nil {
		return "", err
	}
	return created.Id, nil
}

// UpdateDraft は下書きの内容を置き換える（drafts.update）。
func (c *client) UpdateDraft(ctx context.Context, accountID, draftID string, raw []byte) error {
	draft := &gmailapi.Draft{Message: &gmailapi.Message{Raw: encodeRaw(raw)}}
	return c.do(ctx, accountID, callSpec{name: "drafts.update", units: unitsDraft, retry5xx: true}, func(s *gmailapi.Service) error {
		_, err := s.Users.Drafts.Update(userID, draftID, draft).Context(ctx).Do()
		return err
	})
}

// GetDraft は下書きを format=full で取得する。
func (c *client) GetDraft(ctx context.Context, accountID, draftID string) (DraftFull, error) {
	var d *gmailapi.Draft
	err := c.do(ctx, accountID, callSpec{name: "drafts.get", units: unitsDraft, retry5xx: true}, func(s *gmailapi.Service) error {
		var err error
		d, err = s.Users.Drafts.Get(userID, draftID).Format("full").Context(ctx).Do()
		return err
	})
	if err != nil {
		return DraftFull{}, err
	}
	if d.Message == nil {
		return DraftFull{}, fmt.Errorf("gmail: drafts.get: 下書きにメッセージがありません: %w", ErrGeneric)
	}
	mf, err := c.messageFull(ctx, accountID, d.Message)
	if err != nil {
		return DraftFull{}, err
	}
	return DraftFull{ID: d.Id, ThreadID: d.Message.ThreadId, Message: mf, Attachments: mf.Attachments}, nil
}

// DeleteDraft は下書きを削除する（drafts.delete）。
func (c *client) DeleteDraft(ctx context.Context, accountID, draftID string) error {
	return c.do(ctx, accountID, callSpec{name: "drafts.delete", units: unitsDraft, retry5xx: true}, func(s *gmailapi.Service) error {
		return s.Users.Drafts.Delete(userID, draftID).Context(ctx).Do()
	})
}

// FindDraftForThread は drafts.list を全ページ走査し、threadID に紐づく下書きの ID を返す。
func (c *client) FindDraftForThread(ctx context.Context, accountID, threadID string) (string, bool, error) {
	pageToken := ""
	for {
		var resp *gmailapi.ListDraftsResponse
		err := c.do(ctx, accountID, callSpec{name: "drafts.list", units: unitsDraft, retry5xx: true}, func(s *gmailapi.Service) error {
			call := s.Users.Drafts.List(userID).Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			var err error
			resp, err = call.Do()
			return err
		})
		if err != nil {
			return "", false, err
		}
		for _, d := range resp.Drafts {
			if d.Message != nil && d.Message.ThreadId == threadID {
				return d.Id, true, nil
			}
		}
		if resp.NextPageToken == "" {
			return "", false, nil
		}
		pageToken = resp.NextPageToken
	}
}

// ListLabels はラベル一覧を返す。ユーザーラベルは labels.get で色を補う（削除競合の 404 は色なしで残す）。
func (c *client) ListLabels(ctx context.Context, accountID string) ([]Label, error) {
	var list *gmailapi.ListLabelsResponse
	err := c.do(ctx, accountID, callSpec{name: "labels.list", units: unitsLabels, retry5xx: true}, func(s *gmailapi.Service) error {
		var err error
		list, err = s.Users.Labels.List(userID).Context(ctx).Do()
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]Label, 0, len(list.Labels))
	for _, l := range list.Labels {
		label := Label{ID: l.Id, Name: l.Name, Type: l.Type}
		if l.Color != nil {
			label.ColorBg, label.ColorFg = l.Color.BackgroundColor, l.Color.TextColor
		} else if l.Type == "user" {
			if err := c.fillLabelColor(ctx, accountID, &label); err != nil {
				return nil, err
			}
		}
		out = append(out, label)
	}
	return out, nil
}

func (c *client) fillLabelColor(ctx context.Context, accountID string, label *Label) error {
	var full *gmailapi.Label
	err := c.do(ctx, accountID, callSpec{name: "labels.get", units: unitsLabels, retry5xx: true}, func(s *gmailapi.Service) error {
		var err error
		full, err = s.Users.Labels.Get(userID, label.ID).Context(ctx).Do()
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if full.Color != nil {
		label.ColorBg, label.ColorFg = full.Color.BackgroundColor, full.Color.TextColor
	}
	return nil
}
