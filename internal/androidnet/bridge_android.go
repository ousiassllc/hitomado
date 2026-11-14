//go:build android

package androidnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	_ "unsafe" // go:linkname

	"github.com/ousiassllc/hitomado/internal/exportfile"
)

// Wails の Java ブリッジを名前で呼ぶ関数。internal/platform/docbridge_android.go と同じ仕組み（理由・制約もそちらを参照）。
// 本体のないこの宣言には、同じディレクトリの空のアセンブリファイル（bridge_android.s）が要る。
//
//go:linkname androidBridgeStringString github.com/wailsapp/wails/v3/pkg/application.androidBridgeStringString
func androidBridgeStringString(method, arg string) (string, bool)

var errBridgeUnavailable = errors.New("android bridge unavailable")

// bridgeFallback は WailsBridge.netResolve／netDiagnose（NetBridge）を呼ぶ Fallback。
// WailsBridge は Activity から作られるので、Activity の無い Worker（ヘッドレス同期）では使えない（errBridgeUnavailable）。
type bridgeFallback struct{}

func (bridgeFallback) LookupHost(ctx context.Context, host string) ([]string, error) {
	raw, err := callJava(ctx, "netResolve", map[string]string{"host": host})
	if err != nil {
		return nil, fmt.Errorf("netResolve: %w", mapResolveError(err))
	}
	var ips []string
	if err := json.Unmarshal([]byte(raw), &ips); err != nil {
		return nil, fmt.Errorf("netResolve: malformed response: %w", err)
	}
	return ips, nil
}

func (bridgeFallback) Diagnose(ctx context.Context) (Diagnostics, error) {
	raw, err := callJava(ctx, "netDiagnose", map[string]string{})
	if err != nil {
		return Diagnostics{}, fmt.Errorf("netDiagnose: %w", err)
	}
	return ParseDiagnostics(raw)
}

// callJava は WailsBridge.<method>(String) を呼び、封筒の value を返す。JNI の呼び出しは取り消せないので、別ゴルーチンで呼び、
// ctx が切れたら（呼び出しの終わりを待たずに）戻る。
func callJava(ctx context.Context, method string, payload any) (string, error) {
	arg, err := exportfile.ASCIIJSON(payload)
	if err != nil {
		return "", err
	}
	type result struct {
		value string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		raw, ok := androidBridgeStringString(method, arg)
		if !ok {
			done <- result{err: errBridgeUnavailable}
			return
		}
		v, err := exportfile.ParseEnvelope(raw)
		done <- result{value: v, err: err}
	}()
	select {
	case r := <-done:
		return r.value, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
