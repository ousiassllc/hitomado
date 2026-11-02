package app

import "github.com/ousiassllc/hitomado/internal/app/threadcache"

// Services は Wails に登録するバインディングサービスの組。
type Services struct {
	Account *AccountService
	Inbox   *InboxService
	Thread  *ThreadService
	Compose *ComposeService
	AI      *AIService
	App     *AppService
}

// NewServices は共有状態（スレッド本文 LRU）を作り、各サービスを組み立てる。
func NewServices(d *Deps) *Services {
	d.Threads = threadcache.New(d.Gmail, threadcache.DefaultCapacity)
	compose := NewComposeService(d)
	return &Services{
		Account: NewAccountService(d),
		Inbox:   NewInboxService(d),
		Thread:  NewThreadService(d),
		Compose: compose,
		AI:      NewAIService(d, compose),
		App:     NewAppService(d),
	}
}
