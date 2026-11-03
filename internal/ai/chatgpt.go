package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// ChatGPT（Sign in with ChatGPT）連携のエラーコード。エラーには、サーバー応答の文字列ではなく
// ここで定義した固定の文字列だけを載せる。
const (
	codeUsageLimit   = "subscription_sharing_usage_limit_exceeded"
	codeNotEligible  = "subscription_sharing_user_not_eligible"
	codeInvalidUser  = "subscription_sharing_invalid_user"
	codeRateLimitHit = "rate_limit_exceeded"
)

const (
	// maxErrorBodyBytes はエラー応答から error.code を探すために読む本文の上限。
	maxErrorBodyBytes = 16 << 10
	// maxStreamBytes は SSE ストリーム全体の読み取り上限。
	maxStreamBytes = 4 << 20
	// maxStreamLineBytes は SSE の 1 行（response.completed は応答全体を含む）の上限。
	maxStreamLineBytes = 1 << 20
)

// errorCodePattern は error.code として受け入れる形式。
var errorCodePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

// chatGPT は OpenAI Responses API（Sign in with ChatGPT の OAuth アクセストークン）の実装。
type chatGPT struct{ client }

// responsesRequest は Responses API の要求。ここに無いパラメータ（temperature・max_output_tokens など）は
// Sign in with ChatGPT では拒否されるため送らない。
type responsesRequest struct {
	Model        string         `json:"model"`
	Instructions string         `json:"instructions"`
	Input        []responseItem `json:"input"`
	Store        bool           `json:"store"`
	Stream       bool           `json:"stream"`
}

type responseItem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// lenientString は文字列でない値を空文字として読む JSON 文字列（サーバー側の型揺れで全体が失敗しないように）。
type lenientString string

func (s *lenientString) UnmarshalJSON(b []byte) error {
	var v string
	if json.Unmarshal(b, &v) == nil {
		*s = lenientString(v)
	}
	return nil
}

// apiError は応答中の error オブジェクト。code だけを読む（message・detail は読まない）。
// オブジェクト以外（文字列など）は空として扱う。
type apiError struct{ Code string }

func (e *apiError) UnmarshalJSON(b []byte) error {
	var v struct {
		Code lenientString `json:"code"`
	}
	if json.Unmarshal(b, &v) == nil {
		e.Code = string(v.Code)
	}
	return nil
}

// responsesEvent は SSE の data（JSON）のうち使う項目。
type responsesEvent struct {
	Type     string        `json:"type"`
	Delta    lenientString `json:"delta"`
	Code     lenientString `json:"code"`  // type=error のとき
	Error    apiError      `json:"error"` // type=error のとき（code がトップレベルに無い形式）
	Response struct {
		Error apiError `json:"error"` // type=response.failed のとき
	} `json:"response"`
}

// GenerateReply は Provider の実装。ストリームを response.completed まで読み、delta を連結して返す。
func (g *chatGPT) GenerateReply(ctx context.Context, in ReplyInput) (string, error) {
	payload, err := json.Marshal(responsesRequest{
		Model:        g.model,
		Instructions: systemPrompt(in),
		Input:        []responseItem{{Role: "user", Content: userPrompt(in)}},
		Store:        false,
		Stream:       true,
	})
	if err != nil {
		return "", fmt.Errorf("%w: 要求を組み立てられません", ErrFailed)
	}
	headers := map[string]string{"Authorization": "Bearer " + g.apiKey, "Accept": "text/event-stream"}
	resp, err := g.send(ctx, http.MethodPost, g.base+"/responses", headers, payload)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if !isSuccess(resp.StatusCode) {
		code := readErrorCode(resp.Body)
		drain(resp.Body)
		return "", chatGPTStatusError(resp.StatusCode, code)
	}
	text, err := readResponseStream(ctx, resp.Body)
	if err != nil {
		return "", err
	}
	return nonEmpty(text)
}

// readErrorCode はエラー応答の先頭だけを読み、error.code（^[a-z0-9_]+$ に合うものだけ）を返す。
// 形式が違う・コードが無い（{"detail":"..."} など）場合は空文字。
func readErrorCode(body io.Reader) string {
	raw, _ := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes))
	var v struct {
		Error apiError `json:"error"`
	}
	if json.Unmarshal(raw, &v) != nil || !errorCodePattern.MatchString(v.Error.Code) {
		return ""
	}
	return v.Error.Code
}

// codeError は既知のエラーコードを分類したエラーにする。未知のコードは nil（コードは載せない）。
func codeError(code string) error {
	switch code {
	case codeUsageLimit:
		return fmt.Errorf("%w: %s", ErrUsageLimit, codeUsageLimit)
	case codeNotEligible:
		return fmt.Errorf("%w: %s", ErrNotEligible, codeNotEligible)
	case codeInvalidUser:
		return fmt.Errorf("%w: %s", ErrUnauthorized, codeInvalidUser)
	case codeRateLimitHit:
		return fmt.Errorf("%w: %s", ErrRateLimited, codeRateLimitHit)
	}
	return nil
}

// chatGPTStatusError は非 2xx の応答をコードとステータスで分類する。
//   - 401 → ErrUnauthorized
//   - 既知コード（利用上限・対象外・無効ユーザー）→ 対応するエラー
//   - 403 → ErrUnauthorized、429 → ErrRateLimited、それ以外 → ErrFailed
func chatGPTStatusError(status int, code string) error {
	if status != http.StatusUnauthorized {
		if err := codeError(code); err != nil {
			return fmt.Errorf("%w (HTTP %d)", err, status)
		}
	}
	return statusError(status)
}

// readResponseStream は SSE ストリームを読む。成功は response.completed を受けたときだけ。
// response.failed／error はコードで分類、response.incomplete・途中切断・不正な行は ErrFailed。
func readResponseStream(ctx context.Context, body io.Reader) (string, error) {
	sc := bufio.NewScanner(io.LimitReader(body, maxStreamBytes))
	sc.Buffer(make([]byte, 0, 64<<10), maxStreamLineBytes)

	var (
		text strings.Builder
		data []string // 現在のイベントの data 行
	)
	// dispatch は 1 イベント分を処理する。done が true ならストリームの読み取りを終える。
	dispatch := func() (done bool, err error) {
		if len(data) == 0 {
			return false, nil
		}
		payload := strings.Join(data, "\n")
		data = data[:0]
		if payload == "[DONE]" {
			return false, nil
		}
		var ev responsesEvent
		if json.Unmarshal([]byte(payload), &ev) != nil {
			return false, fmt.Errorf("%w: ストリームを解釈できません", ErrFailed)
		}
		switch ev.Type {
		case "response.output_text.delta":
			text.WriteString(string(ev.Delta))
		case "response.completed":
			return true, nil
		case "response.failed":
			return false, streamFailure(ev.Response.Error.Code)
		case "response.incomplete":
			return false, fmt.Errorf("%w: 応答が途中で終了しました", ErrFailed)
		case "error":
			code := string(ev.Code)
			if code == "" {
				code = ev.Error.Code
			}
			return false, streamFailure(code)
		}
		return false, nil
	}

	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if done, err := dispatch(); err != nil {
				return "", err
			} else if done {
				return text.String(), nil
			}
			continue
		}
		if v, ok := strings.CutPrefix(line, "data:"); ok {
			data = append(data, strings.TrimPrefix(v, " "))
		}
	}
	if err := sc.Err(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("ai: 要求を中断しました: %w", ctx.Err())
		}
		return "", fmt.Errorf("%w: ストリームを読み取れません", ErrFailed)
	}
	// 末尾に空行が無いまま終わったイベント（response.completed など）も処理する。
	if done, err := dispatch(); err != nil {
		return "", err
	} else if done {
		return text.String(), nil
	}
	return "", fmt.Errorf("%w: ストリームが完了せずに終了しました", ErrFailed)
}

// streamFailure はストリーム中の失敗（response.failed／error）をコードで分類する。
func streamFailure(code string) error {
	if !errorCodePattern.MatchString(code) {
		code = ""
	}
	if err := codeError(code); err != nil {
		return err
	}
	return fmt.Errorf("%w: 応答の生成に失敗しました", ErrFailed)
}
