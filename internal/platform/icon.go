package platform

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// BadgeIcon は PNG アイコンの右下に未読を示す赤い丸（白縁付き）を描いた PNG を返す。
// トレイアイコンの未読バッジ表現（F-10。形式は実装で決める）。
func BadgeIcon(base []byte) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(base))
	if err != nil {
		return nil, fmt.Errorf("decode icon: %w", err)
	}
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)

	size := min(b.Dx(), b.Dy())
	radius := max(size/4, 3)
	cx, cy := b.Max.X-radius-1, b.Max.Y-radius-1
	fillCircle(dst, cx, cy, radius+max(radius/4, 1), color.RGBA{R: 255, G: 255, B: 255, A: 255})
	fillCircle(dst, cx, cy, radius, color.RGBA{R: 217, G: 48, B: 37, A: 255})

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, fmt.Errorf("encode icon: %w", err)
	}
	return out.Bytes(), nil
}

func fillCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r && image.Pt(x, y).In(img.Bounds()) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}
