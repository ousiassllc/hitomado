package siwcfake

import (
	"context"
	"fmt"
	"sync"

	"github.com/ousiassllc/hitomado/internal/siwc"
)

// Account は Fake が持つ ChatGPT アカウント 1 件。
type Account struct {
	ID       string
	Email    string
	Token    string // このアカウントのアクセストークン
	SignedIn bool
	// NoticePending は初めてのログインで立ち、AckPlanNotice で下りる（再ログインでは立たない）。
	NoticePending bool
}

// Fake は siwc.Auth のフェイク。複数のアカウント・使用中のアカウントと、呼ばれた回数を持つ。
type Fake struct {
	mu sync.Mutex

	// Email・Token は、次に新規登録するアカウントのメールアドレスとアクセストークン（空の Email は me@chatgpt.example）。
	Email string
	Token string
	// NextToken は ForceRefresh が使用中のアカウントに設定するトークン（空なら変えない）。
	NextToken  string
	SignInErr  error
	AccessErr  error
	RefreshErr error
	Revoked    bool // SignOut が返す revoked

	accounts []*Account
	active   string
	seq      int

	SignIns, Refreshes, SignOuts, AccessCalls int
	LastSignInOptions                         siwc.SignInOptions
	LastSignOutID                             string
	OnRefresh                                 func() // ForceRefresh の中で呼ぶ（テストが中止などを差し込む）
}

var _ siwc.Auth = (*Fake)(nil)

func (f *Fake) find(id string) *Account {
	for _, a := range f.accounts {
		if a.ID == id {
			return a
		}
	}
	return nil
}

func (f *Fake) status() siwc.Status {
	st := siwc.Status{Accounts: make([]siwc.Account, 0, len(f.accounts))}
	for _, a := range f.accounts {
		active := a.ID == f.active
		st.Accounts = append(st.Accounts, siwc.Account{
			ID: a.ID, Email: a.Email, SignedIn: a.SignedIn, Active: active, PlanNoticePending: a.NoticePending,
		})
		if active {
			st.SignedIn = a.SignedIn
			if a.SignedIn {
				st.Email = a.Email // 実装（Manager.Status）と同じ: ログアウト済みの使用中アカウントのメールは返さない
			}
		}
	}
	return st
}

// SignIn はログイン済みにして使用中にする。AccountID が空なら新しいアカウントを足す（SignInErr があればそれを返す）。
func (f *Fake) SignIn(_ context.Context, opts siwc.SignInOptions) (siwc.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SignIns++
	f.LastSignInOptions = opts
	if f.SignInErr != nil {
		return siwc.Status{}, f.SignInErr
	}
	var a *Account
	if opts.AccountID != "" {
		if a = f.find(opts.AccountID); a == nil {
			return siwc.Status{}, siwc.ErrUnknownAccount
		}
	} else {
		f.seq++
		email := f.Email
		if email == "" {
			email = "me@chatgpt.example"
		}
		a = &Account{ID: fmt.Sprintf("oaiapp_fake%d", f.seq), Email: email, NoticePending: true}
		f.accounts = append(f.accounts, a)
	}
	a.SignedIn, a.Token = true, f.Token
	f.active = a.ID
	return f.status(), nil
}

// CancelSignIn は何もしない。
func (f *Fake) CancelSignIn() {}

// Status は現在の状態を返す。
func (f *Fake) Status(context.Context) (siwc.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status(), nil
}

// SelectAccount は使用中のアカウントを切り替える（ログイン中のアカウントだけ）。
func (f *Fake) SelectAccount(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.find(id)
	switch {
	case a == nil:
		return siwc.ErrUnknownAccount
	case !a.SignedIn:
		return siwc.ErrNotSignedIn
	}
	f.active = id
	return nil
}

// activeAccount は使用中でログイン中のアカウント（無ければ nil）。
func (f *Fake) activeAccount() *Account {
	if a := f.find(f.active); a != nil && a.SignedIn {
		return a
	}
	return nil
}

// AccessToken は使用中のアカウントの Token を返す（未ログインは ErrNotSignedIn）。
func (f *Fake) AccessToken(context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.AccessCalls++
	if f.AccessErr != nil {
		return "", f.AccessErr
	}
	a := f.activeAccount()
	if a == nil {
		return "", siwc.ErrNotSignedIn
	}
	return a.Token, nil
}

// ForceRefresh は NextToken（無ければ今の Token）を返し、以降の AccessToken もそれにする。
func (f *Fake) ForceRefresh(context.Context) (string, error) {
	if f.OnRefresh != nil {
		f.OnRefresh() // ロックの外で呼ぶ
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Refreshes++
	if f.RefreshErr != nil {
		return "", f.RefreshErr
	}
	a := f.activeAccount()
	if a == nil {
		return "", siwc.ErrNotSignedIn
	}
	if f.NextToken != "" {
		a.Token = f.NextToken
	}
	return a.Token, nil
}

// SignOut は指定したアカウントだけログアウト状態にする。
func (f *Fake) SignOut(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SignOuts++
	f.LastSignOutID = id
	a := f.find(id)
	if a == nil {
		return false, siwc.ErrUnknownAccount
	}
	a.SignedIn, a.Token = false, ""
	return f.Revoked, nil
}

// AckPlanNotice は確認を済みにする。
func (f *Fake) AckPlanNotice(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.find(id)
	if a == nil {
		return siwc.ErrUnknownAccount
	}
	a.NoticePending = false
	return nil
}

// RemoveAccount はログアウト済みのアカウントを忘れる。
func (f *Fake) RemoveAccount(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.find(id)
	switch {
	case a == nil:
		return siwc.ErrUnknownAccount
	case a.SignedIn:
		return siwc.ErrAccountInUse
	}
	for i, x := range f.accounts {
		if x == a {
			f.accounts = append(f.accounts[:i], f.accounts[i+1:]...)
			break
		}
	}
	if f.active == id {
		f.active = ""
	}
	return nil
}
