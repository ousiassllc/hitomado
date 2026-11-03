//go:build android

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	wevents "github.com/wailsapp/wails/v3/pkg/events"

	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/platform"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

// 試作（#81）の自己診断。Android の実機・エミュレータで、Java ブリッジ越しの機能と実際の OAuth フローを 1 回ずつ通し、
// 結果を <dataDir>/selfcheck.log に書く。Android ではアプリの環境変数を渡しにくいため、
// データフォルダに selfcheck.enable があるときだけ動かす。本番には残さない（本実装の Issue で削除する）。
// Run 開始後に非同期で動かす。
func startAndroidSelfCheck(dir string) {
	if _, err := os.Stat(filepath.Join(dir, "selfcheck.enable")); err != nil {
		return
	}
	// Run 前は Dialog などが「アプリ開始前」で何もしないので、ApplicationStarted を待つ。
	application.Get().Event.OnApplicationEvent(wevents.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go runAndroidSelfCheck(dir)
	})
}

func runAndroidSelfCheck(dir string) {
	if _, err := os.Stat(filepath.Join(dir, "selfcheck.enable")); err != nil {
		return
	}
	out, err := os.OpenFile(filepath.Join(dir, "selfcheck.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // 自己診断（試作）。dir はアプリ専用領域
	if err != nil {
		return
	}
	defer func() { _ = out.Close() }()
	logf := func(format string, a ...any) {
		_, _ = fmt.Fprintf(out, time.Now().Format("15:04:05 ")+format+"\n", a...)
		_ = out.Sync()
	}
	logf("started: %s", time.Now().Format(time.RFC3339))
	selfCheckSecrets(logf)
	selfCheckSaveFile(logf)
	selfCheckNotify(logf)
	selfCheckOAuth(dir, logf)
	logf("DONE")
}

func selfCheckSecrets(logf func(string, ...any)) {
	s := secrets.NewKeyring()
	small, big := "dummy-small-value", strings.Repeat("x", 4096) // Windows 資格情報の上限（約 2.5KB）を超える値
	for name, v := range map[string]string{"small": small, "big4096": big} {
		key := "hitomado/selfcheck/" + name
		if err := s.Set(key, v); err != nil {
			logf("secrets %s: Set error: %v", name, err)
			continue
		}
		got, err := s.Get(key)
		logf("secrets %s: Get ok=%v err=%v", name, got == v, err)
		if err := s.Delete(key); err != nil {
			logf("secrets %s: Delete error: %v", name, err)
		}
		_, err = s.Get(key)
		logf("secrets %s: after Delete -> %v (want ErrNotFound)", name, err)
	}
	logf("secrets missing: %v (want ErrNotFound)", func() error { _, err := s.Get("hitomado/selfcheck/none"); return err }())
}

func selfCheckSaveFile(logf func(string, ...any)) {
	app := application.Get()
	// Wails の保存ダイアログそのもの（エラーを返す想定）
	_, err := app.Dialog.SaveFile().SetFilename("a.txt").PromptForSingleSelection()
	logf("wails Dialog.SaveFile: err=%v", err)
	// 試作の代替（アプリ専用 files/exports）
	path, ok, err := platform.NewFiles(app, func() application.Window { return nil }).SaveFile(context.Background(), "a.txt")
	if err == nil {
		err = os.WriteFile(path, []byte("hello"), 0o600)
	}
	logf("platform.Files.SaveFile (android): path=%s ok=%v writeErr=%v", path, ok, err)
}

func selfCheckNotify(logf func(string, ...any)) {
	p, _ := json.Marshal(map[string]any{"title": "Hitomado selfcheck", "body": "dummy"})
	application.Android.Notify(string(p))
	logf("Android.Notify called (結果は dumpsys notification で確認)")
}

// selfCheckOAuth は本物の oauth.Flow を、ローカルの偽 Google（認可 → 即リダイレクト・トークン・userinfo）に向けて通す。
// ブラウザ起動は本番と同じ platform.Browser（Wails の OpenURL → Android の Intent）。
func selfCheckOAuth(dir string, logf func(string, ...any)) {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		http.Redirect(w, r, q.Get("redirect_uri")+"?code=fake-code&state="+q.Get("state"), http.StatusFound) //nolint:gosec // 自己診断用の偽 IdP（試作）
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fake-access", "refresh_token": "fake-refresh", "token_type": "Bearer",
			"expires_in": 3600, "scope": strings.Join(oauth.Scopes, " "),
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"email": "Selfcheck@Example.com", "name": "Self Check"})
	})
	idp := httptest.NewServer(mux) // 127.0.0.1 の一時ポート
	defer idp.Close()

	open := platform.NewBrowser(application.Get()).Open
	f := oauth.NewFlow(oauth.Options{
		OpenBrowser: func(u string) error {
			// 別プロセス（adb shell の nc）から同じ URL を辿れるようにファイルへ書く（ダミーの state のみ。秘密は含まない）
			_ = os.WriteFile(filepath.Join(dir, "selfcheck.url"), []byte(u), 0o600)
			return open(u)
		},
		AuthURL: idp.URL + "/auth", TokenURL: idp.URL + "/token", UserinfoURL: idp.URL + "/userinfo",
		Timeout: 120 * time.Second,
	})
	res, err := f.Authorize(context.Background(), oauth.ClientConfig{ClientID: "x", ClientSecret: "y"})
	logf("oauth Flow.Authorize via system browser: email=%q refreshTokenSet=%v err=%v", res.Email, res.RefreshToken != "", err)
}
