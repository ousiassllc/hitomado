package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// maxResponseBytes は応答本文の読み取り上限。
const maxResponseBytes = 4 << 20

// client は両プロバイダ共通の HTTP 呼び出し部。
type client struct {
	hc     *http.Client
	base   string
	model  string
	apiKey string
}

// postJSON は body を JSON で POST し、2xx の応答を out にデコードする。
// エラーにはレスポンス本文もリクエスト本文も含めない（機密を含み得る）。
//   - 401／403 → ErrUnauthorized
//   - それ以外の非 2xx・不正 JSON → ErrFailed
//   - 通信エラーは net.Error のまま wrap（ctx 終了時は ctx のエラー）
func (c client) postJSON(ctx context.Context, endpoint string, headers map[string]string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%w: 要求を組み立てられません", ErrFailed)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%w: 要求を作れません", ErrFailed)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("ai: 要求を中断しました: %w", ctx.Err())
		}
		return fmt.Errorf("ai: 通信に失敗しました: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return fmt.Errorf("%w: HTTP %d", ErrUnauthorized, resp.StatusCode)
	case resp.StatusCode == http.StatusTooManyRequests:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return fmt.Errorf("%w: HTTP %d", ErrRateLimited, resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return fmt.Errorf("%w: HTTP %d", ErrFailed, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("ai: 要求を中断しました: %w", ctx.Err())
		}
		return fmt.Errorf("%w: 応答を解釈できません", ErrFailed)
	}
	return nil
}
