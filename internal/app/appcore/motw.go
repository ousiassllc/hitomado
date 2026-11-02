package appcore

// MarkInternetZone は保存したファイルを「インターネットから取得」とマークする（Windows の代替データストリーム
// `<path>:Zone.Identifier`。開くとき SmartScreen／保護ビューが働く）。Windows 以外では何もしない。
// 失敗（FAT/exFAT など ADS 非対応のボリューム）は呼び出し側が保存の失敗にしないこと。
// 差し替えられる変数にしているのはテストのため。
var MarkInternetZone = markInternetZone
