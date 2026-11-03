package capability

// PlatformService は実行中の OS で提供する機能をフロントへ伝える Wails サービス（バインディング仕様 §7.3）。
// どの OS でも同じシグネチャで、値だけが変わる（バインディングは OS ごとに生成し直さない）。
type PlatformService struct{ caps Capabilities }

// NewPlatformService は caps を返す PlatformService を作る。
func NewPlatformService(caps Capabilities) *PlatformService { return &PlatformService{caps: caps} }

// GetCapabilities はこの OS で提供する機能を返す。
func (s *PlatformService) GetCapabilities() Capabilities { return s.caps }
