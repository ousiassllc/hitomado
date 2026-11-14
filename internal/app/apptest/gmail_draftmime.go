package apptest

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jhillyerd/enmime/v2"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/textconv"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/gmail/mimeparse"
)

// storeDraft は下書きの MIME を解析して Drafts／DraftRaw に保存する（f.mu を保持して呼ぶ）。
// 宛先・Bcc・件名・本文・添付（attachmentId を新規に振り、内容を Attachments に置く）を GetDraft で返せる形にする。
func (f *FakeGmail) storeDraft(accountID, key, draftID, threadID string, raw []byte) {
	f.seq++
	msgID := fmt.Sprintf("draft-msg-%d", f.seq)
	d := gmail.DraftFull{ID: draftID, ThreadID: threadID, Bcc: []model.Address{}, Attachments: []model.AttachmentInfo{}}
	d.Message = gmail.MessageFull{
		MessageMeta: gmail.MessageMeta{ID: msgID, ThreadID: threadID, LabelIDs: []string{"DRAFT"}, To: []model.Address{}, Cc: []model.Address{}, InternalDate: Epoch.Add(time.Duration(f.seq) * time.Second)},
		References:  []string{}, ReplyTo: []model.Address{}, Attachments: []model.AttachmentInfo{},
		InlineImages: map[string]gmail.InlinePart{},
	}
	if env, err := enmime.ReadEnvelope(bytes.NewReader(raw)); err == nil {
		d.Message.To = mimeparse.ParseAddressList(env.GetHeader("To"))
		d.Message.Cc = mimeparse.ParseAddressList(env.GetHeader("Cc"))
		d.Bcc = mimeparse.ParseAddressList(env.GetHeader("Bcc"))
		d.Message.Subject = env.GetHeader("Subject")
		d.Message.BodyHTML, d.Message.BodyText = env.HTML, env.Text
		d.Message.Snippet = draftSnippet(env.Text, env.HTML)
		for _, a := range env.Attachments {
			f.seq++
			attID := fmt.Sprintf("draft-att-%d", f.seq)
			info := model.AttachmentInfo{ID: attID, Filename: a.FileName, MimeType: a.ContentType, Size: int64(len(a.Content))}
			d.Message.Attachments = append(d.Message.Attachments, info)
			d.Attachments = append(d.Attachments, info)
			f.Attachments[Key(accountID, msgID, attID)] = slices.Clone(a.Content)
		}
	}
	f.Drafts[key] = d
	f.DraftRaw[key] = slices.Clone(raw)
}

// draftSnippet は Gmail の snippet に近い、本文の先頭 100 文字（空白を畳んだプレーンテキスト）。
func draftSnippet(text, html string) string {
	if strings.TrimSpace(text) == "" {
		text = textconv.HTMLToText(html)
	}
	return truncateRunes(strings.Join(strings.Fields(text), " "), 100)
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// dropDraftAttachments は下書きの旧メッセージの添付データを捨てる（f.mu を保持して呼ぶ）。
func (f *FakeGmail) dropDraftAttachments(accountID string, d gmail.DraftFull) {
	for _, a := range d.Attachments {
		delete(f.Attachments, Key(accountID, d.Message.ID, a.ID))
	}
}
