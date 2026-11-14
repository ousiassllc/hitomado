package platform

import (
	"bytes"
	"embed"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/png" // image.DecodeConfig（icoFromPNGs）が PNG を読むため
)

// トレイアイコンの素材（新ロゴ v5c の tray/。#379）。ライト／ダーク × 未読バッジ有無 × 16・32px の 8 枚。
// 未読バッジ（#7a4545・白縁・一辺の約 40%・右下）も素材に描いてあるので、実行時には描かない。
// 虹色（プリズム）は 32px 以下・トレイには入っていない。
//
//go:embed tray/*.png
var trayFS embed.FS

// traySizes は素材のサイズ（px）。Windows は DPI に応じた大きさ（100% で 16、200% で 32）の画像を ICO から選ぶ。
var traySizes = []int{16, 32}

// trayIcons はトレイに渡す 4 種類の画像（Windows は ICO、それ以外は 32px の PNG）。
// ライト／ダークは OS（Windows ではタスクバー）のテーマに合わせて Wails が切り替える（SetIcon／SetDarkModeIcon）。
type trayIcons struct {
	light, lightUnread, dark, darkUnread []byte
}

func loadTrayIcons(goos string) (trayIcons, error) {
	var t trayIcons
	for _, v := range []struct {
		dst  *[]byte
		name string
	}{
		{&t.light, "light"}, {&t.lightUnread, "light-badge"}, {&t.dark, "dark"}, {&t.darkUnread, "dark-badge"},
	} {
		b, err := trayImage(v.name, goos)
		if err != nil {
			return trayIcons{}, err
		}
		*v.dst = b
	}
	return t, nil
}

// trayImage は variant（light／light-badge／dark／dark-badge）のトレイ画像を返す。
// Windows では 16・32px を 1 つの ICO にまとめる（Wails は ICO から表示サイズに合う画像を選ぶので、
// 100% の DPI では手で整えた 16px がそのまま出る）。それ以外の OS（macOS・Linux の試験用ビルド）は 32px の PNG を渡す。
func trayImage(variant, goos string) ([]byte, error) {
	pngs := make([][]byte, len(traySizes))
	for i, size := range traySizes {
		b, err := trayFS.ReadFile(fmt.Sprintf("tray/tray-%d-%s.png", size, variant))
		if err != nil {
			return nil, fmt.Errorf("read tray icon: %w", err)
		}
		pngs[i] = b
	}
	if goos != "windows" {
		return pngs[len(pngs)-1], nil
	}
	return icoFromPNGs(pngs)
}

// icoFromPNGs は PNG（それぞれ 255px 以下の正方形）を、PNG 圧縮の画像として 1 つの ICO にまとめる。
func icoFromPNGs(pngs [][]byte) ([]byte, error) {
	const headerSize, entrySize = 6, 16
	if len(pngs) == 0 || len(pngs) > 255 {
		return nil, fmt.Errorf("ICO needs 1 to 255 images, got %d", len(pngs))
	}
	var out bytes.Buffer
	// ICONDIR: 予約 0・種別 1（ICO）・画像数（255 以下を確認済み）
	out.Write([]byte{0, 0, 1, 0, byte(len(pngs)), 0}) //nolint:gosec // 画像数は 255 以下を確認済み
	offset := headerSize + entrySize*len(pngs)
	for _, p := range pngs {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(p))
		if err != nil {
			return nil, fmt.Errorf("decode tray icon: %w", err)
		}
		if cfg.Width != cfg.Height || cfg.Width < 1 || cfg.Width > 255 {
			return nil, fmt.Errorf("tray icon must be a square of at most 255px, got %dx%d", cfg.Width, cfg.Height)
		}
		// ICONDIRENTRY: 幅・高さ・色数 0・予約 0・プレーン 1・ビット深度 32・データ長・データ位置
		out.Write([]byte{byte(cfg.Width), byte(cfg.Height), 0, 0, 1, 0, 32, 0}) //nolint:gosec // 1〜255 を確認済み
		_ = binary.Write(&out, binary.LittleEndian, uint32(len(p)))             //nolint:gosec // PNG は数 KB
		_ = binary.Write(&out, binary.LittleEndian, uint32(offset))             //nolint:gosec // 同上
		offset += len(p)
	}
	for _, p := range pngs {
		out.Write(p)
	}
	return out.Bytes(), nil
}
