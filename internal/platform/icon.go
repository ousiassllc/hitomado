package platform

import (
	"embed"
	"fmt"
)

// トレイアイコンの素材（新ロゴ v5c の tray-v2。#379）。地（タイル）の無い透明背景の単色グリフで、
// タスクバーの明るさに合わせて 2 色を使い分ける。虹色（プリズム）は入っていない。
//   - tray-light-taskbar*: 明るいタスクバー用（墨 #050505）
//   - tray-dark-taskbar*: 暗いタスクバー用（白）
//   - *-badge: 未読バッジ付き（右下の #7a4545 の丸。リングとの間は白縁ではなく透明に抜いてある）
//
// Windows は ICO（16・20・24・32px 入り。DPI 100%・125%・150%・200% の大きさを Windows が選ぶ）、
// Linux は 32px の PNG を使う。macOS のメニューバーはバッジの無いテンプレート画像（黒＋透明度だけ。trayTemplate.png と @2x）。
//
//go:embed tray/*.ico tray/*.png
var trayFS embed.FS

// macOS のメニューバー用テンプレート画像（墨版の PNG から作った黒＋透明度だけの画像）。
// Wails は 1 枚だけ受け取り、メニューバーの高さに縮めて描くので、@2x（32px）を渡す。
const trayTemplate2x = "tray/trayTemplate@2x.png"

// trayIcons はトレイに渡す画像。Windows・Linux は light／dark × 未読の有無の 4 種類、macOS は template だけ。
type trayIcons struct {
	// light は SystemUsesLightTheme=1（明るいタスクバー）のとき、dark は 0（暗いタスクバー）のときに出る画像。
	// どちらが出るかは Wails が決める（SetIcon／SetDarkModeIcon）。Wails（v3.1.0）の Windows は、タスクバーのテーマを
	// HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize の SystemUsesLightTheme（AppsUseLightTheme ではない）から
	// 起動時に読み、WM_SETTINGCHANGE（ImmersiveColorSet）を受けるたびに読み直してアイコンを差し替える。
	light, lightUnread, dark, darkUnread []byte
	template                             []byte
}

// pickTrayIcon は、タスクバーのテーマ（Windows の SystemUsesLightTheme の値。1＝明るい、0＝暗い）と未読の有無から、使う素材の名前（拡張子なし）を返す。
func pickTrayIcon(systemUsesLightTheme uint32, unread bool) string {
	name := "tray-dark-taskbar"
	if systemUsesLightTheme != 0 {
		name = "tray-light-taskbar"
	}
	if unread {
		name += "-badge"
	}
	return name
}

// loadTrayIcons は OS に合う画像を読む。macOS はテンプレート画像だけ。
func loadTrayIcons(goos string) (trayIcons, error) {
	if goos == "darwin" {
		b, err := trayFS.ReadFile(trayTemplate2x)
		if err != nil {
			return trayIcons{}, fmt.Errorf("read tray icon: %w", err)
		}
		return trayIcons{template: b}, nil
	}
	var t trayIcons
	for _, v := range []struct {
		dst    *[]byte
		light  uint32
		unread bool
	}{
		{&t.light, 1, false}, {&t.lightUnread, 1, true}, {&t.dark, 0, false}, {&t.darkUnread, 0, true},
	} {
		b, err := trayImage(pickTrayIcon(v.light, v.unread), goos)
		if err != nil {
			return trayIcons{}, err
		}
		*v.dst = b
	}
	return t, nil
}

// trayImage は name（pickTrayIcon の返す名前）の画像を返す。Windows は ICO、それ以外（Linux）は 32px の PNG。
func trayImage(name, goos string) ([]byte, error) {
	ext := ".png"
	if goos == "windows" {
		ext = ".ico"
	}
	b, err := trayFS.ReadFile("tray/" + name + ext)
	if err != nil {
		return nil, fmt.Errorf("read tray icon: %w", err)
	}
	return b, nil
}
