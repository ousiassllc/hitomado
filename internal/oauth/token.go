package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxResponseBytes はトークン／userinfo の応答の読み取り上限。
const maxResponseBytes = 1 << 20

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

type userinfoResponse struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// exchange は認可コードをトークンに交換する（PKCE code_verifier と Client Secret 付き）。
// エラーに応答本文・コード・トークンを含めない。
func (f *flow) exchange(ctx context.Context, cfg ClientConfig, code, verifier, redirectURI string) (tokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	}
	var tok tokenResponse
	if err := f.postForm(ctx, f.opts.TokenURL, form, &tok); err != nil {
		var he *httpError
		if errors.As(err, &he) && he.clientRejected() {
			return tokenResponse{}, fmt.Errorf("oauth: トークン交換に失敗しました: %w", ErrClientRejected)
		}
		return tokenResponse{}, fmt.Errorf("oauth: トークン交換に失敗しました: %w", err)
	}
	if tok.AccessToken == "" {
		return tokenResponse{}, errors.New("oauth: トークン交換の応答にアクセストークンがありません")
	}
	return tok, nil
}

// userinfo はアクセストークンで email・name を取得する。
func (f *flow) userinfo(ctx context.Context, accessToken string) (userinfoResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.opts.UserinfoURL, nil)
	if err != nil {
		return userinfoResponse{}, fmt.Errorf("oauth: userinfo の要求を作れません: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	var info userinfoResponse
	if err := f.do(req, &info); err != nil {
		return userinfoResponse{}, fmt.Errorf("oauth: アドレスの取得に失敗しました: %w", err)
	}
	if strings.TrimSpace(info.Email) == "" {
		return userinfoResponse{}, errors.New("oauth: userinfo の応答にメールアドレスがありません")
	}
	return info, nil
}

// Revoke はトークンを Google 側で失効させる。2xx 以外は error。
func (f *flow) Revoke(ctx context.Context, refreshToken string) error {
	if err := f.postForm(ctx, f.opts.RevokeURL, url.Values{"token": {refreshToken}}, nil); err != nil {
		return fmt.Errorf("oauth: トークンの失効に失敗しました: %w", err)
	}
	return nil
}

func (f *flow) postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("要求を作れません: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return f.do(req, out)
}

// httpError は非 2xx 応答。本文・error_description は機密を含み得るため保持せず、
// ステータスと、固定語彙に一致した OAuth の error コードだけを持つ。
type httpError struct {
	status int
	code   string // "invalid_client"／"unauthorized_client" のいずれか。それ以外は空
}

func (e *httpError) Error() string { return fmt.Sprintf("HTTP %d", e.status) }

// clientRejected は Client ID／Secret が拒否された応答か（RFC 6749 §5.2。401 も invalid_client 相当）。
func (e *httpError) clientRejected() bool {
	return e.code != "" || e.status == http.StatusUnauthorized
}

// clientRejectCode は応答本文から OAuth の error コードを取り出す。固定語彙以外は空を返す。
func clientRejectCode(body io.Reader) string {
	var v struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(body, maxResponseBytes)).Decode(&v); err != nil {
		return ""
	}
	if v.Error == "invalid_client" || v.Error == "unauthorized_client" {
		return v.Error
	}
	return ""
}

// do は要求を実行し、2xx なら out（nil 可）へ JSON をデコードする。
// 非 2xx のエラーは *httpError（本文は機密を含み得るので載せない）。
func (f *flow) do(req *http.Request, out any) error {
	resp, err := f.opts.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &httpError{status: resp.StatusCode, code: clientRejectCode(resp.Body)}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return fmt.Errorf("応答を解釈できません: %w", err)
	}
	return nil
}
