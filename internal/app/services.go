package app

import "github.com/ousiassllc/hitomado/internal/app/threadcache"

// Services は Wails に登録するバインディングサービスの組。
type Services struct {
	Account *AccountService
	Inbox   *InboxService
	Thread  *ThreadService
	Compose *ComposeService
	AI      *AIService
	Agent   *AgentService
	App     *AppService
	Log     *LogService
	MCP     *MCPService
}

// NewServices は各サービスを組み立てる。共有のスレッド本文 LRU（d.Threads）が未設定なら作る
// （同期のイベントで無効化するため、main は先に作って Emitter を包んでから渡す）。
func NewServices(d *Deps) *Services {
	if d.Threads == nil {
		d.Threads = threadcache.New(d.Gmail, threadcache.DefaultCapacity)
	}
	compose := NewComposeService(d)
	inbox := NewInboxService(d)
	return &Services{
		Account: NewAccountService(d),
		Inbox:   inbox,
		Thread:  NewThreadService(d),
		Compose: compose,
		AI:      NewAIService(d, compose),
		Agent:   NewAgentService(d),
		App:     NewAppService(d),
		Log:     NewLogService(d),
		MCP:     NewMCPService(d, newMCPBackend(d, inbox)),
	}
}
