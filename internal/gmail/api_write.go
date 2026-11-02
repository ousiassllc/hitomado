package gmail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"

	"github.com/ousiassllc/hitomado/internal/domain/model"
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
	err := c.do(ctx, accountID, callSpec{name: "attachments.get", units: unitsAttachment, retry5xx: true, transfer: true}, func(s *gmailapi.Service) error {
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

// uploadOptions は生 MIME のアップロード用オプション。Content-Type を message/rfc822 に固定し、
// チャンク分割（再開可能アップロード）を無効にして 1 リクエストで送る（リクエストごとに本文を作り直せるので再試行しやすい）。
var uploadOptions = []googleapi.MediaOption{googleapi.ContentType("message/rfc822"), googleapi.ChunkSize(0)}

// Send は生 MIME を送信する（messages.send）。raw を JSON に埋め込むと大きな添付で
// リクエスト上限（413）に当たり得るので、メディアアップロード経路で送る。
// 5xx は二重送信を避けるため再試行しない。
// 応答を受け取れずに失敗した（切断・タイムアウト）ときは、Gmail 側が受理済みの可能性があるので、
// raw の Message-ID で送信済みメッセージを探す（confirmSent）。
func (c *client) Send(ctx context.Context, accountID string, raw []byte, threadID string) (SentMessage, error) {
	msg := &gmailapi.Message{ThreadId: threadID}
	var sent *gmailapi.Message
	err := c.do(ctx, accountID, callSpec{name: "messages.send", units: unitsMessageSend, transfer: true}, func(s *gmailapi.Service) error {
		var err error
		sent, err = s.Users.Messages.Send(userID, msg).Media(bytes.NewReader(raw), uploadOptions...).Context(ctx).Do()
		return err
	})
	if err != nil {
		if resultUnknown(err) {
			return c.confirmSent(ctx, accountID, raw, err)
		}
		return SentMessage{}, err
	}
	return SentMessage{MessageID: sent.Id, ThreadID: sent.ThreadId}, nil
}

// confirmSendTimeout は Send 失敗後に送信済みか確かめる検索の上限（呼び出し側 ctx が切れていても掛ける）。
const confirmSendTimeout = 30 * time.Second

// confirmSent は応答を受け取れなかった Send について、同じ Message-ID のメッセージが Gmail にあるかを確かめる。
// あれば送信成功として返す。無ければ元のエラー cause を返す（未送信として再送できる）。
// 確かめられなければ ErrSendUncertain（送信済みの可能性。再送させない）を返す。
func (c *client) confirmSent(ctx context.Context, accountID string, raw []byte, cause error) (SentMessage, error) {
	id := messageIDOf(raw)
	if id == "" {
		return SentMessage{}, fmt.Errorf("gmail: messages.send: Message-ID が無く送信結果を確認できません: %w", ErrSendUncertain)
	}
	vctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), confirmSendTimeout)
	defer cancel()
	var found *gmailapi.Message
	err := c.do(vctx, accountID, callSpec{name: "messages.list", units: unitsMessageList, retry5xx: true}, func(s *gmailapi.Service) error {
		resp, err := s.Users.Messages.List(userID).Q("rfc822msgid:" + id).MaxResults(1).Context(vctx).Do()
		if err != nil {
			return err
		}
		if len(resp.Messages) > 0 {
			found = resp.Messages[0]
		}
		return nil
	})
	switch {
	case err != nil:
		return SentMessage{}, fmt.Errorf("gmail: messages.send: 送信結果を確認できません: %w", ErrSendUncertain)
	case found != nil:
		c.logger.Warn("応答を受け取れなかった送信は Gmail 側で受理済みでした", slog.String("account_id", accountID))
		return SentMessage{MessageID: found.Id, ThreadID: found.ThreadId}, nil
	default:
		return SentMessage{}, cause
	}
}

// messageIDOf は raw の Message-ID（山括弧なし）を返す。無ければ空。
func messageIDOf(raw []byte) string {
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(m.Header.Get("Message-Id")), "<>")
}

// CreateDraft は下書きを作成する（drafts.create。メディアアップロード経路）。5xx は重複作成を避けるため再試行しない。
func (c *client) CreateDraft(ctx context.Context, accountID string, raw []byte, threadID string) (string, error) {
	draft := &gmailapi.Draft{Message: &gmailapi.Message{ThreadId: threadID}}
	var created *gmailapi.Draft
	err := c.do(ctx, accountID, callSpec{name: "drafts.create", units: unitsDraft, transfer: true}, func(s *gmailapi.Service) error {
		var err error
		created, err = s.Users.Drafts.Create(userID, draft).Media(bytes.NewReader(raw), uploadOptions...).Context(ctx).Do()
		return err
	})
	if err != nil {
		return "", err
	}
	return created.Id, nil
}

// UpdateDraft は下書きの内容を置き換える（drafts.update。メディアアップロード経路）。
func (c *client) UpdateDraft(ctx context.Context, accountID, draftID string, raw []byte) error {
	draft := &gmailapi.Draft{Message: &gmailapi.Message{}}
	return c.do(ctx, accountID, callSpec{name: "drafts.update", units: unitsDraft, retry5xx: true, transfer: true}, func(s *gmailapi.Service) error {
		_, err := s.Users.Drafts.Update(userID, draftID, draft).Media(bytes.NewReader(raw), uploadOptions...).Context(ctx).Do()
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
	bcc := []model.Address{}
	if d.Message.Payload != nil {
		bcc = mimeparse.ParseAddressList(mimeparse.HeaderValue(d.Message.Payload.Headers, "Bcc"))
	}
	return DraftFull{ID: d.Id, ThreadID: d.Message.ThreadId, Message: mf, Bcc: bcc, Attachments: mf.Attachments}, nil
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
