package androidnet

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/ousiassllc/hitomado/internal/exportfile"
)

// bridgeBackgroundBlocked は Java（NetBridge.resolve）が「アプリが背面で、有効なネットワークが無い」ときに返すコード。
const bridgeBackgroundBlocked = "background_no_network"

// mapResolveError は Java の失敗を、Dialer が判断に使うエラーに対応させる（background_no_network は ErrBackgroundBlocked）。
func mapResolveError(err error) error {
	var be *exportfile.BridgeError
	if errors.As(err, &be) && be.Code == bridgeBackgroundBlocked {
		return fmt.Errorf("%w: %w", ErrBackgroundBlocked, err)
	}
	return err
}

// Diagnostics は名前解決が失敗したときの端末の状態。Java（NetBridge）が JSON で返す。値はすべて、秘密を含まない種類・状態・端末の設定値。
// Private DNS のホスト名（利用者のプロファイル ID を含みうる）は載せない（モードだけ）。
type Diagnostics struct {
	// Process はアプリのプロセスの重要度: foreground・visible・service・cached。背面（cached など）では OS が通信を止めることがある。
	Process string `json:"process,omitempty"`
	// Network は有効なネットワークの種類: wifi・cellular・ethernet・vpn・other・none。
	Network   string `json:"network,omitempty"`
	Validated bool   `json:"validated"`
	Metered   bool   `json:"metered"`
	// PrivateDNS は off・opportunistic・strict。
	PrivateDNS string `json:"private_dns,omitempty"`
	// DNSServers はネットワークの DNS サーバーのアドレス（端末の設定値）。
	DNSServers []string `json:"dns_servers,omitempty"`
	// RestrictBackground は Data Saver の状態: disabled・whitelisted・enabled。
	RestrictBackground string `json:"restrict_background,omitempty"`
	// BackgroundRestricted はアプリが「制限」の電池設定で背面の動作を禁じられているか。
	BackgroundRestricted bool `json:"background_restricted"`
	// DeviceIdle は Doze 中か。
	DeviceIdle bool `json:"device_idle"`
	// StandbyBucket はアプリのスタンバイバケット: active・working_set・frequent・rare・restricted。
	StandbyBucket string `json:"standby_bucket,omitempty"`
}

// ParseDiagnostics は Java の返した JSON を Diagnostics にする。
func ParseDiagnostics(raw string) (Diagnostics, error) {
	var d Diagnostics
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return Diagnostics{}, fmt.Errorf("diagnostics: malformed: %w", err)
	}
	return d, nil
}

// Attrs はログの属性にする。DNS サーバーは IP として読めるものだけを通す（Java の値をそのまま載せない）。
func (d Diagnostics) Attrs() []any {
	attrs := []any{
		"process", d.Process, "network", d.Network, "validated", d.Validated, "metered", d.Metered,
		"private_dns", d.PrivateDNS, "restrict_background", d.RestrictBackground,
		"background_restricted", d.BackgroundRestricted, "device_idle", d.DeviceIdle, "standby_bucket", d.StandbyBucket,
	}
	var servers []string
	for _, s := range d.DNSServers {
		if ip := net.ParseIP(s); ip != nil {
			servers = append(servers, ip.String())
		}
	}
	return append(attrs, "dns_servers", strings.Join(servers, ","))
}

// ResolverKind は Go が名前解決に使う解決器の種類（"cgo" は getaddrinfo、"go" は Go の DNS クライアント）を返す。
// cgoResolver はビルドが cgo の解決器を持つか（resolver_cgo.go・resolver_nocgo.go）。
// GODEBUG=netdns で上書きされている場合は、その値を末尾に付ける。
func ResolverKind(preferGo bool, godebug string) string {
	kind := cgoResolver
	if preferGo {
		kind = "go"
	}
	for _, kv := range strings.Split(godebug, ",") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(kv), "netdns="); ok {
			kind += " (GODEBUG netdns=" + v + ")"
		}
	}
	return kind
}
