package main

import (
	"context"
	"sync/atomic"

	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/notify"
	"github.com/ousiassllc/hitomado/internal/platform"
)

// Wails のアプリ・ウィンドウ・トレイは、サービス（app.Services）を渡して application.New した後でないと作れない。
// 一方サービスはイベント発行・ダイアログ・通知を必要とするため、後から接続する遅延束縛のアダプタを挟む。

type lateEmitter struct {
	next atomic.Pointer[events.Emitter]
}

func (l *lateEmitter) set(e events.Emitter) { l.next.Store(&e) }

func (l *lateEmitter) Emit(name string, payload any) {
	if e := l.next.Load(); e != nil {
		(*e).Emit(name, payload)
	}
}

type lateFiles struct {
	f atomic.Pointer[platform.Files]
}

func (l *lateFiles) SaveFile(ctx context.Context, name string) (string, bool, error) {
	if f := l.f.Load(); f != nil {
		return f.SaveFile(ctx, name)
	}
	return "", false, errNotReady
}

type lateBrowser struct {
	b atomic.Pointer[platform.Browser]
}

func (l *lateBrowser) Open(url string) error {
	if b := l.b.Load(); b != nil {
		return b.Open(url)
	}
	return errNotReady
}

type lateNotifier struct {
	n atomic.Pointer[platform.Notifier]
}

func (l *lateNotifier) NewMail(ctx context.Context, m notify.NewMailNotice) error {
	if n := l.n.Load(); n != nil {
		return n.NewMail(ctx, m)
	}
	return nil
}

func (l *lateNotifier) SetUnread(has bool) {
	if n := l.n.Load(); n != nil {
		n.SetUnread(has)
	}
}
