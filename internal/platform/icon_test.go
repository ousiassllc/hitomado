package platform

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var trayVariants = []string{"light", "dark"}

// trayBadgeFill は素材（tray/*.svg）のバッジの塗りの半径（px）。縁は両サイズとも一辺の 20%（外径が 40%）。
var trayBadgeFill = map[int]float64{16: 2.2, 32: 4.9}

func decodeTray(t *testing.T, size int, variant string) image.Image {
	t.Helper()
	b, err := trayFS.ReadFile(fmt.Sprintf("tray/tray-%d-%s.png", size, variant))
	require.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(b))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, size, size), img.Bounds(), "%d-%s の大きさ", size, variant)
	return img
}

func rgb8(c color.Color) [3]uint8 {
	r, g, b, _ := c.RGBA()
	return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
}

// 未読バッジは素材に描いてある。位置・大きさ・色が仕様（右下・一辺の約 40%・#7a4545・白縁）どおりであることを守る。
func TestTrayUnreadBadgeMatchesSpec(t *testing.T) {
	for _, size := range traySizes {
		for _, theme := range trayVariants {
			t.Run(fmt.Sprintf("%d-%s", size, theme), func(t *testing.T) {
				plain := decodeTray(t, size, theme)
				badged := decodeTray(t, size, theme+"-badge")
				s := float64(size)
				cx, cy := 0.769*s, 0.769*s              // 右下。中心は端から一辺の約 23%
				rim, fill := 0.2*s, trayBadgeFill[size] // 縁の外径が一辺の 40%

				ring := 0
				for y := range size {
					for x := range size {
						d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
						got := rgb8(badged.At(x, y))
						switch {
						case d < fill-1:
							assert.Equal(t, [3]uint8{0x7a, 0x45, 0x45}, got, "(%d,%d) はバッジの塗り", x, y)
						case d > fill+0.35 && d < rim-0.35:
							ring++
							assert.GreaterOrEqual(t, min(got[0], got[1], got[2]), uint8(200), "(%d,%d) は白縁（アンチエイリアスで少し混ざる）", x, y)
						case d > rim+1 && !inTileCorner(x, y, size):
							assert.Equal(t, plain.At(x, y), badged.At(x, y), "(%d,%d) はバッジの外。バッジ無しと同じ", x, y)
						}
					}
				}
				assert.GreaterOrEqual(t, ring, 4, "白縁の画素が見つかる")
				assert.NotEqual(t, plain.At(int(cx), int(cy)), badged.At(int(cx), int(cy)), "バッジ有無でバッジ中心の画素が変わる")
			})
		}
	}
}

// inTileCorner は、タイルの角の丸み（素材のバッジ無し・有りで半径が違う）が出る四隅か。
func inTileCorner(x, y, size int) bool {
	m := size / 5
	return (x < m || x >= size-m) && (y < m || y >= size-m)
}

// badgeColor（バッジの塗り）は tokens.css の --danger（ライト値。最初の宣言）と同じ値。トークンを変えたら素材も作り直す
// （docs/components/overview.md §5.3 のウィンドウ外の表面）。
func TestTrayBadgeColorMatchesDangerToken(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "styles", "tokens.css"))
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^\s*--danger:\s*#([0-9a-fA-F]{6});`).FindSubmatch(b)
	require.NotNil(t, m, "tokens.css に --danger（#rrggbb）が無い")
	for _, size := range traySizes {
		for _, theme := range trayVariants {
			c := rgb8(decodeTray(t, size, theme+"-badge").At(size*77/100, size*77/100))
			assert.Equal(t, strings.ToLower(string(m[1])), fmt.Sprintf("%02x%02x%02x", c[0], c[1], c[2]), "%d-%s", size, theme)
		}
	}
}

func TestLoadTrayIcons(t *testing.T) {
	for _, goos := range []string{"windows", "darwin", "linux"} {
		icons, err := loadTrayIcons(goos)
		require.NoError(t, err, goos)
		all := [][]byte{icons.light, icons.lightUnread, icons.dark, icons.darkUnread}
		for i, a := range all {
			assert.NotEmpty(t, a, goos)
			for _, b := range all[i+1:] {
				assert.NotEqual(t, a, b, "%s: 4 種類（ライト・ダーク・未読）は別の画像", goos)
			}
		}
	}
}

func TestTrayImageIsICOOnWindowsAndPNGElsewhere(t *testing.T) {
	ico, err := trayImage("dark-badge", "windows")
	require.NoError(t, err)
	require.Equal(t, []byte{0, 0, 1, 0, 2, 0}, ico[:6], "ICONDIR（ICO・2 枚）")
	for i, size := range traySizes {
		e := ico[6+16*i : 6+16*(i+1)]
		assert.EqualValues(t, size, e[0], "幅")
		assert.EqualValues(t, size, e[1], "高さ")
		n, off := binary.LittleEndian.Uint32(e[8:]), binary.LittleEndian.Uint32(e[12:])
		want, err := trayFS.ReadFile(fmt.Sprintf("tray/tray-%d-dark-badge.png", size))
		require.NoError(t, err)
		assert.Equal(t, want, ico[off:off+n], "%dpx の中身は素材の PNG そのもの", size)
	}

	for _, goos := range []string{"darwin", "linux"} {
		b, err := trayImage("light", goos)
		require.NoError(t, err)
		want, err := trayFS.ReadFile("tray/tray-32-light.png")
		require.NoError(t, err)
		assert.Equal(t, want, b, goos)
	}
}

func TestIcoFromPNGsRejectsBadInput(t *testing.T) {
	_, err := icoFromPNGs([][]byte{[]byte("not png")})
	assert.Error(t, err)

	big := image.NewRGBA(image.Rect(0, 0, 256, 256))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, big))
	_, err = icoFromPNGs([][]byte{buf.Bytes()})
	assert.Error(t, err, "256px 以上は ICO の幅 1 バイトに入らない")
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
