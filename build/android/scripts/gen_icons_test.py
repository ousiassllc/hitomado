"""gen-icons.py の compare_pngs（--check の「少しの違いなら通す」比べ方）の単体テスト（#386）。Chrome は使わない。

    python3 -B -m unittest discover -s build/android/scripts -p 'gen_icons_test.py'
"""

import importlib.util
import random
import tempfile
import unittest
from pathlib import Path

from PIL import Image, ImageChops

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("gen_icons", HERE / "gen-icons.py")
gen_icons = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gen_icons)

# コミット済みの前景（432px）。実物の絵に手を入れて、しきい値が見た目の変化を捉えるかを確かめる。
BASE = gen_icons.RES / "drawable-xxxhdpi" / "ic_launcher_foreground.png"


class ComparePngs(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.tmp = Path(self._tmp.name)
        self.base = Image.open(BASE).convert("RGBA")

    def check(self, img):
        path = self.tmp / "other.png"
        img.save(path)
        return gen_icons.compare_pngs(BASE, path)

    def test_same_image_passes(self):
        self.assertEqual(self.check(self.base), (True, ""))

    def test_antialias_level_noise_passes(self):
        # 縁の画素（透明度が 0 でも 255 でもない画素）だけ、チャンネルを最大 ±4 動かす。Chrome の版の差くらいの違い。
        rng = random.Random(1)
        img = self.base.copy()
        px = img.load()
        for y in range(img.height):
            for x in range(img.width):
                r, g, b, a = px[x, y]
                if 0 < a < 255:
                    d = rng.randint(-4, 4)
                    px[x, y] = tuple(min(255, max(0, c + d)) for c in (r, g, b, a))
        ok, why = self.check(img)
        self.assertTrue(ok, why)

    def test_uniformly_one_level_lighter_passes(self):
        # 全体を 1 段階だけ薄くする（全画素の差が 1 以内）。
        img = self.base.copy()
        img.putalpha(img.getchannel("A").point(lambda v: max(0, v - 1)))
        ok, why = self.check(img)
        self.assertTrue(ok, why)

    def test_color_change_fails(self):
        # インク（黒に近い）を赤みのある色へ。
        r, g, b, a = self.base.split()
        img = Image.merge("RGBA", (r.point(lambda v: min(255, v + 120)), g, b, a))
        ok, why = self.check(img)
        self.assertFalse(ok)
        self.assertIn("平均差", why)

    def test_shifted_shape_fails(self):
        img = ImageChops.offset(self.base, 6, 0)
        ok, why = self.check(img)
        self.assertFalse(ok)
        self.assertIn("平均差", why)

    def test_erased_part_fails(self):
        img = self.base.copy()
        # 絵の外接矩形の上端の中央（リングがある所）を 60 × 30 画素だけ消す。
        left, top, right, _ = img.getchannel("A").getbbox()
        cx = (left + right) // 2
        img.paste((0, 0, 0, 0), (cx - 30, top, cx + 30, top + 30))
        ok, why = self.check(img)
        self.assertFalse(ok)
        self.assertIn("平均差", why)

    def test_size_mismatch_fails(self):
        ok, why = self.check(self.base.resize((self.base.width // 2, self.base.height // 2)))
        self.assertFalse(ok)
        self.assertIn("サイズが違う", why)

    def test_transparent_pixels_color_is_ignored(self):
        # 完全に透明な画素の RGB が違っても見た目は同じ。
        img = self.base.copy()
        px = img.load()
        for y in range(img.height):
            for x in range(img.width):
                if px[x, y][3] == 0:
                    px[x, y] = (255, 0, 255, 0)
        ok, why = self.check(img)
        self.assertTrue(ok, why)


if __name__ == "__main__":
    unittest.main()
