#!/usr/bin/env python3
"""UI Automator の画面ダンプ（uiautomator dump の XML）から、テキスト・content-desc で要素を探す（E2E 用。標準ライブラリのみ）。

使い方: ui.py <dump.xml> find <部分一致の文字列>   → 見つかれば中心座標 "x y" を出して 0、無ければ 1
        ui.py <dump.xml> has <部分一致の文字列>    → 見つかれば 0、無ければ 1
        ui.py <screenshot.png> inked <縦の始まり 0〜1> <縦の終わり 0〜1>
                                                   → 画面のその縦の範囲で、白でない画素の割合（0〜1）を出す。空白の画面なら 0 に近い
        ui.py <dump.xml> texts                     → ノードの数と、text・content-desc を持つノードの一覧（診断用）。常に 0
text・content-desc のどちらかに部分一致する、最初の（クリックの対象になる）ノードを使う。
"""
import re
import struct
import sys
import zlib
import xml.etree.ElementTree as ET


def nodes(path):
    for n in ET.parse(path).getroot().iter("node"):
        yield n


def center(bounds):
    m = re.match(r"\[(\d+),(\d+)\]\[(\d+),(\d+)\]", bounds or "")
    if not m:
        return None
    l, t, r, b = map(int, m.groups())
    if r <= l or b <= t:
        return None
    return (l + r) // 2, (t + b) // 2


def find(path, needle):
    for n in nodes(path):
        hay = (n.get("text") or "") + "\n" + (n.get("content-desc") or "")
        if needle in hay:
            c = center(n.get("bounds"))
            if c:
                return c
    return None


def texts(path):
    try:
        all_nodes = list(nodes(path))
    except ET.ParseError:
        print("(the dump is empty or not valid XML)")
        return
    shown = [(n.get("text") or "", n.get("content-desc") or "") for n in all_nodes]
    shown = [t for t in shown if t[0] or t[1]]
    print(f"{len(all_nodes)} nodes, {len(shown)} with text or content-desc")
    for text, desc in shown[:40]:
        print(f"  text={text!r} content-desc={desc!r}")


def inked_ratio(path, y0, y1):
    """PNG（8 bit の RGB／RGBA。screencap の出力）の、縦 y0〜y1（割合）の範囲にある白でない画素の割合。

    WebView の a11y ツリーは、作り直した WebView では更新されないことがある（実際に、画面には一覧が出ているのにダンプには行が無かった）。
    その場合に「空白の画面ではない」ことを画素で確かめるために使う。
    """
    data = open(path, "rb").read()
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError("not a PNG")
    pos, idat, width, height, bpp = 8, [], 0, 0, 0
    while pos < len(data):
        (length,) = struct.unpack(">I", data[pos:pos + 4])
        kind = data[pos + 4:pos + 8]
        body = data[pos + 8:pos + 8 + length]
        pos += 12 + length
        if kind == b"IHDR":
            width, height, depth, color = struct.unpack(">IIBB", body[:10])
            if depth != 8 or color not in (2, 6):
                raise ValueError("unsupported PNG (need 8 bit RGB/RGBA)")
            bpp = 3 if color == 2 else 4
        elif kind == b"IDAT":
            idat.append(body)
    raw = zlib.decompress(b"".join(idat))
    stride = width * bpp
    first, last = int(height * y0), int(height * y1)
    prev = bytearray(stride)
    inked = total = 0
    for y in range(last):
        base = y * (stride + 1)
        ftype = raw[base]
        line = bytearray(raw[base + 1:base + 1 + stride])
        for i in range(stride):
            a = line[i - bpp] if i >= bpp else 0
            b = prev[i]
            if ftype == 1:
                line[i] = (line[i] + a) & 255
            elif ftype == 2:
                line[i] = (line[i] + b) & 255
            elif ftype == 3:
                line[i] = (line[i] + ((a + b) >> 1)) & 255
            elif ftype == 4:
                c = prev[i - bpp] if i >= bpp else 0
                p = a + b - c
                pa, pb, pc = abs(p - a), abs(p - b), abs(p - c)
                pr = a if pa <= pb and pa <= pc else (b if pb <= pc else c)
                line[i] = (line[i] + pr) & 255
        prev = line
        if y >= first:
            for x in range(0, stride, bpp):
                total += 1
                if line[x] < 235 or line[x + 1] < 235 or line[x + 2] < 235:
                    inked += 1
    return inked / total if total else 0.0


def main():
    if len(sys.argv) == 5 and sys.argv[2] == "inked":
        print(f"{inked_ratio(sys.argv[1], float(sys.argv[3]), float(sys.argv[4])):.4f}")
        return 0
    if len(sys.argv) == 3 and sys.argv[2] == "texts":
        texts(sys.argv[1])
        return 0
    if len(sys.argv) != 4 or sys.argv[2] not in ("find", "has"):
        print(__doc__, file=sys.stderr)
        return 2
    try:
        c = find(sys.argv[1], sys.argv[3])
    except ET.ParseError:
        return 1  # ダンプの途中・空（画面の遷移中）。呼び出し側が再試行する
    if c is None:
        return 1
    if sys.argv[2] == "find":
        print(c[0], c[1])
    return 0


if __name__ == "__main__":
    sys.exit(main())
