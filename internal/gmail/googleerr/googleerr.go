// Package googleerr は Google API のエラー応答（googleapi.Error）から、ログ・エラー文言に載せてよい情報だけを取り出す。
// 応答本文は信用せず、大きい・JSON でない場合は先頭の一部だけを使う。値は伏せ字処理・文字種の制限を済ませて返す。
package googleerr

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"google.golang.org/api/googleapi"

	"github.com/ousiassllc/hitomado/internal/logging"
)

// 取り出す量の上限。
const (
	MaxParseBody   = 64 << 10 // これを超える本文は JSON として解析しない（googleapi.Error の解析済みフィールドだけを使う）
	maxScrubInput  = 4 << 10  // メッセージのうち伏せ字処理にかける先頭のバイト数
	MaxMessageRune = 300      // エラー文言・ログに載せるメッセージの最大文字数
	maxReasons     = 8        // 載せる reason の最大数
	maxTokenLen    = 64       // reason・status の最大長
	maxMetaValue   = 200      // メタデータの値の最大文字数
)

// 載せてよい details[].metadata のキー（ErrorInfo の metadata は任意の値が入り得るため、秘密でないと分かっているものだけ）。
var metadataAllowlist = map[string]bool{"service": true, "consumer": true, "containerInfo": true, "activationUrl": true}

// safeToken は reason・status として載せてよい文字種（ログ注入・秘密の混入を避ける）。
var safeToken = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// Info は Google のエラー応答から取り出した情報。
type Info struct {
	Status   int               // HTTP ステータス
	State    string            // error.status（PERMISSION_DENIED など。無ければ空）
	Reasons  []string          // errors[].reason と details[].reason。重複なし・文字種を制限済み
	Message  string            // error.message（改行なし・長さ制限・伏せ字処理済み）
	Metadata map[string]string // details[].metadata のうち秘密でないもの
}

// errorReply は Google のエラー応答（{"error": {...}}）の取り出す部分。
type errorReply struct {
	Error *struct {
		Message string `json:"message"`
		Status  string `json:"status"`
		Errors  []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
		Details []struct {
			Reason   string         `json:"reason"`
			Metadata map[string]any `json:"metadata"`
		} `json:"details"`
	} `json:"error"`
}

// Parse は googleapi.Error から表示・ログに載せてよい情報だけを取り出す。
func Parse(ge *googleapi.Error) Info {
	info := Info{Status: ge.Code}
	reasons := newReasonSet()
	message := ge.Message
	for _, item := range ge.Errors {
		reasons.add(item.Reason)
	}

	var reply errorReply
	if len(ge.Body) > 0 && len(ge.Body) <= MaxParseBody && json.Unmarshal([]byte(ge.Body), &reply) == nil && reply.Error != nil {
		body := reply.Error
		if message == "" {
			message = body.Message
		}
		if safeToken.MatchString(body.Status) && len(body.Status) <= maxTokenLen {
			info.State = body.Status
		}
		for _, item := range body.Errors {
			reasons.add(item.Reason)
		}
		for _, d := range body.Details {
			reasons.add(d.Reason)
			for k, v := range d.Metadata {
				s, ok := v.(string)
				if !ok || !metadataAllowlist[k] {
					continue
				}
				if info.Metadata == nil {
					info.Metadata = map[string]string{}
				}
				info.Metadata[k] = sanitizeText(s, maxMetaValue)
			}
		}
	} else if message == "" && len(ge.Errors) == 0 {
		// JSON でない応答（プロキシの HTML など）。先頭の一部だけを手がかりとして載せる。
		message = ge.Body
	}
	info.Reasons = reasons.list
	info.Message = sanitizeText(message, MaxMessageRune)
	return info
}

// reasonSet は文字種を制限し、重複と個数を抑えた reason の集まり。
type reasonSet struct {
	list []string
	seen map[string]bool
}

func newReasonSet() *reasonSet { return &reasonSet{seen: map[string]bool{}} }

func (s *reasonSet) add(r string) {
	if r == "" || len(r) > maxTokenLen || !safeToken.MatchString(r) || s.seen[r] || len(s.list) >= maxReasons {
		return
	}
	s.seen[r] = true
	s.list = append(s.list, r)
}

// sanitizeText は Google が返した文字列を、ログ・エラー文言に載せられる形にする。
// 先頭 maxScrubInput バイトだけを対象に、制御文字（改行を含む）を空白にして連続する空白を詰め、
// 秘密・メールアドレスを伏せてから maxRunes 文字に切り詰める（伏せたあとに切るので、切断で秘密の一部が残ることはない）。
func sanitizeText(s string, maxRunes int) string {
	if len(s) > maxScrubInput {
		s = s[:maxScrubInput]
	}
	s = strings.ToValidUTF8(s, "")
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }), " ")
	s = logging.Scrub(s)
	if utf8.RuneCountInString(s) > maxRunes {
		s = string([]rune(s)[:maxRunes]) + "…"
	}
	return s
}
