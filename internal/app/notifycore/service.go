// Package notifycore は通知の許可・OS の通知設定・通知のタップで開くスレッドのバインディング（NotifyService）。
// internal/app のファイル数・行数の上限のためサブパッケージにしている（AIService の aicore と同じ）。
package notifycore

import (
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// NotifyService は通知の許可状態・OS の通知設定・通知のタップで開くスレッド（バインディング仕様 §7.4）。
// 許可の管理があるのは Android だけ（Windows は常に unsupported で、画面に項目を出さない）。
type NotifyService struct{ d *deps.Deps }

// NewNotifyService は NotifyService を作る。
func NewNotifyService(d *deps.Deps) *NotifyService { return &NotifyService{d: d} }

// GetNotificationPermission は通知の許可状態を返す（granted／denied／unsupported）。
// denied は、許可されていない・OS の設定で通知がオフ・それらのどちらでも出せない状態。
func (s *NotifyService) GetNotificationPermission() string {
	return string(s.d.BackgroundOrNop().Permission())
}

// RequestNotificationPermission は通知の許可のダイアログを出す。許可済み・永続的に拒否済みでは何も出ない。結果は待たない。
func (s *NotifyService) RequestNotificationPermission() {
	s.d.BackgroundOrNop().RequestPermission()
}

// OpenNotificationSettings は OS の「このアプリの通知設定」を開く。開けなければ UNSUPPORTED。
func (s *NotifyService) OpenNotificationSettings() error {
	if err := s.d.BackgroundOrNop().OpenSettings(); err != nil {
		return apperr.Wrap(apperr.Unsupported, "OS の通知設定を開けませんでした。", err)
	}
	return nil
}

// TakePendingOpenThread はフロントが購読を始めたあとに 1 回呼ぶ。通知のタップで開くよう要求されたスレッドが
// フロントの準備前にあれば返し（なければ null）、以後の要求は notify:open-thread で届く。
func (s *NotifyService) TakePendingOpenThread() *dto.ThreadRef {
	if s.d.OpenRequests == nil {
		return nil
	}
	ref := s.d.OpenRequests.Ready()
	if ref == nil {
		return nil
	}
	return &dto.ThreadRef{AccountID: ref.AccountID, ThreadID: ref.ThreadID}
}
