#!/usr/bin/env python3
"""UI Automator の画面ダンプ（uiautomator dump の XML）から、テキスト・content-desc で要素を探す（E2E 用。標準ライブラリのみ）。

使い方: ui.py <dump.xml> find <部分一致の文字列>   → 見つかれば中心座標 "x y" を出して 0、無ければ 1
        ui.py <dump.xml> has <部分一致の文字列>    → 見つかれば 0、無ければ 1
        ui.py <dump.xml> texts                     → ノードの数と、text・content-desc を持つノードの一覧（診断用）。常に 0
text・content-desc のどちらかに部分一致する、最初の（クリックの対象になる）ノードを使う。
"""
import re
import sys
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


def main():
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
