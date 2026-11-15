package platform

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// opaqueBounds は、不透明度が threshold 以上の画素を囲む四角（px）を返す。
func opaqueBounds(img image.Image, threshold uint32) image.Rectangle {
	b := img.Bounds()
	out := image.Rectangle{Min: b.Max, Max: b.Min}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a>>8 >= threshold {
				out.Min.X, out.Min.Y = min(out.Min.X, x), min(out.Min.Y, y)
				out.Max.X, out.Max.Y = max(out.Max.X, x+1), max(out.Max.Y, y+1)
			}
		}
	}
	return out
}

// Android のアダプティブアイコンは 108dp のキャンバスで、リングの外径がおよそ 48dp（セーフゾーンの 66dp に十分収まる）。
// 前景とモノクロ（テーマアイコン）のリングは同じ大きさ・同じ中心（#379）。
func TestAndroidAdaptiveIconRingIsAbout48dp(t *testing.T) {
	res := filepath.Join("..", "..", "build", "android", "app", "src", "main", "res")
	for density, scale := range map[string]float64{"mdpi": 1, "hdpi": 1.5, "xhdpi": 2, "xxhdpi": 3, "xxxhdpi": 4} {
		var bounds [2]image.Rectangle
		for i, layer := range []string{"foreground", "monochrome"} {
			b, err := fs.ReadFile(os.DirFS(res), "drawable-"+density+"/ic_launcher_"+layer+".png")
			require.NoError(t, err)
			img, err := png.Decode(bytes.NewReader(b))
			require.NoError(t, err)
			canvas := 108 * scale
			require.EqualValues(t, canvas, img.Bounds().Dx(), "%s %s: 108dp のキャンバス", density, layer)
			require.EqualValues(t, canvas, img.Bounds().Dy(), "%s %s", density, layer)
			// 縁のアンチエイリアスの薄い画素を除き、ほぼ不透明（≥ 0.78）の画素でリングを測る。
			bounds[i] = opaqueBounds(img, 200)
			dp := float64(bounds[i].Dx()) / scale
			assert.InDelta(t, 48, dp, 1.0, "%s %s: リングの外径（dp）", density, layer)
			assert.InDelta(t, 54, float64(bounds[i].Min.X+bounds[i].Max.X)/2/scale, 0.5, "%s %s: 横の中心", density, layer)
			assert.InDelta(t, 54, float64(bounds[i].Min.Y+bounds[i].Max.Y)/2/scale, 0.5, "%s %s: 縦の中心", density, layer)
		}
		assert.Equal(t, bounds[0], bounds[1], fmt.Sprintf("%s: 前景とモノクロのリングは同じ大きさ", density))
	}
}

// 前景はインク（#050505）一色のリングとフラップだけで、ロゴの「プリズムのかすみ」（紫・青の半透明の層）を含まない（#379）。
// かすみは色味のある（無彩色でない）画素なので、不透明度が 0 でない画素がすべてインク色であることで確かめる。
func TestAndroidAdaptiveIconForegroundHasNoPrismHaze(t *testing.T) {
	res := filepath.Join("..", "..", "build", "android", "app", "src", "main", "res")
	for _, density := range []string{"mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"} {
		b, err := fs.ReadFile(os.DirFS(res), "drawable-"+density+"/ic_launcher_foreground.png")
		require.NoError(t, err)
		img, err := png.Decode(bytes.NewReader(b))
		require.NoError(t, err)
		bounds := img.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
				if c.A == 0 {
					continue
				}
				if c.R > 0x10 || c.G > 0x10 || c.B > 0x10 {
					t.Fatalf("%s: (%d,%d) に、インク（#050505）でない画素 %v（プリズムのかすみが残っている）", density, x, y, c)
				}
			}
		}
	}
}

// 古いランチャーアイコン（mipmap）は画像全体がそのままアイコンになるので、アダプティブアイコンの見えている 72dp に当たる。
// リングの外径は、アダプティブ（108dp のキャンバスで 48dp）と同じ大きさに見えるよう、タイルの 48/72（≒ 0.667）で、
// 中心はタイルの中心。輪の太さは輪といっしょに拡大している（前景を中央の 72dp で切り出して拡大しているだけ）（#379）。
func TestAndroidLauncherMipmapRingMatchesAdaptiveIcon(t *testing.T) {
	res := filepath.Join("..", "..", "build", "android", "app", "src", "main", "res")
	for density, scale := range map[string]float64{"mdpi": 1, "hdpi": 1.5, "xhdpi": 2, "xxhdpi": 3, "xxxhdpi": 4} {
		for _, name := range []string{"ic_launcher", "ic_launcher_round"} {
			b, err := fs.ReadFile(os.DirFS(res), "mipmap-"+density+"/"+name+".png")
			require.NoError(t, err)
			img, err := png.Decode(bytes.NewReader(b))
			require.NoError(t, err)
			tile := 48 * scale
			require.EqualValues(t, tile, img.Bounds().Dx(), "%s %s: 48dp のタイル", density, name)
			require.EqualValues(t, tile, img.Bounds().Dy(), "%s %s", density, name)
			// リング（インク）は、地（#f5f5f4）よりずっと暗い画素。縁のアンチエイリアスの薄い画素は除く。
			ring := image.Rectangle{Min: img.Bounds().Max, Max: img.Bounds().Min}
			for y := range img.Bounds().Dy() {
				for x := range img.Bounds().Dx() {
					if c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA); c.A == 255 && c.R < 0x60 {
						ring.Min.X, ring.Min.Y = min(ring.Min.X, x), min(ring.Min.Y, y)
						ring.Max.X, ring.Max.Y = max(ring.Max.X, x+1), max(ring.Max.Y, y+1)
					}
				}
			}
			// 外径がタイルの 48/72。縁の量子化（±1px）と薄い画素の除外を見て、±1.5px（mdpi で約 0.03）の幅で許す。
			assert.InDelta(t, tile*48/72, float64(ring.Dx()), 1.5, "%s %s: リングの外径（px）", density, name)
			assert.InDelta(t, tile*48/72, float64(ring.Dy()), 1.5, "%s %s", density, name)
			assert.InDelta(t, tile/2, float64(ring.Min.X+ring.Max.X)/2, 0.75, "%s %s: 横の中心", density, name)
			assert.InDelta(t, tile/2, float64(ring.Min.Y+ring.Max.Y)/2, 0.75, "%s %s: 縦の中心", density, name)
		}
	}
}

// 古いランチャーアイコン（mipmap の ic_launcher・ic_launcher_round。端末が使うのはアダプティブだが manifest の参照先）にも、
// ロゴの「プリズムのかすみ」（紫・青の半透明の層）を含めない（#379）。
// 絵は Bone（#f5f5f4）の地にインク（#050505）のリングとフラップだけなので、不透明度が 0 でない画素はすべて無彩色
// （地とインクとその混ざり。かすみは色味がある）。
func TestAndroidLauncherMipmapHasNoPrismHaze(t *testing.T) {
	res := filepath.Join("..", "..", "build", "android", "app", "src", "main", "res")
	for _, density := range []string{"mdpi", "hdpi", "xhdpi", "xxhdpi", "xxxhdpi"} {
		for _, name := range []string{"ic_launcher", "ic_launcher_round"} {
			b, err := fs.ReadFile(os.DirFS(res), "mipmap-"+density+"/"+name+".png")
			require.NoError(t, err)
			img, err := png.Decode(bytes.NewReader(b))
			require.NoError(t, err)
			bounds := img.Bounds()
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
					if c.A == 0 {
						continue
					}
					if hi, lo := max(c.R, c.G, c.B), min(c.R, c.G, c.B); hi-lo > 2 {
						t.Fatalf("%s %s: (%d,%d) に、地（#f5f5f4）でもインク（#050505）でもない色味の画素 %v（プリズムのかすみが残っている）", density, name, x, y, c)
					}
				}
			}
		}
	}
}
