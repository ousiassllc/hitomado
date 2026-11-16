#!/usr/bin/env python3
"""Android のランチャーアイコン（前景・モノクロ・mipmap）の PNG を、元の SVG から書き出す（#386）。

    python3 build/android/scripts/gen-icons.py          # res/ の PNG を書き出す
    python3 build/android/scripts/gen-icons.py --check  # 書き出した結果が、コミット済みの PNG と「少しの違いなら同じ」かを確かめる（書き換えない）

--check の比べ方（Chrome の版で縁の画素がわずかに動いても落とさず、絵が変わったら落とす）:
縦横が違えば失敗。PNG を RGBA にデコードし、画素ごと・チャンネルごとの差（0〜255。透明な画素の色は見えないので、
色は透明度を掛けた値で比べる）を出す。全チャンネルの差の平均が MEAN_DIFF_MAX 以下、かつ、どれかのチャンネルの差が
PIXEL_DIFF を超えた画素の割合が OVER_RATIO_MAX 以下なら合格。平均も割合も、数える画素は「どちらかの PNG が透明でない画素」（絵の領域）だけ
（キャンバス全体で割ると、透明な余白が大きい前景でリングの一部が消えても薄まって通る）。どちらかを超えたら、ファイル名・平均差・はみ出た割合を出して失敗。

入力は build/android/icons/ic_launcher_{foreground,monochrome}.svg と build/appicon.png（角丸の四角の切り抜きの形だけを使う）。
mipmap の絵は前景の SVG そのもの（古いランチャーアイコンは画像全体がアイコンになるので、アダプティブの見えている中央 72dp を、
前景の xxxhdpi の描画から切り出して Bone の地に敷き、各密度へ縮める）。
出力は build/android/app/src/main/res/ の drawable-*dpi/ic_launcher_{foreground,monochrome}.png（108dp のキャンバス）と
mipmap-*dpi/ic_launcher{,_round}.png（48dp。丸は円の切り抜き）。

SVG の描画は Chrome（headless）。Pillow（版は build/android/scripts/gen-icons.requirements.txt）で、
mipmap の縮小（Lanczos）と切り抜きをする。Chrome は環境変数 CHROME か、PATH の google-chrome・chromium などを使う。
"""

import argparse
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

from PIL import Image, ImageChops, ImageDraw

ROOT = Path(__file__).resolve().parents[3]
ICONS = ROOT / "build" / "android" / "icons"
RES = ROOT / "build" / "android" / "app" / "src" / "main" / "res"
APPICON = ROOT / "build" / "appicon.png"

# 密度 → 倍率（mdpi = 1）。
DENSITIES = {"mdpi": 1, "hdpi": 1.5, "xhdpi": 2, "xxhdpi": 3, "xxxhdpi": 4}
# 前景・モノクロのキャンバスは 108dp、mipmap のタイルは 48dp。
ADAPTIVE_DP = 108
TILE_DP = 48
# mipmap は、前景の xxxhdpi の描画から中央 72dp を切り出して、各密度へ縮める。
MIPMAP_CROP_DP = 72
# mipmap の地（values/colors.xml の ic_launcher_background）。
BONE = (245, 245, 244, 255)

# --check の合格のしきい値（docs/environment/android.md §6.2 にも書く）。
# 絵の領域（どちらかの PNG が透明でない画素）での、全チャンネル（RGBA）の差の平均がこの値以下であること。
MEAN_DIFF_MAX = 1.0
# どれかのチャンネルの差がこの値を超えた画素を「はみ出た画素」と数える。
PIXEL_DIFF = 16
# はみ出た画素が絵の領域に占める割合の上限（0.005 = 0.5%）。
OVER_RATIO_MAX = 0.005


def find_chrome() -> str:
    candidates = [os.environ.get("CHROME", ""), "google-chrome", "google-chrome-stable", "chromium", "chromium-browser"]
    for c in candidates:
        if c and (path := shutil.which(c)):
            return path
    sys.exit("Chrome が見つからない（環境変数 CHROME に実行ファイルを指定する）")


def render_svg(chrome: str, svg_path: Path, px: int, out: Path) -> None:
    """SVG（width・height は dp）を、px × px の透過 PNG に描く。"""
    svg = svg_path.read_text(encoding="utf-8")
    svg = re.sub(r"<\?xml[^>]*\?>|<!--.*?-->", "", svg, flags=re.S)
    svg = re.sub(r'width="\d+" height="\d+"', f'width="{px}" height="{px}"', svg, count=1)
    with tempfile.TemporaryDirectory() as tmp:
        page = Path(tmp) / "page.html"
        page.write_text(
            f'<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0;background:transparent">{svg}</body></html>',
            encoding="utf-8",
        )
        subprocess.run(
            [
                chrome, "--headless=new", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
                "--default-background-color=00000000", "--force-device-scale-factor=1",
                f"--window-size={px},{px}", f"--screenshot={out}", page.as_uri(),
            ],
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            # Chrome のプロファイルは使い捨てのディレクトリへ（利用者のプロファイルに触れない）。
            env={**os.environ, "HOME": tmp},
        )


def build_all(chrome: str, out_res: Path) -> None:
    """out_res の下へ、全密度の PNG を書き出す。"""
    for layer in ("foreground", "monochrome"):
        for density, k in DENSITIES.items():
            dest = out_res / f"drawable-{density}" / f"ic_launcher_{layer}.png"
            dest.parent.mkdir(parents=True, exist_ok=True)
            render_svg(chrome, ICONS / f"ic_launcher_{layer}.svg", round(ADAPTIVE_DP * k), dest)

    # 前景の xxxhdpi は上で書き出したものをそのまま使う（描画をもう一度しない。切り出しの位置がずれると 1 画素の誤差が出る）。
    full = Image.open(out_res / "drawable-xxxhdpi" / "ic_launcher_foreground.png").convert("RGBA")
    margin = round((ADAPTIVE_DP - MIPMAP_CROP_DP) / 2 * DENSITIES["xxxhdpi"])
    art = full.crop((margin, margin, full.width - margin, full.height - margin))
    tile = Image.new("RGBA", art.size, BONE)
    tile.alpha_composite(art)
    tile = tile.convert("RGB")
    shape = Image.open(APPICON).convert("RGBA").getchannel("A")
    for density, k in DENSITIES.items():
        size = round(TILE_DP * k)
        body = tile.resize((size, size), Image.LANCZOS)
        square = body.convert("RGBA")
        square.putalpha(shape.resize((size, size), Image.LANCZOS))
        circle = body.convert("RGBA")
        mask = Image.new("L", (size, size), 0)
        ImageDraw.Draw(mask).ellipse((0, 0, size - 1, size - 1), fill=255)
        circle.putalpha(mask)
        dest_dir = out_res / f"mipmap-{density}"
        dest_dir.mkdir(parents=True, exist_ok=True)
        square.save(dest_dir / "ic_launcher.png", optimize=True)
        circle.save(dest_dir / "ic_launcher_round.png", optimize=True)


def _premultiplied(path: Path) -> Image.Image:
    """RGBA にデコードし、色に透明度を掛ける（透明な画素の色は見えないので、差に数えない）。"""
    img = Image.open(path).convert("RGBA")
    r, g, b, a = img.split()
    return Image.merge("RGBA", [ImageChops.multiply(c, a) for c in (r, g, b)] + [a])


def compare_pngs(a: Path, b: Path) -> tuple[bool, str]:
    """2 枚の PNG が「少しの違い」かを比べる。(合格か, 失敗の説明) を返す。"""
    ia, ib = _premultiplied(a), _premultiplied(b)
    if ia.size != ib.size:
        return False, f"サイズが違う: {ia.size[0]}x{ia.size[1]} と {ib.size[0]}x{ib.size[1]}"
    bands = ImageChops.difference(ia, ib).split()
    # 数える画素は、どちらかの絵が透明でない画素（絵の領域）だけ。キャンバス全体で割ると、透明な余白が大きい前景・モノクロでは
    # リングの一部が消えても平均・割合が薄まって通ってしまう。
    total = sum(ImageChops.lighter(ia.getchannel("A"), ib.getchannel("A")).histogram()[1:])
    if total == 0:
        return True, ""
    mean = sum(v * n for band in bands for v, n in enumerate(band.histogram())) / (total * len(bands))
    worst = bands[0]
    for band in bands[1:]:
        worst = ImageChops.lighter(worst, band)
    over = sum(worst.histogram()[PIXEL_DIFF + 1 :]) / total
    if mean > MEAN_DIFF_MAX or over > OVER_RATIO_MAX:
        return False, (
            f"平均差 {mean:.3f}（上限 {MEAN_DIFF_MAX}）、差が {PIXEL_DIFF} を超えた画素 {over:.3%}（上限 {OVER_RATIO_MAX:.1%}）"
        )
    return True, ""


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--check", action="store_true", help="書き出した結果が res/ のコミット済みの PNG と少しの違いなら同じかを確かめる（書き換えない）")
    args = ap.parse_args()
    chrome = find_chrome()

    if not args.check:
        build_all(chrome, RES)
        print("wrote", RES)
        return 0

    with tempfile.TemporaryDirectory() as tmp:
        out = Path(tmp)
        build_all(chrome, out)
        mismatched = []
        committed = sorted(p for p in RES.rglob("ic_launcher*.png"))
        generated = sorted(out.rglob("ic_launcher*.png"))
        if [p.relative_to(RES) for p in committed] != [p.relative_to(out) for p in generated]:
            print("PNG の一覧が違う:", [str(p.relative_to(RES)) for p in committed], [str(p.relative_to(out)) for p in generated])
            return 1
        for p in committed:
            ok, why = compare_pngs(p, out / p.relative_to(RES))
            if not ok:
                mismatched.append(f"{p.relative_to(ROOT)}: {why}")
        if mismatched:
            print("コミット済みの PNG と違う（書き出し直すなら gen-icons.py を --check なしで動かす）:", *mismatched, sep="\n  ")
            return 1
    print(f"OK: {len(committed)} 枚がコミット済みの PNG と同じ絵（平均差 {MEAN_DIFF_MAX} 以下、差が {PIXEL_DIFF} 超の画素 {OVER_RATIO_MAX:.1%} 以下）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
