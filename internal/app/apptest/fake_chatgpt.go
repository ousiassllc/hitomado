package apptest

import (
	"context"
	"sync"

	"github.com/ousiassllc/hitomado/internal/siwc"
)

// FakeChatGPT は siwc.Auth のフェイク。ログイン状態と、呼ばれた回数を持つ。
type FakeChatGPT struct {
	mu sync.Mutex

	SignedIn   bool
	Email      string
	Token      string // AccessToken が返すトークン
	NextToken  string // ForceRefresh が返すトークン（空なら Token）
	SignInErr  error
	AccessErr  error
	RefreshErr error
	Revoked    bool // SignOut が返す revoked

	SignIns, Refreshes, SignOuts, AccessCalls int
	LastSignInOptions                         siwc.SignInOptions
	OnRefresh                                 func() // ForceRefresh の中で呼ぶ（テストが中止などを差し込む）
}

var _ siwc.Auth = (*FakeChatGPT)(nil)

// SignIn はログイン済みにする（SignInErr があればそれを返す）。
func (f *FakeChatGPT) SignIn(_ context.Context, opts siwc.SignInOptions) (siwc.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SignIns++
	f.LastSignInOptions = opts
	if f.SignInErr != nil {
		return siwc.Status{}, f.SignInErr
	}
	f.SignedIn = true
	if f.Email == "" {
		f.Email = "me@chatgpt.example"
	}
	return siwc.Status{SignedIn: true, Email: f.Email}, nil
}

// CancelSignIn は何もしない。
func (f *FakeChatGPT) CancelSignIn() {}

// Status は現在の状態を返す。
func (f *FakeChatGPT) Status(context.Context) (siwc.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return siwc.Status{SignedIn: f.SignedIn, Email: f.Email}, nil
}

// AccessToken は Token を返す（未ログインは ErrNotSignedIn）。
func (f *FakeChatGPT) AccessToken(context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.AccessCalls++
	if f.AccessErr != nil {
		return "", f.AccessErr
	}
	if !f.SignedIn {
		return "", siwc.ErrNotSignedIn
	}
	return f.Token, nil
}

// ForceRefresh は NextToken（無ければ Token）を返し、以降の AccessToken もそれにする。
func (f *FakeChatGPT) ForceRefresh(context.Context) (string, error) {
	if f.OnRefresh != nil {
		f.OnRefresh() // ロックの外で呼ぶ
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Refreshes++
	if f.RefreshErr != nil {
		return "", f.RefreshErr
	}
	if !f.SignedIn {
		return "", siwc.ErrNotSignedIn
	}
	if f.NextToken != "" {
		f.Token = f.NextToken
	}
	return f.Token, nil
}

// SignOut はログアウト状態にする。
func (f *FakeChatGPT) SignOut(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SignOuts++
	f.SignedIn = false
	return f.Revoked, nil
}
