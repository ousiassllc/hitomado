package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// maxResponseBytes は応答本文の読み取り上限。
const maxResponseBytes = 4 << 20

// client は各プロバイダ共通の HTTP 呼び出し部。
type client struct {
	hc     *http.Client
	base   string
	model  string
	apiKey string
}

// send は要求を送り、応答を返す（本文のクローズは呼び出し側）。payload が非 nil なら JSON 本文として送る。
// headers は既定ヘッダ（Accept: application/json）を上書きする。
// エラーにはリクエスト本文を含めない。通信エラーは net.Error のまま wrap（ctx 終了時は ctx のエラー）。
func (c client) send(ctx context.Context, method, endpoint string, headers map[string]string, payload []byte) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("%w: 要求を作れません", ErrFailed)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ai: 要求を中断しました: %w", ctx.Err())
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // url.Error は URL を含むため、内側のエラー（net.Error のまま）だけ返す
		}
		return nil, fmt.Errorf("ai: 通信に失敗しました: %w", err)
	}
	return resp, nil
}

// isSuccess は 2xx か。
func isSuccess(status int) bool { return status >= 200 && status <= 299 }

// drain は応答本文を上限つきで読み捨てる（接続の再利用用）。
func drain(r io.Reader) { _, _ = io.Copy(io.Discard, io.LimitReader(r, maxResponseBytes)) }

// statusError は非 2xx の HTTP ステータスを分類したエラーにする。メッセージにはステータスだけを含める。
//   - 401／403 → ErrUnauthorized
//   - 429 → ErrRateLimited
//   - 400／422 → ErrRejected（ErrFailed でもある。エフォートなど、要求の内容をプロバイダが受け付けなかった）
//   - それ以外 → ErrFailed
func statusError(status int) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: HTTP %d", ErrUnauthorized, status)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: HTTP %d", ErrRateLimited, status)
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: HTTP %d", ErrRejected, status)
	default:
		return fmt.Errorf("%w: HTTP %d", ErrFailed, status)
	}
}

// decodeJSON は 2xx 応答の本文を out にデコードする。不正 JSON は ErrFailed（本文は含めない）。
func decodeJSON(ctx context.Context, body io.Reader, out any) error {
	if err := json.NewDecoder(io.LimitReader(body, maxResponseBytes)).Decode(out); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("ai: 要求を中断しました: %w", ctx.Err())
		}
		return fmt.Errorf("%w: 応答を解釈できません", ErrFailed)
	}
	return nil
}

// postJSON は body を JSON で POST し、2xx の応答を out にデコードする。
// エラーにはレスポンス本文もリクエスト本文も含めない（機密を含み得る）。分類は statusError に従う。
func (c client) postJSON(ctx context.Context, endpoint string, headers map[string]string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%w: 要求を組み立てられません", ErrFailed)
	}
	resp, err := c.send(ctx, http.MethodPost, endpoint, headers, payload)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if !isSuccess(resp.StatusCode) {
		drain(resp.Body)
		return statusError(resp.StatusCode)
	}
	return decodeJSON(ctx, resp.Body, out)
}

// getJSON は GET し、2xx の応答を out にデコードする（モデル一覧用）。
// 分類は statusError に加え、404／405／501 → ErrModelsUnsupported。本文・URL・キーはエラーに含めない。
func (c client) getJSON(ctx context.Context, endpoint string, headers map[string]string, out any) error {
	resp, err := c.send(ctx, http.MethodGet, endpoint, headers, nil)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if !isSuccess(resp.StatusCode) {
		drain(resp.Body)
		switch resp.StatusCode {
		case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
			return fmt.Errorf("%w: HTTP %d", ErrModelsUnsupported, resp.StatusCode)
		}
		return statusError(resp.StatusCode)
	}
	return decodeJSON(ctx, resp.Body, out)
}
