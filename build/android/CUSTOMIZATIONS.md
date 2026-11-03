# build/android のテンプレートからの変更点

`build/android/` は Wails v3 の Android テンプレートのコピーで、このリポジトリで管理している。Wails を上げてテンプレートを取り込み直すとき、ここに書いた変更を戻し忘れないよう、**テンプレートから変えた点を 1 行ずつ**記録する（Issue 番号つき）。他の Issue でも、テンプレートのファイル（Java・Gradle・manifest・Taskfile など）を変えたらここへ追記する。

- `app/src/main/java/com/wails/app/SecureStore.java`（新規）: 資格情報の保管を Activity に依存しない static なシングルトンに切り出した。Keystore の鍵の喪失を検出してストアを作り直し、Go へ `key_lost` を返す（#84）
- `app/src/main/java/com/wails/app/WailsBridge.java`: `secureSet`／`secureGet`／`secureDelete` を `SecureStore` への委譲に置き換えた。`securePrefs()`・`cachedSecurePrefs` と `EncryptedSharedPreferences`／`MasterKey`／`SharedPreferences` の import を削除。失敗は `{"ok":false,"error":"<key_lost|unavailable|failed|empty key>"}` だけを返し、値・例外メッセージを含めない（#84）
- `app/src/main/java/com/wails/app/WailsBridge.java`: `handleMessage`／`handleRuntimeCall` のデバッグログから JS のペイロード本文を外し、長さだけにした（引数に API キーなどが含まれうるため）（#84）
- `app/build.gradle`: `androidx.security:security-crypto` を `1.1.0-alpha06` から安定版 `1.1.0` に変更（#84）
- `app/build.gradle`: `testImplementation 'junit:junit:4.13.2'` と `testOptions.unitTests.returnDefaultValues = true` を追加し、`app/src/test/java/com/wails/app/SecureStoreTest.java`（新規）で `SecureStore` を JVM 単体テストする（#84）
- `app/src/main/java/com/wails/app/WailsJSBridge.java`: `invoke`／`invokeAsync` のデバッグログから JS のペイロード本文を外し、長さだけにした（バインディング呼び出しの引数に API キー・Client Secret が含まれるため）（#84）
- `Taskfile.yml`: `test` タスク（`./gradlew testDebugUnitTest`）を追加。CI の android ジョブが `wails3 task android:test` で呼ぶ（#84）
- `Taskfile.yml`: `compile:go:shared` の `ARCH=amd64` だけ `build/android/scripts/libc-overlay.go` を実行して `go build -modfile=…`（パッチ済みの `modernc.org/libc`。`build/android/libc/amd64.patch`）で作る。`clean` が `build/android/gen` も消す（#83。[docs/environment/android.md §12](../../docs/environment/android.md)）
