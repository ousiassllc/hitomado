package apptest

import (
	"sync"

	"github.com/ousiassllc/hitomado/internal/notify"
)

// FakeBackground は notify.Background のフェイク。呼び出しを記録する。
type FakeBackground struct {
	mu sync.Mutex

	Schedules  []bool   // ScheduleSync の引数（呼び出し順）
	Forgotten  []string // ForgetAccount の引数
	Requests   int      // RequestPermission の呼び出し回数
	State      notify.Permission
	SettingsOK bool  // OpenSettings が呼ばれたか
	OpenErr    error // OpenSettings が返すエラー
}

var _ notify.Background = (*FakeBackground)(nil)

// ScheduleSync は呼び出しを記録する。
func (f *FakeBackground) ScheduleSync(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Schedules = append(f.Schedules, enabled)
}

// ForgetAccount は呼び出しを記録する。
func (f *FakeBackground) ForgetAccount(accountID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Forgotten = append(f.Forgotten, accountID)
}

// Permission は State（未設定は granted）を返す。
func (f *FakeBackground) Permission() notify.Permission {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.State == "" {
		return notify.PermissionGranted
	}
	return f.State
}

// RequestPermission は呼び出しを記録する。
func (f *FakeBackground) RequestPermission() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Requests++
}

// OpenSettings は呼び出しを記録して OpenErr を返す。
func (f *FakeBackground) OpenSettings() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SettingsOK = true
	return f.OpenErr
}

// Snapshot は記録の複製を返す（並行利用中に読むため）。
func (f *FakeBackground) Snapshot() (schedules []bool, forgotten []string, requests int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]bool(nil), f.Schedules...), append([]string(nil), f.Forgotten...), f.Requests
}
