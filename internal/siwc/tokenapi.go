package siwc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// resource は ChatGPT プラン利用のトークンが対象とする API（認可・交換・更新の resource に必ず付ける）。
const resource = "https://api.openai.com/v1"

// maxBody は OpenAI からの応答本文の読み取り上限。
const maxBody = 1 << 20

// tokenResponse はトークンエンドポイントの成功応答。値はログ・エラーに出さない。
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

// serverError は OpenAI のトークン・失効エンドポイントがエラーを返したこと。メッセージには HTTP ステータスと
// OAuth のエラーコード（invalid_grant など。形式を検査した短い識別子だけ）しか載せない。応答本文のその他は捨てる。
type serverError struct {
	Status int
	Code   string
}

func (e *serverError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("siwc: OpenAI がエラーを返しました（HTTP %d）", e.Status)
	}
	return fmt.Sprintf("siwc: OpenAI がエラーを返しました（HTTP %d, %s）", e.Status, e.Code)
}

var oauthCodeRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// newServerError は応答から OAuth の error コードだけを取り出す（description など他の項目は使わない）。
func newServerError(status int, body []byte) *serverError {
	var v struct {
		Error any `json:"error"`
	}
	e := &serverError{Status: status}
	if json.Unmarshal(body, &v) == nil {
		if s, ok := v.Error.(string); ok && oauthCodeRE.MatchString(s) {
			e.Code = s
		}
	}
	return e
}

// 更新できないトークン（再ログインが必要）を表すエラーコード（公式ドキュメント Errors and recovery）。
var unusableRefreshCodes = map[string]bool{
	"invalid_grant": true, "invalid_refresh_token": true, "token_expired": true,
	"refresh_token_expired": true, "refresh_token_invalidated": true, "refresh_token_reused": true,
}

// postForm は form を POST し、2xx の本文を返す（上限付き）。非 2xx は *serverError（本文は読むが載せない）。
// 通信エラーは ctx のエラーか *url.Error（URL にクエリ・フォームは含まれない）のまま wrap する。
func (m *Manager) postForm(ctx context.Context, endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: 要求を作れません", ErrFailed)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := m.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("siwc: 要求を中断しました: %w", ctx.Err())
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			// url.Error の文字列は URL と下位のエラーだけ（フォームの値は入らない）。
			return nil, fmt.Errorf("siwc: 通信に失敗しました: %w", ue)
		}
		return nil, fmt.Errorf("siwc: 通信に失敗しました: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("siwc: 要求を中断しました: %w", ctx.Err())
		}
		return nil, fmt.Errorf("%w: 応答を読み取れません", ErrFailed)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newServerError(resp.StatusCode, body)
	}
	return body, nil
}

// requestTokens はトークンエンドポイントへ form を送り、トークン応答を返す。
func (m *Manager) requestTokens(ctx context.Context, form url.Values) (tokenResponse, error) {
	endpoint, err := m.tokenEndpoint(ctx)
	if err != nil {
		return tokenResponse{}, err
	}
	body, err := m.postForm(ctx, endpoint, form)
	if err != nil {
		return tokenResponse{}, err
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return tokenResponse{}, fmt.Errorf("%w: トークン応答を解釈できません", ErrInvalidResponse)
	}
	if tr.AccessToken == "" || (tr.TokenType != "" && !strings.EqualFold(tr.TokenType, "Bearer")) {
		return tokenResponse{}, fmt.Errorf("%w: トークン応答が不正です", ErrInvalidResponse)
	}
	return tr, nil
}

// revoke はリフレッシュトークンを失効させる（公式の失効エンドポイント。空の 200 が成功。既に無効なトークンも 200）。
func (m *Manager) revoke(ctx context.Context, clientID, refreshToken string) error {
	endpoint, err := m.revocationEndpoint(ctx)
	if err != nil {
		return err
	}
	_, err = m.postForm(ctx, endpoint, url.Values{
		"token":           {refreshToken},
		"token_type_hint": {"refresh_token"},
		"client_id":       {clientID},
	})
	return err
}
