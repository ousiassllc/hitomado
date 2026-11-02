package gmail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/ousiassllc/hitomado/internal/gmail/mimebuild"
)

const (
	// confirmSendTimeout は Send 失敗後に送信済みか確かめる処理全体の上限（呼び出し側 ctx が切れていても掛ける）。
	confirmSendTimeout = 30 * time.Second
	// confirmSendMargin は検索範囲を送信開始より前へ広げる余裕（クライアントと Gmail の時計のずれ用）。
	confirmSendMargin = 10 * time.Minute
	// confirmSendCandidates は確認する送信済みメッセージの最大件数（新しい順）。
	confirmSendCandidates = 20
)

// confirmSent は応答を受け取れなかった Send について、同じ送信のメッセージが Gmail の送信済みにあるかを確かめる。
// あれば送信成功として返す。
//
// 見つからなくても「未送信」とは決めつけず ErrSendUncertain（送信済みの可能性。再送させない）を返す。
// Gmail API は送信時に Message-ID を付け替えることがあり、独自ヘッダが残る保証も、検索インデックスが
// 送信直後に追い付いている保証も無いため、「見つからない」は「送られていない」の根拠にならない。
func (c *client) confirmSent(ctx context.Context, accountID string, raw []byte, started time.Time) (SentMessage, error) {
	uncertain := func(why string) (SentMessage, error) {
		return SentMessage{}, fmt.Errorf("gmail: messages.send: 送信結果を確認できません（%s）: %w", why, ErrSendUncertain)
	}
	sendID, messageID := rawHeader(raw, mimebuild.SendIDHeader), rawHeader(raw, "Message-ID")
	if sendID == "" && messageID == "" {
		return uncertain("送信識別子が無い")
	}
	vctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), confirmSendTimeout)
	defer cancel()

	q := "in:sent after:" + strconv.FormatInt(started.Add(-confirmSendMargin).Unix(), 10)
	var candidates []*gmailapi.Message
	err := c.do(vctx, accountID, callSpec{name: "messages.list", units: unitsMessageList, retry5xx: true}, func(s *gmailapi.Service) error {
		resp, err := s.Users.Messages.List(userID).Q(q).MaxResults(confirmSendCandidates).Context(vctx).Do()
		if err == nil {
			candidates = resp.Messages
		}
		return err
	})
	if err != nil {
		return uncertain("送信済みを検索できない")
	}
	for _, cand := range candidates {
		var m *gmailapi.Message
		err := c.do(vctx, accountID, callSpec{name: "messages.get", units: unitsMessageGet, retry5xx: true}, func(s *gmailapi.Service) error {
			var err error
			m, err = s.Users.Messages.Get(userID, cand.Id).Format("metadata").MetadataHeaders(mimebuild.SendIDHeader, "Message-ID").Context(vctx).Do()
			return err
		})
		if errors.Is(err, ErrNotFound) {
			continue // 確認中に消えたメッセージ
		}
		if err != nil {
			return uncertain("送信済みの内容を取得できない")
		}
		if sameSend(m, sendID, messageID) {
			c.logger.Warn("応答を受け取れなかった送信は Gmail 側で受理済みでした", slog.String("account_id", accountID))
			return SentMessage{MessageID: m.Id, ThreadID: m.ThreadId}, nil
		}
	}
	return uncertain("送信済みに見当たらない")
}

// sameSend は m が raw と同じ送信か。独自ヘッダ（送信識別子）か、書き換えられていなければ Message-ID が一致すること。
func sameSend(m *gmailapi.Message, sendID, messageID string) bool {
	if m.Payload == nil {
		return false
	}
	for _, h := range m.Payload.Headers {
		v := strings.TrimSpace(h.Value)
		switch {
		case sendID != "" && strings.EqualFold(h.Name, mimebuild.SendIDHeader) && v == sendID:
			return true
		case messageID != "" && strings.EqualFold(h.Name, "Message-ID") && v == messageID:
			return true
		}
	}
	return false
}

// rawHeader は raw のヘッダ値（前後の空白を除く）を返す。無ければ空。
func rawHeader(raw []byte, name string) string {
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(m.Header.Get(name))
}
