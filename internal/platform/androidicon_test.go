package platform

import (
	"fmt"
	"image"
	"image/png"
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
			f, err := os.Open(filepath.Join(res, "drawable-"+density, "ic_launcher_"+layer+".png"))
			require.NoError(t, err)
			img, err := png.Decode(f)
			require.NoError(t, f.Close())
			require.NoError(t, err)
			canvas := 108 * scale
			require.EqualValues(t, canvas, img.Bounds().Dx(), "%s %s: 108dp のキャンバス", density, layer)
			require.EqualValues(t, canvas, img.Bounds().Dy(), "%s %s", density, layer)
			// 前景のプリズム（右下の薄いぼかし）は半透明なので、ほぼ不透明（≥ 0.78）の画素だけでリングを測る。
			bounds[i] = opaqueBounds(img, 200)
			dp := float64(bounds[i].Dx()) / scale
			assert.InDelta(t, 48, dp, 1.0, "%s %s: リングの外径（dp）", density, layer)
			assert.InDelta(t, 54, float64(bounds[i].Min.X+bounds[i].Max.X)/2/scale, 0.5, "%s %s: 横の中心", density, layer)
			assert.InDelta(t, 54, float64(bounds[i].Min.Y+bounds[i].Max.Y)/2/scale, 0.5, "%s %s: 縦の中心", density, layer)
		}
		assert.Equal(t, bounds[0], bounds[1], fmt.Sprintf("%s: 前景とモノクロのリングは同じ大きさ", density))
	}
}
