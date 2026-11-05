package platform

import (
	"bytes"
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
)

func solidPNG(t *testing.T, size int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestBadgeIconDrawsBadgeDotInBottomRight(t *testing.T) {
	base := solidPNG(t, 32, color.RGBA{R: 10, G: 20, B: 200, A: 255})
	out, err := BadgeIcon(base)
	require.NoError(t, err)

	img, err := png.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 32, 32), img.Bounds())

	r, g, b, _ := img.At(0, 0).RGBA()
	assert.EqualValues(t, []uint32{10, 20, 200}, []uint32{r >> 8, g >> 8, b >> 8}, "左上は元のまま")

	r, g, b, _ = img.At(32-8-1, 32-8-1).RGBA()
	assert.EqualValues(t, []uint32{uint32(badgeColor.R), uint32(badgeColor.G), uint32(badgeColor.B)}, []uint32{r >> 8, g >> 8, b >> 8}, "バッジ中心は --danger 色")
}

func TestBadgeIconRejectsInvalidPNG(t *testing.T) {
	_, err := BadgeIcon([]byte("not png"))
	assert.Error(t, err)
}

// badgeColor は tokens.css の --danger（ライト値。最初の宣言）と同じ値を複製している。トークンを変えたらここも直す
// （docs/components/overview.md §5.3 のウィンドウ外の表面）。
func TestBadgeColorMatchesDangerToken(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "styles", "tokens.css"))
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^\s*--danger:\s*#([0-9a-fA-F]{6});`).FindSubmatch(b)
	require.NotNil(t, m, "tokens.css に --danger（#rrggbb）が無い")
	assert.Equal(t, fmt.Sprintf("%02x%02x%02x", badgeColor.R, badgeColor.G, badgeColor.B), strings.ToLower(string(m[1])))
}
