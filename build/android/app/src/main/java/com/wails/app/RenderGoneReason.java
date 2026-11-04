package com.wails.app;

/**
 * WebView のレンダラが消えた理由のログ文言（#90）。クラッシュと、メモリ不足などで OS に止められた場合を分ける
 * （RenderProcessGoneDetail.didCrash() が true ならクラッシュ、false なら OS が止めた＝ほとんどはメモリ不足）。
 */
final class RenderGoneReason {
    private RenderGoneReason() {}

    static String describe(boolean didCrash) {
        return didCrash
                ? "WebView renderer crashed"
                : "WebView renderer was killed by the system (likely low memory)";
    }
}
