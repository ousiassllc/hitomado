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

// do は要求を実行し、2xx なら out（nil 可）へ JSON をデコードする。
// 非 2xx のエラーはステータスコードのみ（本文は機密を含み得るので載せない）。
func (f *flow) do(req *http.Request, out any) error {
	resp, err := f.opts.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return fmt.Errorf("HTTP %d", resp.StatusCode)
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
