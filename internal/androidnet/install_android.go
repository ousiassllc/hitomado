//go:build android

package androidnet

import (
	"net"
	"net/http"
	"os"
	"time"
)

// アプリの起動時（libwails.so の読み込み時。Activity なしで動く WorkManager の Worker も同じ）に、http.DefaultTransport の接続を
// 名前解決の救済つきに差し替える。アプリの HTTP クライアント（OAuth・Gmail・AI・更新確認）は、いずれも DefaultTransport か、
// その Clone（gmail.defaultTransport）を使うので、ここ 1 か所で全部に効く。Clone は DialContext を引き継ぐ。
func init() {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return
	}
	// 標準の DefaultTransport と同じ接続の設定（net/http の DefaultTransport）。
	base := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	Install(t, &Dialer{
		Base:     base.DialContext,
		Fallback: bridgeFallback{},
		Resolver: ResolverKind(net.DefaultResolver.PreferGo, os.Getenv("GODEBUG")),
	})
}
