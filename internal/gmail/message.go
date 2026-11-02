package gmail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail/mimeparse"
)

// errBadBody は本文データが base64url として壊れているときのエラー。
var errBadBody = fmt.Errorf("gmail: 本文のデコードに失敗: %w", ErrGeneric)

// metaFromMessage は format=metadata／full のメッセージからヘッダ由来のメタデータを作る。
func metaFromMessage(m *gmailapi.Message) MessageMeta {
	meta := MessageMeta{
		ID:           m.Id,
		ThreadID:     m.ThreadId,
		InternalDate: time.UnixMilli(m.InternalDate),
		LabelIDs:     append([]string{}, m.LabelIds...),
		Snippet:      m.Snippet,
		To:           []model.Address{},
		Cc:           []model.Address{},
	}
	if m.Payload == nil {
		return meta
	}
	h := m.Payload.Headers
	meta.From = mimeparse.ParseFrom(mimeparse.HeaderValue(h, "From"))
	meta.To = mimeparse.ParseAddressList(mimeparse.HeaderValue(h, "To"))
	meta.Cc = mimeparse.ParseAddressList(mimeparse.HeaderValue(h, "Cc"))
	meta.Subject = mimeparse.DecodeHeader(mimeparse.HeaderValue(h, "Subject"))
	meta.HasAttachment = mimeparse.HasAttachment(m.Payload)
	return meta
}

// messageFull は format=full のメッセージを MessageFull にする。
// 本文が別取得（attachmentId）のとき、および本文から参照されるインライン画像は attachments.get で取得する。
func (c *client) messageFull(ctx context.Context, accountID string, m *gmailapi.Message) (MessageFull, error) {
	mf := MessageFull{
		MessageMeta:  metaFromMessage(m),
		References:   []string{},
		ReplyTo:      []model.Address{},
		Attachments:  []model.AttachmentInfo{},
		InlineImages: map[string]InlinePart{},
	}
	if m.Payload == nil {
		return mf, nil
	}
	h := m.Payload.Headers
	mf.MessageID = strings.TrimSpace(mimeparse.HeaderValue(h, "Message-ID"))
	mf.References = mimeparse.SplitFields(mimeparse.HeaderValue(h, "References"))
	mf.ReplyTo = mimeparse.ParseAddressList(mimeparse.HeaderValue(h, "Reply-To"))

	parts := mimeparse.Walk(m.Payload)
	var err error
	if mf.BodyHTML, err = c.joinSegments(ctx, accountID, m.Id, parts.HTML); err != nil {
		return MessageFull{}, err
	}
	if mf.BodyText, err = c.joinSegments(ctx, accountID, m.Id, parts.Text); err != nil {
		return MessageFull{}, err
	}
	mf.Attachments = append(mf.Attachments, parts.Attachments...)
	if err := c.collectInlines(ctx, accountID, m.Id, parts.Inlines, &mf); err != nil {
		return MessageFull{}, err
	}
	mf.HasAttachment = len(mf.Attachments) > 0
	return mf, nil
}

// joinSegments は本文パートをデコードし、文字コードを UTF-8 にして連結する。
func (c *client) joinSegments(ctx context.Context, accountID, messageID string, segs []mimeparse.Segment) (string, error) {
	pieces := make([]string, 0, len(segs))
	for _, s := range segs {
		var b []byte
		var err error
		if s.Data == "" && s.AttID != "" {
			if b, err = c.GetAttachment(ctx, accountID, messageID, s.AttID); err != nil {
				return "", err
			}
		} else if b, err = mimeparse.DecodeBase64URL(s.Data); err != nil {
			return "", errBadBody
		}
		pieces = append(pieces, mimeparse.DecodeText(b, s.Charset))
	}
	return strings.Join(pieces, "\n"), nil
}

// collectInlines は本文（HTML）が cid: で参照するインライン画像を取得して mf.InlineImages に入れる。
// 参照されない画像と Content-Disposition: attachment の画像は添付一覧に回す。
func (c *client) collectInlines(ctx context.Context, accountID, messageID string, inlines []mimeparse.InlineRef, mf *MessageFull) error {
	lowerHTML := strings.ToLower(mf.BodyHTML)
	for _, in := range inlines {
		referenced := strings.Contains(lowerHTML, "cid:"+strings.ToLower(in.CID))
		if (in.AsAttach || !referenced) && in.AttID != "" {
			mf.Attachments = append(mf.Attachments, in.Info)
		}
		if !referenced || in.Size > mimeparse.MaxInlineImageBytes {
			continue
		}
		var b []byte
		var err error
		if in.Data != "" {
			if b, err = mimeparse.DecodeBase64URL(in.Data); err != nil {
				continue // 壊れた画像は表示しないだけで、スレッド全体は失敗にしない
			}
		} else if in.AttID != "" {
			b, err = c.GetAttachment(ctx, accountID, messageID, in.AttID)
		} else {
			continue
		}
		if errors.Is(err, ErrAttachmentGone) {
			continue // 取得できない画像は表示しないだけで、スレッド全体は失敗にしない
		}
		if err != nil {
			return err
		}
		if len(b) > mimeparse.MaxInlineImageBytes {
			continue
		}
		mf.InlineImages[in.CID] = InlinePart{MimeType: in.MimeType, Data: b}
	}
	return nil
}
