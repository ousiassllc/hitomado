package platform

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func rgb8(c color.Color) [3]uint8 {
	r, g, b, _ := c.RGBA()
	return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)} //nolint:gosec // RGBA() は 16 ビット値なので >> 8 は 8 ビットに収まる
}

func decodeTrayPNG(t *testing.T, file string, size int) image.Image {
	t.Helper()
	b, err := trayFS.ReadFile(file)
	require.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(b))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, size, size), img.Bounds(), file)
	return img
}

// タスクバーのテーマ（SystemUsesLightTheme の 1・0）と未読の有無で、選ばれる素材が決まる。
func TestPickTrayIcon(t *testing.T) {
	for _, c := range []struct {
		systemUsesLightTheme uint32
		unread               bool
		want                 string
	}{
		{1, false, "tray-light-taskbar"},
		{1, true, "tray-light-taskbar-badge"},
		{0, false, "tray-dark-taskbar"},
		{0, true, "tray-dark-taskbar-badge"},
	} {
		assert.Equal(t, c.want, pickTrayIcon(c.systemUsesLightTheme, c.unread), "SystemUsesLightTheme=%d unread=%v", c.systemUsesLightTheme, c.unread)
	}
}

// 明るいタスクバー（SetIcon の側）に墨のグリフ、暗いタスクバー（SetDarkModeIcon の側）に白のグリフが入る。
func TestLoadTrayIconsMapsTaskbarThemeToFile(t *testing.T) {
	for _, c := range []struct {
		goos, ext string
	}{{"windows", ".ico"}, {"linux", ".png"}} {
		icons, err := loadTrayIcons(c.goos)
		require.NoError(t, err, c.goos)
		for _, v := range []struct {
			got  []byte
			file string
		}{
			{icons.light, "tray-light-taskbar"},
			{icons.lightUnread, "tray-light-taskbar-badge"},
			{icons.dark, "tray-dark-taskbar"},
			{icons.darkUnread, "tray-dark-taskbar-badge"},
		} {
			want, err := trayFS.ReadFile("tray/" + v.file + c.ext)
			require.NoError(t, err)
			assert.Equal(t, want, v.got, "%s: %s", c.goos, v.file)
		}
		assert.Nil(t, icons.template, c.goos)
	}
}

// macOS のメニューバーは、バッジの無いテンプレート画像だけ（未読でも変えない）。
func TestLoadTrayIconsDarwinIsTemplateOnly(t *testing.T) {
	icons, err := loadTrayIcons("darwin")
	require.NoError(t, err)
	want, err := trayFS.ReadFile("tray/trayTemplate@2x.png")
	require.NoError(t, err)
	assert.Equal(t, want, icons.template)
	assert.Nil(t, icons.light)
	assert.Nil(t, icons.lightUnread)
	assert.Nil(t, icons.dark)
	assert.Nil(t, icons.darkUnread)
}

// Windows の ICO は 16・20・24・32px を持つ（DPI 100%・125%・150%・200% で Windows が選ぶ）。
func TestTrayICOHasFourDPISizes(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, badge := range []string{"", "-badge"} {
			file := fmt.Sprintf("tray/tray-%s-taskbar%s.ico", theme, badge)
			ico, err := trayFS.ReadFile(file)
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(ico), 6+16*4, file)
			require.Equal(t, []byte{0, 0, 1, 0, 4, 0}, ico[:6], "%s: ICONDIR（ICO・4 枚）", file)
			for i, size := range []int{16, 20, 24, 32} {
				e := ico[6+16*i : 6+16*(i+1)]
				assert.EqualValues(t, size, e[0], "%s: 幅", file)
				assert.EqualValues(t, size, e[1], "%s: 高さ", file)
				n, off := binary.LittleEndian.Uint32(e[8:]), binary.LittleEndian.Uint32(e[12:])
				assert.LessOrEqual(t, uint64(off)+uint64(n), uint64(len(ico)), "%s: %dpx の中身が ICO に収まる", file, size)
			}
		}
	}
}

// グリフは地の無い単色（明るいタスクバー用は墨 #050505、暗いタスクバー用は白）で、四隅は透明。
func TestTrayGlyphIsFlatInkOrWhiteOnTransparent(t *testing.T) {
	for theme, want := range map[string][3]uint8{"light": {0x05, 0x05, 0x05}, "dark": {0xff, 0xff, 0xff}} {
		img := decodeTrayPNG(t, "tray/tray-"+theme+"-taskbar.png", 32)
		opaque := 0
		for y := range 32 {
			for x := range 32 {
				_, _, _, a := img.At(x, y).RGBA()
				if a == 0 {
					continue
				}
				opaque++
				if a == 0xffff {
					assert.Equal(t, want, rgb8(img.At(x, y)), "%s (%d,%d)", theme, x, y)
				}
			}
		}
		assert.Positive(t, opaque, theme)
		for _, p := range [][2]int{{0, 0}, {31, 0}, {0, 31}, {31, 31}} {
			_, _, _, a := img.At(p[0], p[1]).RGBA()
			assert.Zero(t, a, "%s の四隅は透明（タイルを持たない）", theme)
		}
	}
}

// 未読バッジの塗りは tokens.css の --danger（ライト値。最初の宣言）と同じ #7a4545。トークンを変えたら素材も作り直す
// （docs/components/overview.md §5.3 のウィンドウ外の表面）。バッジ無しの同じ位置は、透明かグリフの色で、バッジの色ではない。
func TestTrayBadgeColorMatchesDangerToken(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "styles", "tokens.css"))
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^\s*--danger:\s*#([0-9a-fA-F]{6});`).FindSubmatch(b)
	require.NotNil(t, m, "tokens.css に --danger（#rrggbb）が無い")
	for _, theme := range []string{"light", "dark"} {
		badged := decodeTrayPNG(t, "tray/tray-"+theme+"-taskbar-badge.png", 32)
		plain := decodeTrayPNG(t, "tray/tray-"+theme+"-taskbar.png", 32)
		const x, y = 25, 25 // 右下のバッジの中心の近く
		c := rgb8(badged.At(x, y))
		assert.Equal(t, strings.ToLower(string(m[1])), fmt.Sprintf("%02x%02x%02x", c[0], c[1], c[2]), theme)
		_, _, _, a := badged.At(x, y).RGBA()
		assert.EqualValues(t, 0xffff, a, "%s: バッジの塗りは不透明", theme)
		assert.NotEqual(t, badged.At(x, y), plain.At(x, y), "%s: バッジ有無で右下の画素が変わる", theme)
	}
}

// macOS のテンプレート画像は、黒と透明度だけ（色を持たない）で、@2x は 2 倍の大きさ。
func TestTrayTemplateIsBlackAndAlphaOnly(t *testing.T) {
	for file, size := range map[string]int{"tray/trayTemplate.png": 16, "tray/trayTemplate@2x.png": 32} {
		img := decodeTrayPNG(t, file, size)
		visible := 0
		for y := range size {
			for x := range size {
				_, _, _, a := img.At(x, y).RGBA()
				if a == 0 {
					continue
				}
				visible++
				assert.Equal(t, [3]uint8{0, 0, 0}, rgb8(img.At(x, y)), "%s (%d,%d) は黒", file, x, y)
			}
		}
		assert.Positive(t, visible, file)
	}
}

// windowBackground は tokens.css の --bone（ライト＝:root の最初の宣言・ダーク＝prefers-color-scheme: dark の中の宣言）と同じ値を複製している。
// トークンを変えたらここも直す（docs/components/overview.md §5.3 のウィンドウ外の表面）。
func TestWindowBackgroundMatchesBoneToken(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "styles", "tokens.css"))
	require.NoError(t, err)
	all := regexp.MustCompile(`(?m)^\s*--bone:\s*#([0-9a-fA-F]{6});`).FindAllSubmatch(b, -1)
	require.Len(t, all, 2, "tokens.css の --bone（ライト・ダーク）が 2 つ無い")
	hex := func(c application.RGBA) string { return fmt.Sprintf("%02x%02x%02x", c.Red, c.Green, c.Blue) }
	assert.Equal(t, hex(WindowBackground(false)), strings.ToLower(string(all[0][1])), "ライト")
	assert.Equal(t, hex(WindowBackground(true)), strings.ToLower(string(all[1][1])), "ダーク")
}
