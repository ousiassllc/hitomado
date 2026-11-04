// Package androidbg は Android の WorkManager の Worker（Java の HeadlessSync）から Go のヘッドレス同期
// （internal/headless）を呼ぶ JNI の入口。cgo が使えるビルド（libwails.so。jni_android.go）だけに実体がある。
//
// Wails の Java ブリッジ（WailsBridge）は Activity から作られ、Go の Wails アプリが起動してからでないと Go へ届かない。
// Worker はプロセスを起こされただけで、Activity も Wails アプリも無いことがあるため、Wails を通さず libwails.so の
// JNI 関数を直接呼ぶ（Go のランタイムは libwails.so を System.loadLibrary した時点で始まり、main は動かさない）。
// 資格情報は、Worker が渡す Java のオブジェクト（SecretBridge。SecureStore を使う）を JNI で呼び戻して読む。
package androidbg
