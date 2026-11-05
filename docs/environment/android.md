# Android ビルド

> 関連: [環境構築](setup.md) / [プロジェクト概要](../overview.md) / [アーキテクチャ設計](../architecture/overview.md)

本書は Android 版（`jp.ousias.hitomado`）の APK／AAB をビルドする手順と、ビルド基盤の方針（SDK の版・署名・版番号・バインディング・manifest・CI）を定める（Issue #82）。Wails v3 の Android テンプレート（`build/android/`）をもとにしている。以降の節で、Android で動かさない機能（§10・#89）、資格情報の保管（§11・#84）、`modernc.org/libc` の android/amd64 問題（§12・#83）、添付・ログの保存先（§13・#85）、Google OAuth のリダイレクト方式（§14・#86）、バックグラウンド同期と新着通知（§15・#88）、スマホ向けの画面（§16・#87）、E2E・WebView のクラッシュ対策・性能（§17・#90）、AI の既定のプロバイダと ChatGPT ログイン（§18・#104）を決めた。 試作と調査（#81）の結果表・作業の一覧は [android-investigation.md](android-investigation.md) にまとめている。

## 1. 必要なもの

| もの | 版 | 備考 |
|------|----|------|
| JDK | 17（Temurin） | Gradle・Android Gradle Plugin（AGP）用。CI は `actions/setup-java`（temurin 17） |
| Android SDK | platform `android-36`、build-tools `36.0.0`（`aapt2`・`apksigner`） | compileSdk／targetSdk が 36（§3）。cmdline-tools の `sdkmanager` で入れる |
| NDK | `26.3.11579264` | `build/android/Taskfile.yml` の `NDK_VERSION` と CI の `ANDROID_NDK_VERSION` が同じ値。`ANDROID_NDK_HOME` があればそれを使い、無ければ `$ANDROID_HOME/ndk/<この版>` |
| Go | `go.mod` の版 | CGO で `libwails.so`（c-shared）を作る。NDK の clang を使う |
| `wails3` | `ci.yml` の `WAILS_VERSION`（v3.1.0） | overlay の生成（`wails3 android overlay:gen`）に使う |
| `task`、`bun`、`adb`（platform-tools） | — | `adb` は実機へ入れるときだけ |

環境変数: `ANDROID_HOME`（または `ANDROID_SDK_ROOT`）、`JAVA_HOME`、`ANDROID_NDK_HOME`（任意）。ホストは Linux（動作を確認したのは Linux x86_64 のみ）。Taskfile は macOS（`darwin-x86_64` のツールチェーン）も想定しているが未確認。Windows ホストでの Android ビルドは対象外（WSL を使う）。

SDK の入れ方の例:

```bash
sdkmanager "platforms;android-36" "build-tools;36.0.0" "ndk;26.3.11579264" "platform-tools"
```

## 2. タスク

`VERSION=X.Y.Z`（§5）は省略すると `0.0.0`。`ARCH` の既定は `arm64`（実機は arm64）。成果物は `bin/`（git 管理外）。

| コマンド | 内容 | 出力 |
|----------|------|------|
| `task android:build [ARCH=arm64\|amd64]` | フロントをビルドし、`libwails.so` を作る（Go のみ。APK にはしない）。ほかの ABI の古い `libwails.so` は先に消す | `build/android/app/src/main/jniLibs/<abi>/libwails.so` |
| `task android:package [ARCH=arm64\|amd64] VERSION=X.Y.Z` | release APK（署名あり、§4）。`ARCH` の ABI だけを含む | `bin/hitomado.apk` |
| `task android:package:fat VERSION=X.Y.Z` | arm64＋x86_64 を含む release APK | `bin/hitomado.apk` |
| `task android:bundle [ARCH=…] VERSION=X.Y.Z` | release AAB（単一 ABI） | `bin/hitomado.aab` |
| `task android:bundle:fat VERSION=X.Y.Z` | release AAB（arm64＋x86_64。Play へ上げる形） | `bin/hitomado.aab` |
| `task android:verify [APK=<path>] VERSION=X.Y.Z` | APK を検査する（`build/android/scripts/verify-apk.sh`。§6） | — |
| `task android:install [DEVICE_ID=<serial>] [APK=<path>]` | `adb install -r` で実機へ入れる（既定は `bin/hitomado.apk`） | — |
| `task android:run` / `android:run:device` | debug ビルドをエミュレーター／実機へ入れて起動する（`bin/hitomado-debug.apk`） | — |
| `task android:test` | Java の単体テスト（`build/android/app/src/test`。JVM のみで端末は不要。§11） | — |
| `task android:lint` | android タグ側の Go lint（§8） | — |
| `task android:e2e:apk [ARCH=…]` / `android:e2e:run` | E2E 用 debug APK（ビルドタグ `e2e`。シード済みのフェイク Gmail）を作る／接続中の端末で E2E を回す（§17.3）。release には使わない | `bin/hitomado-e2e.apk` |
| `task android:clean` | 生成物を消す | — |

`task` の代わりに `wails3 task …` でも同じ。`package:fat`・`bundle:fat` の最初の ABI は `arm64`、2 つ目が `amd64`（x86_64）。

実機へ入れる手順:

```bash
task android:package ARCH=arm64 VERSION=0.1.0     # bin/hitomado.apk（署名は §4）
adb devices                                       # 端末の USB デバッグを有効にして接続
task android:install                              # = adb install -r bin/hitomado.apk
```

署名の鍵が違う同名のアプリが入っていると `INSTALL_FAILED_UPDATE_INCOMPATIBLE` になる（デバッグ鍵と release 鍵の入れ替えなど）。`adb uninstall jp.ousias.hitomado` で消してから入れる（**アプリのデータも消える**）。

## 3. compileSdk／targetSdk を 36 に揃える理由

Wails v3.1.0 のテンプレートは `compileSdk = 36`・`targetSdk = 36`（`app/build.gradle`）で、`build/android/Taskfile.yml` の `TARGET_SDK: '35'` と食い違っていた。この Taskfile の `TARGET_SDK` はどこからも使われていなかったため削除し、**`app/build.gradle` の 1 か所（36）に決める**。

- Google Play は **2026-08-31 以降、新規アプリも更新も targetSdk 36（Android 16）以上を要求する**（[Target API level requirements for Google Play apps](https://support.google.com/googleplay/android-developer/answer/11926878)）。35 のままだと配布の直前に上げ直すことになる。
- テンプレート（Wails の Java ブリッジ・androidx の依存）が 36 で書かれており、35 へ下げる利点が無い。
- 36 では、エッジ・ツー・エッジの強制など targetSdk 36 の挙動変更が入る。UI への影響は §16（スマホ向けの画面。safe area は Activity が insets を当てる）で扱う。

`minSdk` は **31（Android 12）**。2026-11-04 に悠太さんが決め、テンプレートの既定（21）から上げた。`Taskfile.yml` の `MIN_SDK` は NDK の clang の API レベル（`aarch64-linux-android31-clang`）で、`app/build.gradle` の `minSdk` と同じ値にしておく（NDK 26.3 に `aarch64-linux-android31-clang`／`x86_64-linux-android31-clang` がある）。理由:

- 最低対応を Android 12（API 31）以上にするという悠太さんの決定（2026-11-04）。
- 暗号化ストア（§11。EncryptedSharedPreferences）は API 23 以上を要し、minSdk 21 では API 21〜22 で資格情報を保存できない矛盾があった。31 ならこの矛盾が無く、API 23 未満向けの分岐・失敗コード（`unavailable`）も要らない。
- 対応する Android の主版を 12 以上に絞ることで、挙動の比較・実機確認の範囲が小さくなる（同じメジャー版の系列で比べられる。API 26 未満の通知チャネルの分岐なども不要になる）。

自前のコードの API 31 未満向けの分岐は外した。Wails のテンプレート由来の `SDK_INT` の分岐（`WailsBridge.java` など）は、テンプレートとの差分を増やさないため残している（`build/android/CUSTOMIZATIONS.md`）。

## 4. 署名

署名鍵（keystore）はリポジトリに入れない（`.gitignore` の対象外のファイルは追加しない。鍵はリポジトリの外に置く）。release ビルド（`android:package*`・`android:bundle*`）の署名は環境変数で決まる。

| 環境変数 | 内容 |
|----------|------|
| `ANDROID_KEYSTORE_FILE` | keystore のパス |
| `ANDROID_KEY_ALIAS` | 鍵のエイリアス |
| `ANDROID_KEYSTORE_PASSWORD` | keystore のパスワード |
| `ANDROID_KEY_PASSWORD` | 鍵のパスワード |

- **4 つすべて設定したとき**: その鍵で署名する（`Signing: RELEASE key …` と表示）。`ANDROID_KEYSTORE_FILE` を設定して残りが欠けている、またはファイルが無いときは、デバッグ鍵に黙って落とさず**ビルドを失敗させる**。
- **`ANDROID_KEYSTORE_FILE` が未設定のとき**: ローカルの実機へ入れるために、Android の**デバッグ鍵**（`~/.android/debug.keystore`。Gradle が無ければ作る）で署名する。このとき `WARNING: Signing: DEBUG key …` を表示する。これは release 署名ではなく、Play には上げられない（Play は debug 署名の AAB を拒否する）。`android:verify` も `SIGNING: DEBUG key` と表示する。
- パスワードの渡し方: Gradle は環境変数（`System.getenv`）から読む。`-Pandroid.injected.signing.store.password=…` のように **`-P`／コマンドライン引数に載せない**（`ps` やビルドログに出る）。`task`・Gradle のログにパスワードは出ない（`signing:notice` は鍵のエイリアスしか出さない）。Gradle デーモンは環境変数を引数にしない。
- Play App Signing を使う場合、ここで設定する鍵は**アップロード鍵**になる（Google が検証し、配布用の鍵で再署名する）。アップロード鍵と、ローカル用のデバッグ鍵を混同しない。release 用の鍵は別に作って保管する（失うと更新できない）。
- 鍵の作り方（例）: `keytool -genkeypair -keystore ~/hitomado-upload.jks -alias hitomado -keyalg RSA -keysize 2048 -validity 10000`

`task android:run`／`android:run:device` は debug ビルド（`assembleDebug`。debug 鍵、`bin/hitomado-debug.apk`）。

## 5. 版番号

`VERSION=X.Y.Z` 1 つから決める（`-X main.version`、Windows の `APP_VERSION` と同じ規則・同じ入力）。先頭に `v` を付けない（タグ `vX.Y.Z` から作るときは `v` を外す）。X.Y.Z 以外は失敗する。`VERSION` を渡さないときは `0.0.0`（ルート `Taskfile.yml` の `includes` の `android.vars.APP_VERSION`。`windows` と同じ定義の仕方で、`build/config.yml` の `0.0.0` と一致）。

| 値 | 決め方 |
|----|--------|
| `versionName` | `X.Y.Z` そのまま |
| `versionCode` | `X * 10,000,000 + Y * 10,000 + Z`（`0.0.0` は `1`）。例: `0.1.0` → `10000`、`0.1.7` → `10007`、`1.20.3` → `10200003` |

- 範囲: X ≤ 209、Y ≤ 999、Z ≤ 9999（Play の上限 2,100,000,000 に収まる）。超えるとビルドを失敗させる。
- 単調増加: 版が上がる（X.Y.Z の辞書式の大小）と必ず `versionCode` も増える。リリースは `release.yml` が patch を 1 つずつ上げるため、最新タグの versionCode より大きくなる。同じ版を作り直した APK は同じ `versionCode` になる（Play には同じ `versionCode` を二度上げられない。再アップロードが必要なら版を上げる）。
- 計算は `build/android/app/build.gradle` の先頭（`appVersionCode`）。Taskfile は `-PappVersion=X.Y.Z` を渡すだけ（秘密ではない値だけをコマンドラインに載せる）。
- 注入先: `libwails.so`（`-ldflags -X main.version=X.Y.Z`。release のみ）と `AndroidManifest.xml`（versionName／versionCode）。`0.0.0` の版は更新確認をしない（`main.go`・[環境構築 §4.3](setup.md#43-windows-インストーラー)）。

## 6. manifest と検査

`build/android/app/src/main/AndroidManifest.xml`:

- 置く権限: `INTERNET`、`ACCESS_NETWORK_STATE`（ネットワーク変化の通知）、`VIBRATE`、`POST_NOTIFICATIONS`（§15）。WorkManager が manifest のマージで `WAKE_LOCK`・`RECEIVE_BOOT_COMPLETED` を足す（許可リストに入っている）。前面サービスの権限（`FOREGROUND_SERVICE`・`FOREGROUND_SERVICE_DATA_SYNC`）と `WailsForegroundService` は #88 で削除した（§15.1・§15.7）。
- 削除した権限: `CAMERA`、`ACCESS_FINE_LOCATION`、`ACCESS_COARSE_LOCATION`、`USE_BIOMETRIC`。`androidx.biometric`（`WailsBridge.java` が参照する）がマージ時に `USE_BIOMETRIC`／`USE_FINGERPRINT` を足すため、`tools:node="remove"` で取り除く。カメラ用の `<queries>` も削除した。
- `allowBackup="false"`、`fullBackupContent="false"`、`dataExtractionRules`（`res/xml/data_extraction_rules.xml`。クラウドバックアップ・端末間移行ともすべて除外）。DB（メール本文のメタデータを含む）と、Android Keystore の鍵に紐づく暗号化済み資格情報は、別の端末に復元しても使えないため。

`task android:verify` は `aapt2 dump badging`／`dump xmltree` と `apksigner verify --print-certs` で、packageName、versionName、versionCode、minSdk（31）、targetSdk（36）、権限が**許可リスト**に収まること（新しい権限が入ると失敗する。意図して増やすときは `verify-apk.sh` の許可リストと manifest を一緒に直す）、`allowBackup=false`、`dataExtractionRules` の有無、debuggable でないこと、`extractNativeLibs=true` と `libwails.so`・`classes.dex` が圧縮（Deflate）で入っていること（§6.1）、署名の種類（`EXPECT_SIGNING=debug|release` を付けるとその種類でなければ失敗）を確認する。

AAB の検査には `bundletool` が要る（SDK に含まれない）。`https://github.com/google/bundletool/releases` の `bundletool-all-<版>.jar`（CI は 1.18.3）を取得して `java -jar bundletool-all-1.18.3.jar validate --bundle bin/hitomado.aab` を実行する。`dump manifest --bundle` で manifest を見られる。

### 6.1 `.so` と dex を圧縮して格納する（サイズ優先）

`app/build.gradle` の `android.packaging` で `jniLibs.useLegacyPackaging = true` と `dex.useLegacyPackaging = true` にしている。minSdk が 28 以上のときの AGP の既定は **`.so` も dex も無圧縮（Stored）** で、Go 製の `libwails.so`（約 32MB）がそのまま APK に入る。

| | 無圧縮（AGP の既定） | 圧縮（本アプリ） |
|---|---|---|
| release APK（arm64。`task android:package ARCH=arm64 VERSION=0.1.0`） | 45,838,252 バイト | 17,413,116 バイト |
| `libwails.so` | Stored（32,026,544） | Defl:N |
| `classes.dex`・`classes2.dex` | Stored | Defl:N |
| 端末での `.so` | APK から直接 `mmap`（展開しない。`extractNativeLibs=false` 相当） | インストール時に展開（manifest に `android:extractNativeLibs="true"`） |

（表の無圧縮の行は work-runtime 2.10.5 のとき。圧縮の行は 2.12.0（§15.4）のときで、依存の差の分も含む。）

トレードオフ:

- **得るもの**: APK のダウンロード・保存サイズが約 1/2.6。AAB から Play が作る base の APK（arm64）の合計も約 16.2MB（`bundletool get-size total`）。
- **失うもの**: `extractNativeLibs="true"` により、インストール時（と更新時）に端末が `.so` を `/data/app/…/lib/arm64` へ**展開する**ので、端末の保存領域を APK の圧縮版と展開版の両方で使う（`libwails.so` の展開後は約 32MB）。無圧縮なら APK から直接 `mmap` して展開が要らない。インストールの時間もわずかに伸びる。dex の圧縮は、起動時に ART が展開する分の手間があるが、dex はコンパイル済みの oat に変換されるため実行時の影響は小さい。
- Android 12 以上でも動く（`extractNativeLibs=true` は `System.loadLibrary`（Wails の `libwails.so` の読み込み）を含め、従来どおりのパスで読まれる。`task android:verify` が manifest の値を確認する）。
- AAB（`bundle:fat`）: 生成される base の split APK の manifest も `extractNativeLibs=true` で、`libwails.so` は Deflate で入る（`bundletool build-apks` の結果で確認）。`bundletool validate` も通る。
- 無圧縮に戻すときは、この 2 つの `useLegacyPackaging` を外す（`android:verify` の検査も外す）。

## 7. バインディングと生成物の方針

`wails3 generate bindings -tags android` は、Android で通知サービスを登録しない（`notifsvc_android.go`）ため `frontend/bindings/…/services/notifications/*` を消す。`task check` と CI の「Generated files are up to date」（`wails3 generate bindings -f '-tags purego'` → `git diff --exit-code`）を壊さないよう、**バインディングはデフォルトのタグ（`purego`）でしか生成しない**。

- Android のビルド（`android:*`）はバインディングを再生成しない。テンプレートの `generate:android:bindings` タスクは削除し、`android:build` のフロントのビルド（`build:frontend`）は共通の `common:build:frontend`（バインディングを再生成する）を使わず、`bun run build` だけを実行する。
- フロントは通知サービスのバインディングを参照していない（`frontend/src` に参照なし）。Android で通知サービスが無くてもフロントのビルドは通る。
- 生成物（`frontend/bindings`）を変えるのは `task generate`（既定のタグ）だけ。Android 用の生成物は git 管理に入れない。

## 8. CI・リリース（`ci.yml`・`release.yml`）

- `lint` に `Go lint (android)` を追加（`GOOS=android GOARCH=arm64 CGO_ENABLED=0 golangci-lint run`）。`*_android.go`・`!android` 側の型検査と lint が対象になる。手元では `task android:lint`（`build/android/gen` は overlay の生成物で単体では型検査に失敗するため、先に消してから実行し、次の `android:build` が作り直す）。
- ビルド環境と release APK の作り方は、`.github/actions/` の composite action に切り出して `ci.yml` と `release.yml` で共有する（Windows の `build-installer` と同じ流儀）。
  - `setup-android`: Go・bun・JDK 17・wails3（`WAILS_VERSION`）・SDK platform 36・build-tools・NDK（`sdkmanager`）・フロントの依存。呼び出し元の `env` に `WAILS_VERSION`・`ANDROID_NDK_VERSION`・`ANDROID_BUILD_TOOLS_VERSION` が要る（NDK は `build/android/Taskfile.yml` の `NDK_VERSION` と同じ値）。
  - `build-android-apk`（入力 `version`・`output`）: `keytool` で**使い捨ての鍵**を作り（パスワードは `::add-mask::` で伏せ、ジョブの中だけに存在し、成果物には含めない）、環境変数で渡して `android:package ARCH=arm64 VERSION=…` → `EXPECT_SIGNING=release android:verify` を通した APK を `output` へ置く。
- `ci.yml` の `android` ジョブ（ubuntu-latest、**pull_request のみ**。`release.yml` が呼ぶとき（push）は動かさず、リリース用の APK は `release.yml` の `android` ジョブが同じ action で作る）: `setup-android` のあと、SecureStore の JVM 単体テスト、デバッグ鍵（フォールバック）の APK の検査、`build-android-apk`（版 `0.99.1`）、AAB（arm64＋x86_64）の作成と `bundletool validate` を行い、`hitomado-android` のアーティファクトに上げる。
- `release.yml` の `android` ジョブ（ubuntu-latest。`needs: [version, ci]`）: `setup-android` → `build-android-apk`（版はタグから `v` を除いた値）で `hitomado-vX.Y.Z-android-arm64.apk` を作り、`publish` が Release に添付して `SHA256SUMS.txt` に載せる。**署名は使い捨ての鍵**（実鍵は使わない）なので、版ごとに署名が変わり、上書きインストールはできない（更新は `adb uninstall jp.ousias.hitomado` → 入れ直し。アプリのデータは消える）。Google Play では配布しない。arm64 だけで、AAB・x86_64 は Release に添付しない。この APK のビルドが落ちると `publish` が動かず、Release は作られない。
- Gradle のキャッシュは明示していない（初回は Gradle 本体の取得を含む）。
- `android-e2e.yml`（**workflow_dispatch のみ**。`ci.yml` には入れていない）: KVM つきのエミュレーターでの E2E（§17.4）。

## 9. 既知の事項

- **x86_64（`ARCH=amd64`）は、パッチ済みの `modernc.org/libc` でビルドする**（§12）。パッチなしだと SQLite の最初のファイル操作で `SIGSYS` で落ちる。arm64（実機）はこの問題の対象外で、アプリ内の SQLite の実行は**実機で未確認**（§12.3）。
- Android の最小実装（アプリが起動してクラッシュしないための土台）: `dataDir` は `getFilesDir()` 配下（`datadir_android.go`。保存先は §13）、資格情報は EncryptedSharedPreferences（`internal/secrets/keyring_android.go`・`SecureStore.java`。§11）、通知サービスは未登録で、新着は Java の `NewMailNotifier`（`internal/platform/toast_android.go`。§15）、保存ダイアログは Storage Access Framework（§13）。
- `build/android/` は Wails のテンプレートを取り込んだもので、`.linterly.yml` の対象外。Gradle は `build/android/gradlew`（Gradle 9.2.1）を使う。
- `build/android/overlay.json`・`build/android/gen/`・`jniLibs/*/libwails.so` などの生成物は git 管理外（`.gitignore`）。

## 10. Android で動かさない機能

Android 版はタッチ前提の画面で、Releases の APK は検証用（Google Play では配布しない。§8）のため、Android では次の機能を**コード上も動かさず、設定画面にも出さない**（Issue #89。Windows 版の動きは変えない）。

| 機能 | Android | どう止めるか |
|------|---------|--------------|
| MCP サーバー（[docs/mcp.md](../mcp.md)） | 動かさない | `MCPService.Start` が待ち受けない（DB に PC で保存した「オン」の設定があっても）。`MCPService` はバインディングへ登録しない。設定画面の MCP タブを出さない |
| システムトレイ・× でトレイへ格納 | 作らない | `run.go` が `platform.NewTray` を呼ばない。ウィンドウの閉じるフックも付けない（`platform.NewMainWindow`）。通知のバッジ更新は何もしない（`Notifier` のトレイは `nil`） |
| 多重起動の防止 | 設定しない | `application.Options.SingleInstance` を `nil` にする（`singleInstanceOptions`）。Android の起動は OS が管理する |
| ウィンドウの位置・サイズ・最大化の保存復元 | 保存も復元もしない | `WindowKeeper` を作らない（`window.state` を読まず、書かない） |
| アプリ内の自動更新（定期確認・手動確認・自動アップデート） | 動かさない | 更新確認ループを起動しない。`update.Manager`（自動アップデート）も作らない。`AppService` は保存値にかかわらず無効を返し、`CheckForUpdate` は通信しない。`SetUpdateCheckEnabled` は `UNSUPPORTED`。設定画面の更新確認の項目を出さない |

### 仕組み（機能ベース）

- 機能の有無は `internal/capability` の `Capabilities`（`SupportsMCP`・`SupportsTray`・`SupportsAutoUpdate`・`SupportsWindowState`・`SingleInstance`）で表す。値を決めるのは、ビルドタグで分けた薄い 2 ファイル（`current_android.go`＝すべて無効、`current_other.go`＝すべて有効）だけで、判定の中身はタグなしの純粋な関数・値（`Desktop`／`Android`）なので、ホストでテストできる（Android のビルドはホストで実行できない）。
- `run.go` が `capability.Current()` を 1 回だけ読み、`app.Deps.Capabilities` と各部品の生成へ渡す。`Deps.Capabilities` のゼロ値は全機能が無効（設定し忘れて機能が動いてしまうことを避ける）。
- フロントへは `PlatformService.GetCapabilities`（[バインディング仕様 §7.3](../api/endpoints.md#73-platformservice)）で渡す。画面は OS 名を比べず、`platformStore`（`frontend/src/stores/platform.ts`）の `supportsMcp` などで出し分ける（スマホ向けの画面 §16 も `touchUi` で切り替える）。読み込み前・失敗時はデスクトップ（全機能あり）の値。
- **バインディングは OS で変えない**（§7）。`PlatformService` は全 OS で同じシグネチャで、値だけが変わる。MCP は登録の有無だけが変わるが、型は同じで、バインディングは既定のタグ（`purego`）で 1 つだけ生成する。
- 通知・バックグラウンド同期は §15、添付・ログの保存先は §13。ここでは対象外。

## 11. 資格情報の保管（#84）

リフレッシュトークン・OAuth クライアントの Client Secret・AI の API キー・ChatGPT ログインのトークン・MCP トークンは、Android では **EncryptedSharedPreferences**（鍵は Android Keystore）にだけ保存する。SQLite・ログ・logcat には入れない（NF-S01）。Windows の動き（`internal/secrets/keyring.go`。`!android`）は変えない。

### 11.1 構成

```
Go の secrets.Store（internal/secrets/bridge.go。純 Go。鍵の喪失を ErrKeyLost へ）
  → Wails の application.Android.SecureGet/SecureSet/SecureDelete（JNI）
  → WailsBridge.secureGet/secureSet/secureDelete（Java。委譲だけ）
  → SecureStore（Java。applicationContext だけを使う static なシングルトン。排他・鍵の喪失の検出と復旧）
  → EncryptedSharedPreferences
```

| 項目 | 値 |
|------|----|
| 保存先（SharedPreferences ファイル） | `hitomado_secrets`。Wails の既定（`wails_secure`）とは分ける。試作（#81）の保存分は引き継がない（試作はエミュレーターの検証のみで、本番のデータは無い） |
| Keystore の鍵の別名 | `hitomado_secrets_master_key`（AES256-GCM）。androidx の既定の別名とは分ける |
| キーの暗号化／値の暗号化 | `AES256_SIV`／`AES256_GCM`（security-crypto の標準） |
| 名前空間 | キー名は Windows と同じ `hitomado/<種別>/<ID>`（[ER §1](../architecture/er.md)）。保存先のファイルが専用なので、Windows の「サービス名 `Hitomado`」に当たる区切りは別にいらない |
| 値の大きさ | SharedPreferences に Windows 資格情報マネージャーのような 2.5KB の上限は無い。4096 バイトを超える値（ChatGPT の JWT など）もそのまま往復できる（`bridge_test.go`・`SecureStoreTest`）。`internal/siwc` の分割保存は上限が無くても動く |
| API レベル | minSdk は 31（§3）。暗号化ストア（EncryptedSharedPreferences・Android Keystore の AES256-GCM）が要る API 23 を常に満たすので、「暗号化ストアを使えない」失敗コードも分岐も無い（資格情報を平文で置くフォールバックも作らない） |
| 削除 | 無いキーの削除はエラーにしない（`secrets.Store` の契約） |

### 11.2 security-crypto の版（alpha06 → 1.1.0）

| 版 | 状態 | 内容 |
|----|------|------|
| 1.0.0（2021-04） | 安定。**minSdk 23** | `MasterKey` クラスが無い（`MasterKeys`〔deprecated〕のみ）。minSdk 31 のこのアプリでは minSdk は問題にならないが、`MasterKey` が無い |
| 1.1.0-alpha06（2023-04。旧） | alpha | `MasterKey` あり、minSdk 21。Tink 1.8.0 |
| 1.1.0-alpha07／beta01（2025） | — | **全 API を deprecated にしただけ**（機能の変更なし。プラットフォームの API と Android Keystore の直接利用へ移行する方針） |
| **1.1.0（2025-07-30）** | **安定（採用）** | alpha07 以降と同じ。minSdk 21、`MasterKey` あり |

- **1.1.0 を採用する**。alpha を外せること、`MasterKey` があること（1.0.0 には無い。`MasterKeys`〔deprecated〕のみ）、保存形式が alpha06 と同じで互換が切れないことが理由。minSdk は 31 なので、1.0.0 の minSdk 23 は障害にならない（残る理由は `MasterKey` の有無）。出典: [AndroidX Security のリリースノート](https://developer.android.com/jetpack/androidx/releases/security)。
- **deprecated であることの扱い**: 1.1.0 は全 API が deprecated（置き換えは Android Keystore を直接使った AES-GCM）だが、安定版として公開済みで、ここでの使い方（`MasterKey` ＋ `EncryptedSharedPreferences.create`）は動く。`SecureStore` の本番の保存先（`AndroidBackend`）に `@SuppressWarnings("deprecation")` を付け、deprecated な API の使用をこの 1 クラスに閉じ込めてある。Keystore の直接利用へ移す場合も、`SecureStore.Backend` を差し替えるだけで済む（呼び出し側の Go・WailsBridge は変えない）。移行は、security-crypto に不具合・非互換が出たとき、または targetSdk の引き上げで動かなくなったときに行う。今は行わない（自前の暗号は検証の負担が大きい）。
- 版の更新は `build/android/app/build.gradle` の 1 行。

### 11.3 Keystore の鍵が失われたとき

バックアップ復元・端末の変更・画面ロックのリセット・鍵の無効化などで、保存済みの値を復号できなくなることがある。失敗は 2 つに分けて扱う（EncryptedSharedPreferences は復号の失敗を `SecurityException` などに包むため、どちらも `cause` の連鎖をたどって判定する）。

- **確実な鍵の喪失**: `InvalidKeyException`（`KeyPermanentlyInvalidatedException` を含む）／`AEADBadTagException`（`BadPaddingException`）／`UnrecoverableKeyException`、Tink のキーセットの破損（`InvalidProtocolBufferException`）。再試行せず、すぐにストアを作り直す。
- **鍵の喪失か Keystore の一時的な不調か区別できない失敗**: `java.security.KeyStoreException`／`ProviderException`／`android.security.KeyStoreException`。`isTransientFailure()` は API 33 以上でしか使えないため、API を問わず、間を置いて 2 回まで再試行する（最大 3 回の実行）。再試行で直ればストアは消さない。**3 回とも失敗したときだけ**鍵の喪失として作り直す（`isTransientFailure()` が真と分かるときは、続いても消さない）。バックグラウンドの同期 Worker が、Keystore の一時的な不調で全アカウントを黙って `needs_reauth` にしないための扱い。

| 層 | 動作 |
|----|------|
| Java（`SecureStore`） | 暗号化ストア（ファイルと Keystore の鍵）を消して作り直し、**起動は落とさない**。読み出しは `key_lost` を返す。書き込み・削除で見つけたときは、作り直したうえで続きを実行する（保存は 1 回だけ再試行）。Keystore が一時的に使えないだけの失敗（`KeyStoreException.isTransientFailure()` が真）や、ディスクの `IOException` だけの失敗は鍵の喪失とみなさず、ストアも消さない（`failed`。資格情報を消すのは確実なときと、再試行しても Keystore の失敗が続くときだけ） |
| Go（`secrets.Store`） | `key_lost` を `secrets.ErrKeyLost` にする。`ErrKeyLost` は `errors.Is(err, secrets.ErrNotFound)` も真になる。ストアは作り直されて空なので、2 回目以降は通常の `ErrNotFound` |
| アプリ | 既存の呼び出し側が `ErrNotFound` で扱う。**リフレッシュトークン**は `gmail.ErrNeedsReauth`（`internal/gmail/options.go`）→ アカウントが `needs_reauth` になり、受信箱の「要再接続」とアカウント設定の「再接続」へ案内される（F-01。新しい UI は無い）。**Client Secret** は未設定（`hasSecret=false`。「再接続」では OAuth クライアントの Secret の登録を案内する）。**API キー**は未設定。**ChatGPT** は未ログイン。**MCP トークン**は未発行（再発行する） |

`bridge_test.go`（Java を模した fake 越しに、往復・削除・大きな値・鍵の喪失・それ以外の失敗）と `internal/gmail/errors_test.go`（`TestKeyLostRefreshTokenIsNeedsReauth`）、`SecureStoreTest`（Java の分類と復旧）で確かめている。バックアップは `allowBackup=false`・`dataExtractionRules`（§6）で除外しているため、通常は端末の移行で鍵の喪失は起きない。それでも `adb restore` などで起きうるので備えている。

### 11.4 Activity なしで読む（#88 の WorkManager）

`SecureStore.getInstance(context)` は `applicationContext` だけを保持する（Activity を参照しない）。`WailsBridge` の `secureGet`／`secureSet`／`secureDelete`、WorkManager の `Worker`、JNI 経由の Go は、**同じ 1 つのインスタンス**（同じファイル・同じ Keystore の鍵）を使い、全メソッドが 1 つの monitor で直列化される（同時の読み書きで壊れない）。

```java
// Worker（Activity なし）での使い方
String refreshToken = SecureStore.getInstance(getApplicationContext()).get("hitomado/refresh_token/" + accountId);
// 無ければ null。鍵が失われていれば SecureStoreException（kind() == KEY_LOST。ストアは作り直し済み）→ 再ログインが必要として扱う。
```

- Wails の `SecureGet`（Go）は、`WailsBridge`（Activity から作る）の `secureGet` を呼ぶ。Go は `libwails.so` が読み込まれた同じプロセスでしか動かないため、Activity が無いままプロセスだけが残っているとき（Worker 起動時）は Go の `Store` ではなく、Java から `SecureStore` を直接使う。`WailsBridge` は `SecureStore.getInstance(activity)` で `getApplicationContext()` を使い、Activity が破棄されたあとも保管先は使える。
- Worker からの実際の呼び出しは #88 で入れた（§15.2。`SecretBridge` を Go が JNI で呼び戻す。エミュレーターで、Activity の無いプロセスの Worker が `found:false` を受け取り要再接続になるところまで確認した）。#84 の時点では、コード（`SecureStore` が `Context` を引数に取り Activity に依存しないこと）と JVM 単体テスト（`SecureStoreTest.concurrentUseFromBackgroundThreadsIsSerialized`。Activity・Context なしのスレッドから同じ `SecureStore` を並行に使える）で示す。Worker からの実際の呼び出しは #88 で入れる。

### 11.5 ログに出さない

- Go: `Store` のエラーは `secrets: <操作> "<キー名>": <Java の失敗コード>` だけで、値を含まない（キー名は `hitomado/<種別>/<ID>` で秘密ではない）。`slog` に値を渡している箇所は無い（`internal/` を grep して確認）。
- Java: `SecureStore`／`WailsBridge` の失敗コードは `key_lost`／`failed`／`empty key` だけで、例外のメッセージ（値を含みうる）は返さず、logcat にも例外のクラス名だけを出す。
- **Wails のテンプレートが debug ビルドで logcat に出していた JS からの呼び出しの本文を、長さだけに変えた**（`WailsBridge.handleMessage`／`handleRuntimeCall`、`WailsJSBridge.invoke`／`invokeAsync`）。バインディング呼び出しの引数（`SetAIApiKey` の API キー、`AddOAuthClient` の Client Secret）が本文に入るため。release ビルド（`BuildConfig.DEBUG` が偽）は元から出さない。`MainActivity` の `Wails API call`（パスとクエリだけ）、WebView のコンソールのメッセージ（debug のみ）は、フロントが `console.error(err)` を出す 1 か所（`errorReporter.ts`。AppError のメッセージで、値は含まない）を除き、値を出す箇所が無い。
- Go 側の Wails のフレームワークのログ（`[JS message]`、`runtime call returned status`）は `production` タグで無効になる（release のビルドは `-tags production,android`）。debug ビルド（`android:run`）では `[JS message]`（ウィンドウのメッセージ）が logcat に出る。

### 11.6 確認の手順（実機・エミュレーター）

自動で確かめたのは、Go と Java の単体テスト・ビルド・`android:verify` まで。次は実機・エミュレーターが要る（未確認）:

1. 往復: アカウントを接続（または設定で AI の API キーを登録）→ アプリを終了して再起動 → 接続・キーが残っている。`adb shell run-as jp.ousias.hitomado cat shared_prefs/hitomado_secrets.xml`（debug ビルド）に、トークンが**平文で無い**（キーも値も暗号化されている）。
2. ログ: `adb logcat -d | grep -i -E "token|ya29|1//|sk-"` でトークン・キーが出ない（debug ビルドで、接続・キー登録・同期のあと）。
3. 鍵の喪失: debug ビルドで接続 → `run-as` で `shared_prefs/hitomado_secrets.xml` を端末の外へ退避 → `adb shell pm clear jp.ousias.hitomado`（Keystore の鍵も消える）→ 一度起動して終了 → 退避したファイルを `run-as` で書き戻す → 起動。アカウントが「要再接続」になり、再接続で復旧する。logcat に `Keystore key lost; resetting secure storage (<例外クラス名>)` が出る。
4. Worker: Activity が無い状態の定期実行でトークンを読めること。#88 で入れた。ただしエミュレーターでは「値が無い」の経路（`found:false`）まで。実トークンを読んで同期できることは実機・本物の Google で未確認（§15.9）。

## 12. modernc.org/libc の android/amd64 問題（#83）

### 12.1 問題と上流の状況

`modernc.org/libc`（`modernc.org/sqlite` が使う C ランタイム）の `linux/amd64` 実装は、musl の C を ccgo で変換したもので、x86_64 にしか無い旧式のシステムコール番号（`lstat`、`access`、`mkdir`、`unlink`、`rename`、`pipe`、`dup2` ほか）を直接呼ぶ。Android のアプリには seccomp フィルターがかかっており、bionic の `SYSCALLS.TXT` と許可リストにある番号しか通さない。x86_64 では `*at` 系（`openat`、`newfstatat` …）だけが載っていて、旧式の番号のうち許可されているのは `open`・`stat`・`readlink` だけなので、それ以外を呼ぶとプロセスが `SIGSYS (SYS_SECCOMP)` で落ちる（実測: `syscall 6` ＝ `lstat`）。arm64 のカーネル ABI には旧式の番号が無く、`ccgo_linux_arm64.go` は `*at` 系だけを使う。

- 上流には**同じ問題の issue がある**: [cznic/libc#41](https://gitlab.com/cznic/libc/-/issues/41)（2024-08 に報告。「No feedback, closing.」で 2025-04 に閉じられた。修正はない）。
- **直っていない**: `go.mod` の v1.79.0 と、最新の v1.80.0（2026-11-01）・v1.79.1 のいずれも、`ccgo_linux_amd64.go` の旧式 syscall の呼び出し 15 か所が同じで、`syscall_musl.go` は v1.79.0 と v1.80.0 で同一（2026-11-03 に確認）。上流が直す見込みは薄い。
- 試作（#81）の `libc_linux_amd64.go` を書き換えたパッチは、そのファイルのビルドタグが `!(linux && amd64)` で amd64 では**使われない**ので効かない。効くのは `syscall_musl.go` 側（`___syscall_cp`・`X__syscall1`〜`3`）の読み替えである。ここのパッチはその形にしてある。
- 上流への報告文は下書きを [android-libc-upstream.md](android-libc-upstream.md) に置いた（**まだ送っていない**。外部サービスへの投稿はこの Issue の範囲外）。

### 12.2 x86_64 の扱い（決定）

**x86_64 をアプリとして残し、`modernc.org/libc` に小さなパッチを当てて android/amd64 でも動かす。** パッチは android/amd64 のビルドにだけ当たる。arm64・Linux・Windows のビルドとテストは変わらない。

| 案 | 判断 |
|----|------|
| (a) パッチ版 libc を `go.mod` の `replace` で全 OS に効かせる | 採らない。`replace` は全 OS・全ターゲットに効き、`go.sum`・NOTICE（依存のライセンス表記）・Windows ビルドの入力が変わる。 |
| **(a') パッチを `-modfile` の `replace` で android/amd64 のときだけ当てる**（採用） | `go.mod`・`go.sum` は変えない（`go mod tidy -diff`・NOTICE 生成の結果が同じ）。Windows・Linux・arm64 は上流の libc のまま。 |
| (b) x86_64 を対象外にして、AAB を arm64 だけにする | 採らない。コストは最も低いが、x86_64 の端末（Chromebook・x86_64 エミュレーター）で起動しなくなる。このホストで動かせる Android のエミュレーターは x86_64 だけ（`/dev/kvm` が使えず `-accel off`）なので、#89 などのエミュレーターでの確認ができなくなる。 |

採用理由（コストが低い）: パッチは libc の新ファイル 2 つ（android/amd64 の読み替え、それ以外のスタブ）と、`syscall_musl.go` の 4 関数の入口に足す 3 行ずつ（計約 140 行）だけ。libc の版を上げるとパッチの版チェックが**ビルドを失敗させる**ので、黙って古いパッチが当たることはない。CI の `android` ジョブは `bundle:fat`（amd64 を含む）をビルドするため、libc の更新（Renovate など）でパッチが当たらなくなると CI で気づく。

仕組み:

| 場所 | 内容 |
|------|------|
| `build/android/libc/amd64.patch` | libc v1.79.0 へのパッチ（`patch -p1`）。`legacy_android_amd64.go`（`//go:build android && amd64`。旧式の番号を `*at` 系に読み替える）、`legacy_other.go`（`!(android && amd64)`。何もしない）、`syscall_musl.go` の編集 |
| `build/android/scripts/libc-overlay.go` | libc をモジュールキャッシュから `build/android/gen/libc-amd64/libc` へ複製してパッチを当て、`go.mod`・`go.sum` の複製に `replace modernc.org/libc => …` を足した `build/android/gen/libc-amd64/go.mod` を作る。`go.mod` の libc の版が `libcVersion`（v1.79.0）と違えば失敗する |
| `build/android/Taskfile.yml` の `compile:go:shared` | `ARCH=amd64` のときだけ上のスクリプトを実行し、`go build -modfile=…/go.mod` で作る。`ARCH=arm64` では何もしない |

`go build -overlay` はモジュールキャッシュ内のファイルを置き換えられないため、`-modfile`（ビルドごとの別 `go.mod`）を使っている。生成物は `build/android/gen/`（git 管理外）の下だけ。

読み替える番号: `open`・`stat`・`lstat`・`access`・`pipe`・`dup2`・`rename`・`mkdir`・`rmdir`・`link`・`unlink`・`symlink`・`readlink`・`chmod`・`chown`・`lchown`・`mknod`（許可されている `open`・`stat`・`readlink` も、Android の版に依らず動くよう読み替える）。bionic の許可リストと照らして、残る未許可の番号（`poll`・`select`・`epoll_*`・`eventfd`・`inotify_init`・`fork`・`utimes`・`pause`・`signalfd`・`msg*`/`sem*`/`shm*` など）は SQLite が使わないので読み替えていない。ここに挙げたものをアプリのコードが `modernc.org/libc` 経由で呼ぶようになったら、パッチに足す。

**libc の版を上げるとき**: `go.mod` の libc を上げるとパッチの版チェックで `android:build ARCH=amd64` が失敗する。新しい版の `syscall_musl.go` に同じ編集をし直して `amd64.patch` を作り直し、`libc-overlay.go` の `libcVersion` を更新する。上流が直したら（§12.1 の issue）、パッチ・スクリプト・Taskfile の分岐ごと削除する。

### 12.3 検証した範囲（2026-11-03）

**x86_64（android/amd64）: アプリ内で確認した。** API 35（google_apis x86_64）のエミュレーター（`-accel off`、`hitomado-a83` 用 AVD・ポート 5584。確認後に停止）で、`task android:build ARCH=amd64` → `android:assemble:apk` の debug APK を入れて起動した。

| 場合 | 結果 |
|------|------|
| パッチあり（`task android:build ARCH=amd64`） | プロセスが生き続け、アプリ内に `files/Hitomado/hitomado.db`（と `-wal`・`-shm`）ができる。取り出して `sqlite3` で見ると、`goose_db_version` に 0・1 の 2 行が適用済み、テーブル 11 個（`accounts`・`threads`・`messages` ほか）、`PRAGMA integrity_check` は `ok`（`journal_mode=wal`）＝ `store.Open` のマイグレーションと書き込みがアプリ内で動いた |
| パッチなし（対照。`go build` に `-modfile` を付けない） | `Fatal signal 31 (SIGSYS), code 1 (SYS_SECCOMP), syscall 6`。`hitomado.db` はできない |

**arm64（実機の Android）: アプリ内の SQLite の実行は未確認。** 実機と KVM が無い。arm64 のエミュレーターは、このホスト（x86_64）では Android Emulator が拒否する（`FATAL | Avd's CPU Architecture 'arm64' is not supported by the QEMU2 emulator on x86_64 host. System image must match the host architecture.`。`-accel off` でも同じ。arm64 の system-image は取得済みだが起動できない）。したがって「arm64 のアプリ内で `hitomado.db` の作成・マイグレーション・読み書きができる」は**未確認**で、実機で確認する（§12.4）。

静的に確認できたこと（arm64）:

- `modernc.org/libc` v1.79.0 の `ccgo_linux_arm64.go` は、旧式の番号の定数（`SYS_open`・`SYS_stat`・`SYS_lstat`・`SYS_access`・`SYS_mkdir`・`SYS_unlink`・`SYS_rename`・`SYS_rmdir`・`SYS_pipe`・`SYS_dup2`・`SYS_readlink`・`SYS_link`・`SYS_symlink`・`SYS_chmod`・`SYS_chown`・`SYS_mknod`）を**1 つも定義していない**（amd64 の `ccgo_linux_amd64.go` はすべて定義している）。ファイル操作は `openat`（56）・`newfstatat`（79）・`mkdirat`（34）・`unlinkat`（35）・`faccessat`（48）・`renameat2`（276）を使う（`_fstatat_kstat`、`Xopen`、`mkdir` で確認）。
- 手書き側（`abort_linux.go` と `libc_musl_linux_arm64.go`）が直接呼ぶのは `rt_sigaction`・`rt_sigprocmask`・`renameat2` だけ。
- `X__syscall*` から呼ばれる番号（arm64、201 個）を bionic の許可リスト（`SYSCALLS.TXT`＋`SECCOMP_ALLOWLIST_*`）と照合すると、許可リストに無いのは `faccessat2`・`fchmodat2`・`fanotify_*`・`msg*`/`sem*`/`shm*`・`name_to_handle_at`・`open_by_handle_at`・`remap_file_pages`・`vhangup` だけで、SQLite のファイル・ロック・mmap・スレッドの経路は使わない（musl の `faccessat` はフラグ 0 なら `faccessat` を使う）。amd64 は同じ照合で旧式の番号が多数引っかかる（§12.1）。
- `GOOS=android GOARCH=arm64 CGO_ENABLED=1 CC=<NDK clang> go build -buildmode=c-shared` で `libwails.so`（`ELF 64-bit … ARM aarch64`）ができ、`go tool nm` で `modernc.org/libc.X__syscall2`・`Xopen`・`_fstatat_kstat` を含むこと、パッチの `androidLegacySyscall` を含まないことを確認した。
- ストア層は android/arm64・android/amd64 でビルド・`go vet` が通る（§12.5）。

### 12.4 arm64 の実機で確認する手順

リリース APK は debuggable ではないので `adb shell run-as` は使えない。確認用に **debug APK**（`task android:run:device`。debuggable。起動時に自己確認のコードは入れていない）を実機へ入れて確かめる。

```bash
task android:run:device                       # arm64 の debug APK を実機へ入れて起動（既存のアプリは消える）
adb shell pidof jp.ousias.hitomado            # プロセスが生きている（SIGSYS で落ちていない）
adb logcat -d | grep -E "SIGSYS|SYS_SECCOMP|Fatal signal"   # 何も出ない
adb shell run-as jp.ousias.hitomado ls -la files/Hitomado    # hitomado.db・hitomado.db-wal がある
adb exec-out run-as jp.ousias.hitomado cat files/Hitomado/hitomado.db > hitomado.db
adb exec-out run-as jp.ousias.hitomado cat files/Hitomado/hitomado.db-wal > hitomado.db-wal
sqlite3 hitomado.db "select version_id,is_applied from goose_db_version; pragma integrity_check;"
```

マイグレーション（`goose_db_version` の全行が適用済み）と `integrity_check = ok` が見えれば、作成・マイグレーション・書き込みは通っている。読み取りは、アプリで Google アカウントを追加して同期させ（本物の Google が要る）、`accounts` や `threads` の行が増えることを `sqlite3` で確かめる。`logcat` には Wails のログ（`Page loaded: https://wails.localhost/`）が出る。debug APK の署名は debug 鍵なので、実機にリリース鍵のアプリが入っていれば先に `adb uninstall jp.ousias.hitomado` が要る（データも消える）。

### 12.5 Linux・Windows への影響がないことの確認

| 確認 | 結果 |
|------|------|
| `go mod tidy -diff` | 差分なし（`go.mod`・`go.sum` は変更していない） |
| NOTICE の生成（`bun .github/scripts/gen-notice.ts --check`） | 差分なし |
| `GOOS=windows go build ./...`・`go vet ./internal/store/...` | 通る |
| Linux の `go vet ./internal/store/...`・`go test ./internal/store/...` | 通る |
| `GOOS=android GOARCH=arm64 CGO_ENABLED=1 CC=<NDK clang> go vet ./internal/store/...` | 通る |
| `GOOS=android GOARCH=amd64 CGO_ENABLED=1 CC=<NDK clang> go vet ./internal/store/...`（パッチなし／`-modfile` でパッチあり） | どちらも通る |
| `-modfile`（パッチ済み libc）で `go vet ./internal/store/...` を android/arm64・linux/amd64・windows/amd64、`go test ./internal/store/...` を linux/amd64 | 通る（パッチの `legacy_other.go` が何もしないため、android/amd64 以外ではコードが同じ） |

## 13. 添付の保存・ログの書き出し・ファイルを開く（#85）

Android では Wails の `Dialog.SaveFile()` が使えない（`save file dialogs are not supported on Android`）。添付の保存（`ThreadService.SaveAttachment`）とログの書き出し（`LogService.ExportLogs`）は、**Storage Access Framework の `ACTION_CREATE_DOCUMENT`** で保存先を利用者に選ばせて書く。`LogService.OpenLogFolder` が使う `PathOpener.OpenPath` は `ACTION_VIEW` + FileProvider。Windows の動き（`savefile_other.go`・`open_other.go`。保存ダイアログのパスへ直接書く／エクスプローラー）は変えない。

### 13.1 構成

```
呼び出し側（ThreadService.SaveAttachment／LogService.ExportLogs）
  FileDialog.SaveFile → exportfile.Target（Path に書く → Commit → Close）
Windows: PathTarget（ダイアログのパスへ直接書く。Commit は何もしない）
Android: exportfile.Saver（純 Go。待ち合わせ・一時ファイル・後始末）
  → platform/savefile_android.go（go:linkname で Wails の Java ブリッジ呼び出しを使う）
  → WailsBridge.documentBegin／documentCommit／documentAbort／documentTempDir（Java。委譲だけ）
  → DocumentExporter（Java。Android に依存しない状態機械）→ MainActivity.launchCreateDocument（ACTION_CREATE_DOCUMENT）
  ← onActivityResult → WailsBridge.documentPickerResult → emitEvent("hitomado:document") → Go（app.Event.On）
```

流れ（保存先を**先に**選ばせ、キャンセルなら添付を取得しない。Windows と同じ順）:

1. Go: `Saver.Pick` → Java `documentBegin(id, 表示名, MIME)` → 保存先選択の画面を出す。Go は結果のイベントを待つ。
2. 利用者が選ぶ／キャンセルする → Java が `hitomado:document` イベント（`picked`／`cancelled`）で知らせる。キャンセルは `ok=false`（エラーではない）。
3. Go: 選ばれたら `cache/exports/save-<id>` に内容を書く（添付は Gmail から取得。ログは zip）。**巨大な `byte[]` を JNI に通さない**（添付は最大 25MB）。
4. Go: `Commit` → Java `documentCommit(id, パス)` → 別スレッドで一時ファイルから選ばれた Uri へ `ContentResolver.openOutputStream(uri, "wt")` でストリームコピー → `saved`／`failed` イベント。Go は一時ファイルを消す。
5. 書き込みに失敗・待ちの打ち切り（ctx のキャンセル・時間切れ）のときは `Close` が Java へ `documentAbort` を送り、選ばれて空のままの保存先（`DocumentsContract.deleteDocument`）を消す。

### 13.2 設計の判断

| 論点 | 判断 | 理由 |
|------|------|------|
| 保存先の選び方 | SAF の `ACTION_CREATE_DOCUMENT`（優先）。共有シート（`ACTION_SEND`）は使わない | 利用者が保存先を選べる（要件）。共有シートは「保存」ではなく他アプリへの送信で、ファイルが残る場所が利用者に分からない。ストレージの権限（`READ/WRITE_EXTERNAL_STORAGE`）も要らない（権限の許可リストを増やさない） |
| Go → Java の呼び出し | `//go:linkname` で Wails の `application.androidBridgeStringString`（SecureGet などが使う、Java のメソッドを名前で呼ぶ JNI 関数）を使う | `application.Android` の公開 API には任意のメソッドを呼ぶものが無い。Wails を fork したり、自前の JNI（JavaVM の取得と二重の `JNI_OnLoad`）を持つより小さい。Wails を上げて名前・シグネチャが変わると**リンクで失敗する**（実行時に黙って壊れない）。`docbridge_android.s`（空）は本体なしの関数宣言に必要 |
| Activity 結果の返し方 | `WailsBridge.emitEvent("hitomado:document", {id,status,…})` を Go が `app.Event.On` で受ける。保存ごとの `id` で待ち合わせる | Wails が非同期の結果を返すときと同じ（`common:capture` など）。JNI の export を増やさずに済み、`id` で古い結果と取り違えない。イベントは WebView にも届くが、ペイロードは `id`／状態／保存先の表示名／失敗コードだけでパス・Uri・内容は含まない |
| キャンセルの表し方 | `status: "cancelled"` → `SaveFile` は `(nil, false, nil)`。呼び出し側は今までどおり `Saved=false`（トーストなし） | Windows のキャンセルと同じ扱いで、UI を変えない |
| プロセスが死んで戻ったとき | **結果を捨てることを許容する** | 保存先選択中にプロセスが終わると Go の待ちも一時ファイルも一緒に消える。復元しても、結果を待つ呼び出し（UI のボタン操作）が無いので、保存しても意味がない。次の最初の `Pick` が `cache/exports` を空にして取りこぼしを片付ける。遅れて届いた選択結果は、保存先が空のまま残らないよう Java が削除して捨てる（`DocumentExporter.onPickerResult`）。Activity の再生成（回転は `configChanges` で防いでいる）では `DocumentExporter.INSTANCE`（プロセス内の static）が状態を持つので続く |
| 待ちの上限 | 選択 10 分・コピー 5 分。呼び出しの ctx のキャンセルでも打ち切る | 利用者が放置しても goroutine と一時ファイルを残さない（`Saver` のテスト） |
| 同時に 1 件 | `ErrBusy`（Java も `busy` を返す） | `startActivityForResult` の結果の取り違えを避ける（保存ボタンの連打） |
| ファイル名 | Go（`appcore.SanitizeFilename`）と Java（`DocumentPaths.sanitizeName`）の両方で、区切り（`/`・`\`）・制御文字・`<>:"|?*`・予約名・長さ（255 バイト）を直す。ディレクトリ部分（`../`）は捨てる。**一時ファイル名には使わない**（`save-<id>`） | メールの添付名は信頼できない入力。保存先のファイル名は SAF のプロバイダーも直すが、アプリでも直す |
| MIME | `exportfile.MIMEForName`（拡張子から。Android には `/etc/mime.types` が無いので表を持つ）。不明は `application/octet-stream` | 保存先選択画面の種類（`setType`）にだけ使う |
| 一時ファイルの場所 | `cache/exports`（`getCacheDir()` 配下。FileProvider が公開しない） | Java はここの**直下のファイルだけ**を読む（`DocumentPaths.resolveChildFile`。正規化してから比較。`../`・サブディレクトリ・シンボリックリンクの外向きは拒否） |
| 保存後の Mark-of-the-Web | Android では何もしない（`motw_other.go`） | 該当する仕組みが無い |
| `SaveAttachmentResult.Path`／`ExportLogsResult.Path` | Windows は保存したパス、Android は選ばれた保存先の表示名 | Uri や内部パスを画面・ログに出さない |

### 13.3 ファイルを開く（`OpenPath`）

- `ACTION_VIEW` + FileProvider（`WailsBridge.documentOpen`）。開けるのは **`files/Hitomado/logs` の配下のファイルだけ**（`DocumentPaths.resolveUnder` と FileProvider の公開範囲の両方で制限）。
- `res/xml/file_paths.xml` は Wails のテンプレートでは試作の `cache-path captures/`（カメラ用）だったが、**ログ（`<files-path path="Hitomado/logs/">`）だけを公開する**ものに替えた。データベース・資格情報・保存の一時ファイルは公開しない。カメラ撮影（`launchCameraCapture`）はこのアプリでは使わない（CAMERA 権限を持たない。§6）。
- **フォルダは開けない**（アプリ専用の領域を表示するファイルマネージャーの経路が Android に無い）。`OpenLogFolder` は `exportfile.ErrFolderNotSupported` を `UNSUPPORTED` にして、「ログを書き出す」を案内するメッセージを返す。設定画面の「ログフォルダを開く」ボタンを Android で隠すことは、このリポジトリの範囲外（フロント。§13.5）。
- 開けるアプリが無いときは `ActivityNotFoundException` を `no_app`（`exportfile.ErrNoApp`）にして返す。Android 11 以降は他のアプリが見えず（`<queries>` を宣言していない）事前確認できないので、Intent を投げた結果で判断する。

### 13.4 テスト

| 対象 | 場所 | 内容 |
|------|------|------|
| 純 Go（ホストで実行） | `internal/exportfile/*_test.go` | `Saver`（選択・キャンセル・取りやめ・時間切れ・busy・一時ファイルの後始末・前回の取りこぼしの掃除・結果の取り違え）、ファイル名の安全化、MIME 推定、結果／応答の JSON の解釈、JNI へ渡す JSON の ASCII 化 |
| 呼び出し側（ホスト） | `internal/app/apptests`・`internal/app/mcptests` | `Commit` と `Close` の呼び出し、取得失敗・書き込み失敗では `Commit` しない、Android の保存先表示名の返却、`OpenLogFolder` の `UNSUPPORTED` |
| Java（JVM。`task android:test`） | `DocumentExporterTest`・`DocumentPathsTest` | 状態機械（選ぶ・キャンセル・コピー成功／失敗・取りやめ・遅れて届く結果・id 違い・パス検査）、3MB のストリームコピー、パス検査（`../`・外のファイル・サブディレクトリ・シンボリックリンク・前方一致）、ファイル名の安全化、JNI 用の JSON の `\uXXXX` 化 |

### 13.5 未確認と確認の手順

自動で確かめたのは、Go・Java の単体テストと APK のビルド・`android:verify` まで。次は**実機・エミュレーターが要る（未確認）**:

1. 添付の保存: スレッドの添付の「保存」→ SAF の画面が出る → 場所を選ぶ → トースト「保存しました」→ 選んだ場所のファイルを開いて内容が正しい（PDF・日本語名・25MB 近いファイル）。
2. キャンセル: 保存先選択で戻る → トーストなし・添付の取得（通信）なし。続けて保存をやり直せる。
3. ログ: 設定 → ログ →「ログを書き出す」→ 保存先を選ぶ → zip を展開して `hitomado.log` が入っている。「ログフォルダを開く」→ 「ログを書き出す」を案内するエラー（ボタンを隠すのはモバイル UI の別 Issue）。
4. 保存先選択中にアプリを `adb shell am kill` → 起動し直す: 保存はされず、アプリは正常に動き、再度保存できる（`cache/exports` に残骸が無い: `adb shell run-as jp.ousias.hitomado ls cache/exports`。debug ビルド）。
5. ファイルを開く（`ACTION_VIEW`）: ログ 1 ファイルを開く導線は現状の UI に無い（`OpenLogFolder` のみ）。`documentOpen` の `ACTION_VIEW` は、導線を足す Issue の確認で扱う。
6. `go:linkname` が実機の cgo ビルド（`libwails.so`）でリンクされ動くこと（`task android:package` でのリンクは確認、実行は未確認）。

## 14. Google OAuth のリダイレクト方式（#86）

### 14.1 決定

**ループバック（`http://127.0.0.1:<一時ポート>/callback`）＋ PKCE ＋ `state` を、Android でも今のまま使う。** OAuth クライアントは Windows と同じ **「デスクトップ アプリ」種別**（Client ID と Client Secret）で、利用者が自分の Google Cloud に作る（BYO。README「OAuth クライアントの作り方」）。Android 専用のクライアントは作らない。

| 案 | 判断 | 理由（一次情報。2026-11-03 に確認） |
|----|------|------|
| **(a) ループバック（デスクトップ アプリ種別）** | **採用** | [Google の OAuth 2.0 for Desktop apps](https://developers.google.com/identity/protocols/oauth2/native-app)（2026-11-03 取得）の「Loopback IP address」は、アプリケーションの種類を **Desktop app** にする方式として載っている。非推奨（DEPRECATED）と書かれているのは **Android・iOS・Chrome アプリの「クライアント種別」** で、[ループバックの移行ガイド](https://developers.google.com/identity/protocols/oauth2/resources/loopback-migration)（同日取得）は「Desktop app クライアントでのループバックは**引き続きサポートされる**。対応は不要」と明記している。Google の認可サーバーが見るのは `client_id`（＝種別）と `redirect_uri` で、端末の OS は要求に含まれない（[INFERENCE] 文書にこの点の明記はなく、要求の中身からの推測。実機で確認する）。現在のコード（`internal/oauth`）がそのまま使え、Windows と同じクライアントを共用できる |
| (b) Android クライアント種別（パッケージ名＋署名鍵の SHA-1）＋カスタムスキーム | 採らない | ネイティブアプリ向け文書の `redirect_uri` の表に「**Custom URI schemes are no longer supported on Android and Chrome apps**」とあり、エラー表にも `Custom URI scheme is not supported on Android or Chrome apps` がある。Android 種別のクライアントには `client_secret` が無く（同文書の `client_secret` の行）、この種別が想定する取得方法は Google Identity Services の `AuthorizationClient`（Play 開発者サービス）。リフレッシュトークンが要る（サーバー側アクセス）場合は `requestOfflineAccess(webClientId)` で認可コードを受け、Web クライアントの Secret で交換する形になり（[Android の認可ガイド](https://developer.android.com/identity/authorization)・移行ガイド）、Play 開発者サービスが前提で、利用者ごとに Android クライアント＋Web クライアントの 2 つを作り、APK の署名鍵の SHA-1 を登録する手間が加わる。コードの変更も Java 側の認可クライアント呼び出し・Go の「Secret 無し」「サーバー認可コード」対応と大きい |
| (c) Web クライアント＋カスタムスキーム | 採らない | 上の (b) と同じ理由でカスタムスキームが使えない。Web クライアントはリダイレクト先に `https` の公開 URL を要し、公開者サーバーを持たない方針（NF）に合わない |

**BYO と署名鍵の SHA-1**: (a) を採ると、クライアントにパッケージ名（`jp.ousias.hitomado`）も署名鍵の SHA-1 も**登録しない**。そのため「デバッグ鍵で入れた APK と release 鍵の APK で SHA-1 が違い、クライアントを作り直す」という問題は起きない（どちらの APK でも同じ Client ID で認可できる）。署名鍵が違う APK の入れ替えで要るのは、`adb uninstall`（データも消える。§2）だけ。

**リスク**: Google が「モバイルでのループバック」の扱いを厳しくし、Desktop app クライアントでも Android から来た要求を拒否するようになる可能性がある（`native-app` 文書の `redirect_uri` の表に「mobile apps でのループバックは DEPRECATED」という一般的な注記もあるが、ここでいう対象はクライアント種別、と移行ガイドは区別している）。拒否されたら `invalid_request`（認可画面のエラー）になる。その場合の代替は (b) の `AuthorizationClient` 方式で、別 Issue になる。**実機・本物の Google での確認は未実施**（§14.6）。

### 14.2 ブラウザに出ている間の流れ

```
利用者が「アカウントを追加」→ StartAddAccount（Go）
  → 127.0.0.1:<一時ポート> で待受（Go の net/http。プロセス内）
  → Tracker.Begin()（「認可の途中」の印を SecureStore に保存）
  → Android.OpenURL（Intent ACTION_VIEW）で既定のブラウザへ
  → 利用者が Google で同意 → ブラウザが http://127.0.0.1:<ポート>/callback?code=…&state=… へ GET
  → 待受が code を受け取り、完了ページ「このタブを閉じて Hitomado に戻ってください」を返す
  → 利用者がアプリへ戻る（アプリ切替・戻る）→ トークン交換 → userinfo → アカウント作成 → Tracker.End()
```

- ブラウザからアプリへ**自動では戻らない**。完了ページの案内どおり、利用者がアプリ切替などで戻る（アプリ側は待受が受け取った時点で完了処理を進め、戻ったときには受信箱にアカウントが追加済みの画面が出る）。ブラウザからアプリを呼び戻すには `https` のアプリリンク（公開ドメインの `assetlinks.json`）かカスタムスキームの intent-filter が要り、公開者サーバーを持たない・Google がカスタムスキームを許可しない（§14.1）ため、採らない。manifest・`verify-apk.sh`・`CUSTOMIZATIONS.md` は変えていない（Java は変更なし）。
- Android の 127.0.0.1 は端末内のループバック。試作（#81）では、アプリ内の待受へ `adb shell` の別プロセス（`nc`）から GET して、完了まで動くことを確認した（ブラウザからの GET と本物の Google は未確認）。
- `state` を検証し（一致しない要求は無視して待受を続ける）、PKCE（S256）を使う。コードの扱いは Windows と同じ（`internal/oauth/callback.go`）。

### 14.3 プロセスが止められたとき（決定）

Android はブラウザが前面に出ている間、アプリのプロセス（バックグラウンド）を、メモリ不足などで終了させうる。Go の待受はプロセスと一緒に消え、ブラウザが後から `127.0.0.1:<ポート>` に接続しても拒否される。**この認可は続きからは完了できない。**

| 検討したこと | 判断 |
|--------------|------|
| `state`・PKCE verifier を SecureStore に保存して、起動後に完了する | しない。保存しても認可コードを受け取る口（待受）が無く、コードはブラウザのアドレスバーにしか残らない。使えない verifier を暗号化ストアに残さない（カスタムスキーム方式なら Intent で受けた `code` と保存した verifier で完了できるが、(b)(c) は採らない） |
| 前面サービス（`WailsForegroundService`、`dataSync`）でログイン中だけプロセスを生かす | しない。前面サービスはこのアプリで使わない（§15.1。#88 で削除した）。プロセスが止められる確率を下げるだけで、止められた後の扱いは結局必要 |
| **止められたことを次の起動で検出して「やり直し」と案内する（採用）** | `internal/oauth` の `Tracker`。認可を始めるときに SecureStore へ印（時刻のみ。秘密は入れない。キー `hitomado/oauth_in_progress`）を保存し、成功・失敗・キャンセル・タイムアウトのどれでも `End()` で消す。プロセスが止められると印が残るので、次の起動の最初の `TakeInterruptedAuth` が一度だけ `true` を返し、フロント（`accounts` ストアの `start`）が「ログインの途中でアプリが終了したため、完了できませんでした。もう一度『アカウントを追加』（または『再接続』）からやり直してください」のトーストを出す |

「ログイン中の状態を壊さない」: 認可の途中のデータはメモリ（待受・`state`・verifier・進行中フラグ）だけで、永続化は上の印（時刻）だけ。プロセスが止められても DB・トークン・既存アカウントには何も書かれておらず、再起動後の「別のアカウント認可が進行中です」の状態も残らない（進行中フラグはメモリ上）。トークン交換の後（アカウント保存）でプロセスが止められた場合は、保存の途中の状態が既存の経路で扱われる（アカウント作成→トークン保存の失敗時はロールバック。`account_auth.go`）。

- 印の読み書きに失敗（Keystore の鍵の喪失など）しても認可は続け、検出を諦めるだけ（`ErrKeyLost` は印が無いのと同じ）。
- 印は起動直後ではなく、最初に必要になったとき（フロントの `TakeInterruptedAuth`、または認可の開始）に読む。起動直後に Java へ JNI で問い合わせないため。
- Windows（デスクトップ）は `Tracker` を作らない（`nil`。`oauthtracker_other.go`）。Windows の認可の動きは変わらない。バインディング `TakeInterruptedAuth` は常に `false` を返す。

### 14.4 送信元の固定

リダイレクト方式は認可コードを受け取る経路だけを変える。アカウントのアドレスは、認可後に Google の userinfo から取った値（小文字に正規化）で決まり、返信・転送の送信元は `accounts.email` から作る（変更できない。[バインディング仕様 §5](../api/endpoints.md)・[アーキテクチャ設計 §7.3](../architecture/overview.md)）。`Reconnect` は認可されたアドレスが既存のアカウントと違えば拒否する。テスト: `internal/app/composecore/compose_oauth_sender_test.go`（OAuth で追加したアカウントの返信が、Google が返したアドレスに固定され、`SENDER_LOCKED`／`SENDER_MISMATCH` で変えられず、`From` がそのアドレスになる）、`internal/app/apptests/account_test.go`（`TestReconnectRejectsDifferentAddress`）。

### 14.5 自動テスト

| テスト | 内容 |
|--------|------|
| `internal/oauth/flow_test.go`・`callback`（既存） | 偽 IdP（httptest）で、ループバックの待受・PKCE・`state` の検証（不一致は無視）・`access_denied`・スコープ不足・トークン交換の失敗・タイムアウト・キャンセルを試す。Android でもこのコードがそのまま動く（ビルドタグなし） |
| `internal/oauth/tracker_test.go` | 印がブラウザに出ている間だけ存在し、成功・拒否・タイムアウト・ブラウザ起動失敗・キャンセルのどれでも消える。印に `state`・verifier・コード・トークンを含まない。**プロセスが止められた（`End` が呼ばれない）ときに、次の起動の `Tracker` が一度だけ検出する**。検出の前に新しい認可を始めても完了でき、前回の中断も案内される。ストアの失敗が認可を壊さない。`nil` の `Tracker` は何もしない（Windows） |
| `internal/app/apptests/account_interrupted_test.go` | `TakeInterruptedAuth` の一度だけの検出と、`nil`（Windows）での `false`。検出後のやり直しが完了する |
| `frontend/test/accountsInterrupted.test.ts` | 検出時にやり直しのお知らせ（インラインアラート）を 1 回だけ出す。呼び出しの失敗で起動を止めない |

### 14.6 実機で確認する手順（未確認）

**この Issue では、実機と本物の Google アカウントを使っていないため、次は未確認。** 自動テストで確かめたのは偽 IdP 越しの Go のフローとプロセス停止の検出までで、本物の Google に対するフローと、Android の挙動は確認していない。

1. README の手順で、**デスクトップ アプリ種別**の OAuth クライアントを作り（同意画面が外部・「テスト」ならテストユーザーに自分の Gmail を登録）、Client ID と Client Secret を実機のアプリの設定 → OAuth クライアントに登録する。
2. 設定 → アカウントを追加 → 認可を開始。ブラウザが開き、Google の同意画面が出る（`redirect_uri` が拒否される `invalid_request` や `redirect_uri_mismatch` にならない）。
3. 同意すると、ブラウザに「認可の操作が完了しました。このタブを閉じて Hitomado に戻ってください」が出る（127.0.0.1 へ届いた）。アプリへ戻ると、アカウントが追加されて同期が始まる。
4. 追加したアカウントのスレッドを返信で開き、送信元がそのアドレスに固定され、変更できないこと。
5. 別の鍵（debug／release）の APK に入れ替えたあとも、同じ Client ID で認可できること（SHA-1 を登録していないため）。
6. プロセス停止: 認可を開始してブラウザが出たら、`adb shell am kill jp.ousias.hitomado`（バックグラウンドのプロセスだけを終了する）でアプリを止め、ブラウザで同意（`127.0.0.1` への接続は失敗する）。アプリを開くと「ログインの途中でアプリが終了したため…やり直してください」のトーストが出る。もう一度アカウントを追加して完了できる。debug ビルドなら `adb shell run-as jp.ousias.hitomado cat shared_prefs/hitomado_secrets.xml` に平文のトークン・`state` が無いこと。
7. Google がモバイルのループバックを拒否した場合（§14.1 のリスク）: 認可画面に `invalid_request` が出る。この結果を Issue に記録し、(b) の `AuthorizationClient` 方式を別 Issue で起こす。

## 15. バックグラウンド同期と新着通知（#88）

### 15.1 方針

- アプリが**前面にいる間**は今の 60 秒ループ（Go のゴルーチン。`internal/sync`）。**閉じている間**は WorkManager の定期実行（**15 分間隔**＝`PeriodicWorkRequest` の下限・条件は**ネットワーク接続**）が、Go の「1 回だけ同期して新着を返す」関数（`internal/headless`）を呼ぶ。通知は Java の `NotificationManager` で出し、タップでそのスレッドを開く。
- **Pub/Sub・FCM は使わない**（公開者サーバーが要り、「サーバーを持たない」現行方針 [NF §4](../requirements/non-functional.md) と矛盾するため）。**前面サービス（`WailsForegroundService`・`FOREGROUND_SERVICE`・`FOREGROUND_SERVICE_DATA_SYNC`）も使わない**: 常時表示の通知が要る・targetSdk 35 以降は `dataSync` の前面サービスに時間の上限がある・Play の審査対象になるため。Wails のテンプレートにあった前面サービスの Java（`WailsForegroundService.java`・`WailsBridge.startForegroundService`／`stopForegroundService`）、manifest の権限 2 つとサービス宣言は削除した（Go 側の `application.Android.StartForegroundService` はこのアプリから呼ばない）。
- 即時性は最大 15 分遅れる。Doze 中・省電力中は OS がさらに遅らせる。利用者向けの案内は F-10。

### 15.2 Worker から Go を呼べる形（設計と理由）

**前提（Wails の c-shared エントリの順序）**: `libwails.so` は `System.loadLibrary("wails")` で読み込まれた時点で Go のランタイムと各パッケージの `init` が始まる（Go のランタイムは 1 プロセスに 1 つ）。`main` は自動では動かない。`WailsBridge.nativeInit`（Activity の `bridge.initialize()`）が `RegisterAndroidMain` に登録された `main` をゴルーチンで起動して初めて、Wails のアプリ（`application.New`・ウィンドウ・同期ループ）が動き出す。WorkManager の Worker は**アプリと同じプロセス**のバックグラウンドスレッドで動き、プロセスが無ければ OS がプロセスだけを起こす（Activity も `nativeInit` も無い）。

**設計**: Worker は Wails を通さず、`libwails.so` が export する別の JNI 関数 `Java_com_wails_app_HeadlessSync_nativeSyncOnce`（`internal/androidbg`。cgo。`headless_android.go` がパッケージを main に取り込む）を直接呼ぶ。この関数は Wails のアプリを作らず、`internal/headless` だけを動かす（store・gmail・sync。ウィンドウ・トレイ・Wails の通知サービス・イベントには触れない）。

| 検討 | 判断 |
|------|------|
| `nativeInit` を Worker から呼んで Wails ごと起動する | しない。ウィンドウ・60 秒ループ・MCP などが動き続け、バックグラウンドで電池を使い続ける。`WailsBridge` は Activity を前提にする |
| Go から資格情報を Wails の `application.Android.SecureGet` で読む | Worker では使えない。Wails の Java ブリッジ（`g_bridge`）は `nativeInit` が保持する `WailsBridge`（Activity から作る）で、Worker のプロセスでは `nil`。Go 側の `secrets.NewKeyring()` はそのままではここで失敗する |
| **Worker が `SecretBridge`（Java。`SecureStore` を使う）を JNI の引数で渡し、Go が呼び戻して読む（採用）** | `internal/secrets/envelope.go`（`NewEnvelopeBridge`）が `secureGet`／`secureSet`／`secureDelete` を「JSON の封筒」で呼び、既存の `bridgeStore`（#84。鍵の喪失は `ErrKeyLost`＝再接続）にそのままつなぐ。`SecretBridge` は `WailsBridge.secure*` と同じ実装で、`SecureStore.getInstance(context)` は `applicationContext` だけを使う（§11.4）。リフレッシュトークンを更新したときの書き戻しも同じ経路 |
| Worker が必要な資格情報を先に全部読んで JSON で渡す | しない。必要なキー（アカウントごとの Client Secret の行 ID）は Go が DB を読んで決めるため、Java が先回りできない。値を JNI の引数に長く載せるのも避ける |

Go のゴルーチンは任意の OS スレッドで動くため、呼び戻しの C（`jni_android.go`）は `JNIEnv` が無ければアタッチし、終わったらデタッチする（Wails と同じ方法）。呼び出しごとにグローバル参照を作って解放する。ライブラリを読めない・例外が出たときは、例外のクラス名だけをログに残し（メッセージは出さない）、Worker は失敗として扱う。

### 15.3 アプリ本体との排他（`headless.Gate`）

同じ DB・同じ資格情報・同じプロセスを、アプリ本体（前面の 60 秒ループ）と Worker が使いうる。**プロセス内の mutex ではなく、プロセスの状態で 3 通りに分ける**（`internal/headless/gate.go`）:

| 状況 | 動き |
|------|------|
| アプリ本体がこのプロセスで動いていない（OS がプロセスを止めていた・Worker が起こした） | Worker が自分の store・gmail（`headless.SyncOnce`）で 1 回同期し、終わったら DB を閉じる。同期状態（`history_id`）と新着の判定（`ApplyThread` が返す新規メッセージ ID）は DB が持つので、あとでアプリ本体が同じ DB を開いても、同じ新着を二重に通知しない。ヘッドレス同期同士は 1 つだけ（2 つ目は `busy`＝成功扱い） |
| アプリ本体が動いている（Activity が破棄されただけでプロセスと同期ループは残っている場合を含む） | Worker は別の同期を始めず、アプリ本体のスケジューラに**「今すぐ 1 回更新」**（`Scheduler.Refresh`）を頼む。60 秒のタイマーが OS に止められていても、Worker が起きたときに追いつく。新着の通知はアプリ本体の通知経路が出す（結果の新着は空） |
| ヘッドレス同期の実行中にアプリ本体が起動した | `run.go` が DB を開く前に `headless.AppStarting()` を呼び、実行中のヘッドレス同期を取り消して終わりを待つ（上限 10 秒）。その後は上の行になる |

- ヘッドレス同期の組み立て: ログ（`Hitomado/logs`。アプリ本体と同じファイル）→ `store.Open` → `gmail.NewClient`（資格情報は `SecretBridge`）→ `syncer.New`（通知は**集めるだけ**の `Notifier`。トレイは無い）→ `RefreshAll`（再接続待ち以外の全アカウントを `Refresh`）→ `Shutdown`。60 秒ループと**同じ `cycle`**（初回・リセット同期は通知しない、通知を切ったアカウントは通知しない、1 スレッド 1 通知）なので、新着の判定は Windows・前面と同じ。DB がまだ無い（アカウント未登録）ときは何もしない（DB を作らない）。1 回の上限は 5 分（WorkManager のジョブの上限より短い）。
- Worker が OS に止められたとき（制約が外れた・時間切れ）は `onStopped` → `nativeCancel` で Go の同期を取り消す。
- 結果は JSON（`headless.Encode`。ASCII のみ。失敗は理由のコード `busy`／`failed` だけ）。新着の表示文字列は、Windows のトーストと同じ `notify.Text`（制御文字・不可視文字の除去と長さの切り詰め）で整形してから渡す。

### 15.4 WorkManager

- 依存: `androidx.work:work-runtime:2.12.0`（2026-11 時点の最新安定版。[リリースノート](https://developer.android.com/jetpack/androidx/releases/work)と Google Maven の `maven-metadata.xml` の `release` で確認。rc／beta／alpha は選ばない）。2.10 系は minSdk 21 の最終の系列で、2.11 は minSdk 23、**2.12 は 24**（`work-runtime` の AAR の manifest で確認）。minSdk 31（§3）で満たし、`compileSdk` 36 も要件（33 以上）を満たすので `checkAarMetadata` に引っかからない。2.10.5 から上げて、`SyncWorker`・`SyncScheduler` の変更は要らなかった（`task android:test` の 73 件も通る）。WorkManager が manifest のマージで足す権限は `WAKE_LOCK`・`RECEIVE_BOOT_COMPLETED`（再起動後の再登録に要る）で、`FOREGROUND_SERVICE` は `tools:node="remove"` で取り除く（前面サービスは使わない）。2.12.0 の AAR も同じ `FOREGROUND_SERVICE` を足すが、マージ後の APK に残っていない（`aapt2 dump badging`）。`SystemForegroundService` の宣言は残るが、起動されない。2.12 は minSdk 24 以上になったので、2.10.5 にあった `SystemAlarmService` と `ConstraintProxy*` のレシーバー（API 23 未満のフォールバック）の宣言が無くなり、マージ後の WorkManager 由来の宣言は `SystemJobService`・`SystemForegroundService`・`ForceStopRunnable$BroadcastReceiver`・`RescheduleReceiver`・`DiagnosticsReceiver`・`InitializationProvider` になった。新しい権限・サービスは増えていない。
- 登録: `SyncScheduler.schedule` が `enqueueUniquePeriodicWork("hitomado-sync", KEEP, 15 分・ネットワーク接続)`。**アプリの起動時**（`run.go` の `startBackground`。アカウントがあれば登録、無ければ解除）と**アカウント追加後**（`AccountService.StartAddAccount`）に、Go が `Background.ScheduleSync(true)` で Java の `scheduleSync` を呼ぶ。`KEEP` なので何度呼んでも 1 つ。最後のアカウントを削除したら解除する（アカウント 0 件では何もしない）。
- 結果（`SyncPlan`）: すべて失敗（一時エラー）なら `Result.retry()`（WorkManager のバックオフ）。ただし 3 回目の試行からは `success`（次の定期実行に任せる）。**再接続が必要**（`needs_reauth`。リフレッシュトークンの失効・喪失、Client ID／Secret の拒否）は再試行しても直らないので `success` とし、「再接続が必要です」を**そのアカウントにつき 1 回だけ**通知する（`SharedPreferences hitomado_notify_state` に通知済みを保存。再び同期できたら消す）。別の同期が実行中（`busy`）も `success`。

### 15.5 通知

- **アカウント別チャネル**: チャネル ID は `mail_<アカウント ID>`、表示名はメールアドレス（`NotificationPlan`）。OS の通知設定でアカウントごとに音・重要度を変えられる。チャネルは通知を出すときに作り（既にあれば利用者の変更を保つ）、アカウント削除（`AccountService.RemoveAccount` → `Background.ForgetAccount`）でチャネルと表示中の通知を削除する。再接続の通知は固定のチャネル `reconnect`。
- **グループ化**: スレッドごとに 1 通知（通知 ID はアカウント＋スレッド ID から決まり、同じスレッドの新着は更新される）、アカウントごとにサマリー通知を 1 つ（グループキー `account_<ID>`。表示中のスレッド通知の数を表示）。
- **載せる内容**: 既存のデスクトップ通知の仕様（F-10）と同じ。見出しはアカウント名、本文は差出人と件名（`BigTextStyle`。折りたたみは 1 行）。**ロック画面は `VISIBILITY_PRIVATE`**（チャネルも通知も。公開版は「新着メールがあります」で、件名・差出人を含まない）。本文（メール本文）は通知に載せない（DB にも無い。NF-S08）。ログにも件名・差出人を出さない（NF-S10）。
- **許可**（Android 13 以降の `POST_NOTIFICATIONS`）: **最初のアカウントの追加が済んだところ**で `RequestNotificationPermission`（`StartAddAccount` が既存 0 件のときだけ）。起動直後や、追加の途中では求めない（理由を伝えられる文脈が無いため）。許可されていない・OS の設定で通知がオフのときは、設定画面（一般）の「通知」に状態と案内（「通知を許可する」「OS の通知設定を開く」）を出す。永続的に拒否されるとダイアログは出ないため、「OS の通知設定を開く」（`ACTION_APP_NOTIFICATION_SETTINGS`）へ案内する。状態は設定を開いたとき・アプリへ戻ったとき（`focus`）に読み直す。許可されていない間も同期は行う（通知だけ出さない）。
- **未読バッジ**: ランチャーのバッジは OS の通知件数に任せる。アプリ内の未読表示は従来のフロントのまま。トレイのバッジ（`notify.Notifier.SetUnread`）は、Android では `Tray` が `nil` で何もしない（`platform.Notifier.SetUnread` は `tray` が `nil` なら即戻る。§10）ので、空振りで失敗しない。

### 15.6 ディープリンク（通知のタップでスレッドを開く）

1. 通知の `PendingIntent` は `MainActivity` を開く（アクション `jp.ousias.hitomado.OPEN_THREAD`、extras はアカウント ID・スレッド ID だけ。件名・差出人は載せない）。
2. `MainActivity.onCreate`（プロセスが無い・Activity が無い）／`onNewIntent`（`singleTask` の起動済み）が `OpenRequest`（プロセス内の 1 件の保持領域）へ入れる。**要求そのものは Java が保持し、Go が取りに来る**（Go（Wails）が起動する前に Java が発行したイベントは取りこぼされるため）。
3. 起動済みのときは、Java が `hitomado:open-thread`（中身なしの合図）を Go へ送る。Go の `notify.OpenRequests.Wake` が `takeOpenRequest` で取り出し、フロントが準備済みなら `notify:open-thread` を発行する（既存のフロントのハンドラ `threadStore.onNotifyOpen` がそのスレッドを開く）。フロントが準備前なら最新の 1 件を保持する。
4. コールドスタートでは、フロントが `thread.start()`（イベント購読）のあとに `NotifyService.TakePendingOpenThread` を 1 回呼び、保留中のスレッドを同じ `onNotifyOpen` で開く。Go はこの呼び出しで「フロントが準備できた」と記録し、以後の要求は `notify:open-thread` で発行する。**準備前に来た要求は、準備後に必ず `TakePendingOpenThread` が返す**（取りこぼさない。`internal/notify/openrequest_test.go`・`frontend` の `threadStore.test.ts`）。
5. 狭幅（#87）の画面遷移は、フロントの「スレッドを開く」の既存の操作（`inbox.select`＋`open`）が選択中のスレッドから導くので、ここは何も足さない。

### 15.7 manifest の整理

- 削除: `FOREGROUND_SERVICE`・`FOREGROUND_SERVICE_DATA_SYNC`、`WailsForegroundService` の宣言（Wails の Java ブリッジが参照していたのは `startForegroundService`／`stopForegroundService` だけで、Go から呼ぶ箇所もこのアプリには無い）。Wails の `postNotification`（タップのデータを持たず、チャネルも 1 つ）も `NewMailNotifier` に置き換えて削除した。
- 許可リスト（`verify-apk.sh`）: `INTERNET`・`VIBRATE`・`ACCESS_NETWORK_STATE`・`POST_NOTIFICATIONS`・`WAKE_LOCK`・`RECEIVE_BOOT_COMPLETED`（最後の 2 つは WorkManager）。`WailsForegroundService` が manifest に残っていたら失敗する検査も足した。

### 15.8 自動テスト

| テスト | 内容 |
|--------|------|
| `internal/headless/*_test.go` | `syncWith`（新着を返し通知しない・同じ新着を二重に返さない・通知オフのアカウント・失敗の分類〈再接続／一時／Client 拒否／再接続待ちは問い合わせない〉・アカウント 0 件・再接続待ちだけのとき `Refresh` に空を渡さない）、実際の SQLite での `SyncOnce`（DB が無ければ何もしない・資格情報が無ければ要再接続になり DB にも残る）、`Gate`（排他・アプリ本体への委譲・起動時の取り消しと待ち・取り消し）、`Encode`（ASCII のみ・失敗は理由のコードだけ・表示文字列の整形） |
| `internal/secrets/envelope_test.go` | `SecretBridge` の封筒を Store にする部分（往復・鍵の喪失は `ErrKeyLost`・値をエラーに載せない・ASCII のみ・空のキー） |
| `internal/notify/*_test.go` | `OpenRequests`（準備前の保持・コールドスタートの取り出し・Wake・準備後の発行）、`JavaBackground`、`Text`（旧 `noticeText`） |
| `internal/app/apptests/notify_test.go` | 最初のアカウントの追加でだけ許可を求める・追加で同期を登録・削除でチャネルを消し 0 件で解除・許可状態・Windows（`Background` なし）は `unsupported` |
| Java（`./gradlew testDebugUnitTest`。JVM のみ） | `NotificationPlan`（チャネル ID・表示名・グループ・通知 ID・extras）、`OpenRequest`、`SyncOutcome`（JSON の読み取り）、`SyncPlan`（通知・戻り値・再接続の通知は 1 回だけ）、`SecretBridge`（Activity なしで往復）、`SyncScheduler`（15 分・ネットワーク接続・`SyncWorker`） |
| `frontend` | `threadStore.test.ts`（コールドスタートの保留スレッドを開く）、`settings.test.ts`（通知の許可の項目の出し分けとボタン） |

### 15.9 確認の状況

**エミュレーター（x86_64・API 35・KVM なしのソフト実行。自分用の AVD `a88sync`。確認後に停止・削除した）で確認したこと**（`adb root` で DB に偽のアカウント〈資格情報なし〉を入れて試した）:

- アプリの起動で `scheduleSync` が呼ばれ、WorkManager のジョブ（ネットワーク接続の制約つき・周期 15 分・バックオフ 30 秒）が JobScheduler に登録される（`dumpsys jobscheduler`）。
- プロセスを `kill -9` して、`cmd jobscheduler run -f -n androidx.work.systemjobscheduler jp.ousias.hitomado <id>` でジョブを強制実行すると、**Activity・Wails アプリの無いプロセスが起こされ**、`SyncWorker` が `System.loadLibrary("wails")` → `Java_com_wails_app_HeadlessSync_nativeSyncOnce` → Go の `headless.SyncOnce` まで動き、`Worker result SUCCESS` で終わる（クラッシュなし）。
- 資格情報が無いアカウントは、Go が `SecretBridge`（`SecureStore`）を JNI で呼び戻して読み（`found:false`）、DB の状態が `needs_reauth`／「再接続が必要です」になり、Worker が**「再接続が必要」の通知を 1 回**出した（`dumpsys notification`: チャネル `reconnect`・`vis=PRIVATE`・`publicVersion` あり・`AUTO_CANCEL`・アカウント ID が tag）。通知済みは `shared_prefs/hitomado_notify_state.xml` に保存される。
- 周期ジョブを続けて強制実行すると WorkManager は「周期より前の実行」として実行しない（`Delaying execution … because it is being executed before schedule`）。確認の再実行には WorkManager の DB の `last_enqueue_time` を書き換える必要がある（`adb root` で書くと所有者が root になりアプリが DB を開けなくなるので、`chown`・`restorecon` が要る）。

**未確認**（ここでは確認できない）:

- **実機・本物の Gmail での「アプリを閉じていても新着が最大 15 分ほどで通知される」**（新着が見つかる経路 — 実際の Gmail の `history.list`・`threads.get` — と、`NewMailNotifier.postNewMail` による新着の通知〈スレッドごとの通知・サマリー・グループ〉は、エミュレーターでは動かしていない。ホストの Go のテスト〈偽の Gmail〉と JVM のテストまで）。
- **通知のタップでスレッドが開くこと**（`PendingIntent`→`MainActivity`→`OpenRequest`→Go→`notify:open-thread`／`TakePendingOpenThread`→フロント）。エミュレーターは負荷（ソフトウェア実行）で System UI が応答せず、画面で確認できなかった。起動時の `onCreate` が落ちないところまで。各段はホストのテスト（Go の `OpenRequests`・フロントの `threadStore`）と JVM のテスト（`OpenRequest`）。
- **アプリ本体が同じプロセスで動いているときの Worker**（`Gate` の委譲。Go の単体テストのみ）。
- **通知の許可のダイアログ**（Android 13 以降の実機の挙動）と、設定画面の案内の見た目。
- **arm64 の実機**での `HeadlessSync`（arm64 は APK のビルドと JNI シンボルの export（`nm -D`）までで、実行は x86_64 のみ）。
- Doze・アプリの待機バケット・メーカー独自の省電力（バッテリー最適化）による実際の実行間隔。WorkManager は 15 分を保証せず、OS が遅らせる。

## 16. スマホ向けの画面（#87）

画面のコードは Windows 版と共通（同じ Vue）。幅による 3 ペイン／2 ペイン／1 画面ずつの切り替え、戻る、タッチ、のフロント側の設計は [コンポーネント設計 §5.1](../components/overview.md#51-画面幅によるレイアウトの切り替えandroid)、画面の仕様は [機能要件 §5.1](../requirements/functional.md#51-androidスマホの画面)。ここでは Android 側（Go・Java）の取り決めを書く。

### 16.1 OS による切り替え（`TouchUI`）

- `capability.Capabilities.TouchUI`（JSON `touchUi`）を足した。`Android()` だけ `true`、`Desktop()`・ゼロ値は `false`。フロントは `platformStore.touchUi`（OS 名ではなく機能）で、幅による切り替えと `styles/touch.css`（タップ領域 44px 以上）を有効にする。Windows 版は `false` なので、幅に関わらず今の 3 ペインのまま。
- **最小幅 960px の制限を Android では外す**: 制限は 2 か所にあった。①Go の `internal/platform` の `MinWidth`・`MinHeight`（ウィンドウ作成時の `MinWidth`／`MinHeight`、保存した状態の補正 `windowstate.go`）、②フロントの `.shell { min-width: 960px }`（`AppShell.vue`）。①は `platform.MinWindowSize(caps)` が `TouchUI` の OS では 0（制限なし）を返す。保存した状態の補正は Android では `WindowKeeper` を作らないので効かない。②は `layout-wide`（3 ペイン）のときだけ付ける。なお Wails の Android 実装（`webview_window_android.go`）の `setMinSize` は何もしないので、①は意図を明示するためのもの。

### 16.2 safe area（エッジ・ツー・エッジ）

- targetSdk 36 では、Activity のウィンドウがステータスバー・ナビゲーションバーの背後まで広がる（エッジ・ツー・エッジが強制される）。テンプレートの `MainActivity` は insets を扱っておらず、そのままでは WebView の上端・下端がバーに隠れる。
- **Activity 側で insets を当てる方式を選んだ**（`MainActivity.applyWindowInsets()`）。system bars・display cutout・IME の insets を、WebView を入れた `main_container` の padding に当てる。CSS の `env(safe-area-inset-*)` と `viewport-fit=cover` は使わない: Android の WebView では `env()` の値が端末・WebView のバージョンで 0 のままになることがあり、信頼できないため。Java で 1 か所に当てれば、フロントの CSS は safe area を知らなくてよい（`index.html` の viewport も変えない）。IME の insets も同じ padding に含めるので、ソフトキーボードが出たときも入力欄が隠れない（`adjustResize` は 36 のエッジ・ツー・エッジでは効かないため）。
- バーの背景色は `app_background`（`values`＝`#F5F5F4`、`values-night`＝`#050505`。フロントの `--bone` のライト／ダークと同じ値）。バーのアイコンの明暗は `updateSystemBarIcons()` が uiMode に合わせる。フロントの配色を変える（`tokens.css`）ときはここも合わせる。

### 16.3 戻るボタン

- `MainActivity` は `OnBackPressedCallback`（`getOnBackPressedDispatcher()`）で戻るを受ける。テンプレートの `onBackPressed()` の上書きは、targetSdk 36 では呼ばれないので削除した。
- コールバックは `webView.evaluateJavascript` で `window.hitomadoHandleBack()` を呼ぶ。フロントの `lib/androidBack.ts` が `AppShell` で登録する関数で、一つ前の画面へ戻したら `true` を返す（オーバーレイ → ドロワー → 返信・作成 → スレッド → 検索結果の順）。`true` ならアプリは何もしない。`false`・未登録（起動中）・失敗のときは、WebView の履歴があればそれに戻り（`canGoBack`）、無ければ `moveTaskToBack(true)` でアプリをバックグラウンドへ送る（終了しない）。
- Wails のイベント（Java → Go → `Events.Emit`）も `window.history`（`pushState`＋`popstate`）も使わない。前者は Go を経由する分遅れて「消費したか」を Java へ返せず、後者は履歴の積み方と `canGoBack` が一致しないとアプリが終了するため。1 つの関数の戻り値で「消費したか」を同期的に受け取る方が確実。

### 16.4 確認の状況

- ブラウザ（vite dev・Wails ランタイムをモック）で 390x844・800x900・1440x900 の画面を確認した（スクリーンショットは作業ログ）。`bun test` は、1 画面ずつの遷移（一覧 → スレッド → 返信 → 戻る）、幅を変えても選択・下書きが残ること、戻るの順序、受け口の登録を確認する。
- **未確認**: 実機・エミュレーターでの表示（safe area・戻るボタン・ソフトキーボード・Fold の折り畳み）。Java は `task android:package`／`android:verify` でビルド・検査できるが、動作はエミュレーターか実機で確認が要る。

## 17. 実機・エミュレーターでの E2E・WebView のクラッシュ対策・性能（#90）

### 17.1 WebView のレンダラが落ちたとき（`onRenderProcessGone`）

**問題**: Wails 3.1.0 のテンプレートの `WebViewClient` は `onRenderProcessGone` を実装していない。WebView のレンダラ（別プロセス）がクラッシュするか OS に止められると、Chromium は `Render process crash wasn't handled ... triggering application crash` として**アプリのプロセスごと**落とす（試作 #81 で確認）。

**決定**（`MainActivity`）:

| 項目 | 内容 |
|------|------|
| 戻り値 | `true`（アプリのプロセスを落とさない） |
| 復旧 | 落ちた `WebView` を親から外して `destroy()` し、新しい `WebView` を同じ位置に作る（`recreateWebView`）。`setupWebView()` が設定・`WebViewClient`・`WebChromeClient`・JS インターフェース（`window.wails`）・`WailsBridge` の WebView 参照を新しい WebView につなぎ直し、`loadApplication()` が同じ URL（アセットは Go から配信）を読み直す。`window.hitomadoHandleBack`（戻る）は読み込み直したフロントが登録し、Java→JS の同期イベントは `onPageFinished` の `emitSystemSnapshot` で再送される。`Activity.recreate()` ではなく WebView だけを作り直す（Go のランタイム・DB・同期は同じプロセスで動き続け、Activity の再生成で `bridge.initialize()` が二重に走るのを避ける） |
| 上限 | 直近 **5 分に 3 回**まで（`RecoveryBudget`）。超えたら WebView を外し、「画面の表示を続けられませんでした。アプリを終了して、もう一度起動してください。」と「アプリを終了」ボタンのエラー画面（`showFatalError`）を出す。ボタンは `finishAndRemoveTask()` とプロセスの終了（次の起動で Go のランタイムもきれいに始める）。無限ループしない |
| ログ | `didCrash()` が真なら `WebView renderer crashed`、偽（OS が止めた＝ほとんどはメモリ不足）なら `WebView renderer was killed by the system (likely low memory)`（タグ `WailsActivity`。`RenderGoneReason`） |
| 古い通知 | すでに作り直した WebView（`view != webView`）からの通知は何もしない |

**下書きはどうなるか**（確認した範囲）: スレッド画面の返信下書きは、**Go が「作成セッション」をメモリ上で持ち**、フロントは入力内容を都度渡す（[バインディング仕様](../api/endpoints.md)）。保存は編集停止の約 3 秒後の**自動保存**（`useAutosave`。`AUTOSAVE_DELAY_MS = 3000`）で、Gmail の下書きとして保存される（[F-12](../requirements/functional.md#f-12-下書き保存)）。DB には下書きを持たない（[ER](../architecture/er.md)）。したがって WebView を作り直しても、**自動保存済みの分は Gmail の下書きに残り**、スレッドを開き直せば「下書きを開く」で復元できる。**残課題**: 作り直しの直前の約 3 秒分の未保存の入力は失われる。作成セッションは Go のメモリに残るが、作り直したフロントはそれを参照しない（プロセスを終了するまでの小さな残り）。

### 17.2 初回の `index.html` 待ちの 10 秒（`timeout waiting for app to be ready`）

- Wails の Go 側（`application_android.go` の `nativeServeAsset`）は、アプリの準備（`appReady` チャネル）を**最大 10 秒**待ち、超えると `[JNI] timeout waiting for app to be ready, dropping request for /index.html` と記録して**空の応答**を返す。待ち時間はハードコードで、変えるには Wails 本体の修正が要る（このリポジトリでは `-overlay` で Go のファイルを差し替える仕組みはあるが、Wails の上流の挙動の複製は保守の負担が大きいので採らない）。
- 空の応答では `WailsPathHandler` が `null` を返し、WebView が実際のネットワークへ `https://wails.localhost/` を取りに行って失敗する（メインフレームの読み込みエラー）→ **白い画面のまま**になる。
- **決定**: 待ち時間は延ばさず、`WebViewClient.onReceivedError`（メインフレームのアプリ URL だけ）で **3 秒後に読み直す**。**直近 5 分に 6 回まで**（`RecoveryBudget`）。超えたら 17.1 のエラー画面。準備が間に合わなかった 1 回目だけが失敗し、読み直しのときにはアプリが準備済みなので通る。最小の変更で、遅い端末・初回起動でも白画面のままにならない。
- 実測（KVM なしのソフトウェアエミュレーター）では、この 10 秒のタイムアウトは今回の実行では発生せず（`Serving asset: /index.html` から応答まで約 10 秒以内）、読み直しの経路は**実機・エミュレーターでは動かして確認していない**（`RecoveryBudget` の単体テストと、コードの読み合わせのみ）。

### 17.3 E2E

**構成**（本物の Google アカウントは使わない）:

- **テスト用起動モード**（`internal/e2e`）: ビルドタグ **`e2e`**（かつ `production` でない）のときだけ、`run.go` が Gmail クライアントと資格情報ストアを、**メモリ上のシード済みフェイク**に差し替える。フェイクは既存のテスト用 `internal/app/apptest.FakeGmail`（`internal/app/apptests` が使うのと同じもの）で、シードは**アカウント 1 件（`e2e-user@example.com`）・INBOX のスレッド 3 件（件名 `E2E seed thread 1`〜`3`、新しいものから 1 時間ずつ古く、最新の 2 件が未読）**。DB（実 SQLite）は通常どおり、初回だけアカウントと OAuth クライアントを作る。同期は**実エンジン**が通常どおりこのフェイクに対して走る（一覧は DB から出る）。`RecordingGmail` が下書きの作成・更新を `files/Hitomado/e2e/drafts/<id>.txt` にも書き出し、E2E が「返信下書きが Gmail へ渡った」ことを読み取る。
- **release に混入させない**: ① `internal/e2e/override_off.go`（`!e2e || production`）は何もしない。シード・フェイクを含むファイルは全て `e2e && !production` のビルドタグつきで、通常のビルドにはコンパイルされない。② `android:e2e:apk`（`E2E=true`）は debug ビルドだけ（`build` の precondition が `PRODUCTION=true` との併用を拒否）。③ `verify-apk.sh` が `libwails.so` に E2E の印（`hitomado-e2e-backend-v1`）が**無い**ことを検査する（`task android:verify` の対象に追加）。④ E2E ビルドでは WorkManager の登録（`ScheduleSync`）を行わない（フェイクの資格情報は Worker（ヘッドレス）から使えないため）。
- **Go の単体テスト**: `go test -tags e2e ./internal/e2e/...`（シードの冪等・実エンジンでの初回同期で一覧に 3 件・`RecordingGmail` の書き出し）。

**スクリプト**（`build/android/e2e/run.sh`。adb と UI Automator の画面ダンプ `uiautomator dump` だけ。Python 3 標準ライブラリの `ui.py` がダンプから要素を探す）:

1. インストール → コールドスタート（`am start -W`。`TotalTime` 等を記録）→ プロセスが動いている。
2. **一覧**: シードの件名 3 件が画面（`uiautomator dump` の text／content-desc）に出るまで待つ。起動から一覧までの秒を記録。
3. **返信下書き**: 1 件目を開く → 「全員に返信」→ エディタ（`本文`）にタップして `hello e2e draft` を入力 → 自動保存（約 3 秒）後、`run-as` で `RecordingGmail` のファイルを読み、本文と `thread: e2e-thread-1`（スレッドに紐づく）を確認。
4. **レンダラ落ち**（`E2E_KILL_RENDERER=1`、既定。`adb root` が要るのでエミュレーターの `google_apis`・`userdebug` イメージ向け。root が取れなければこの手順だけ SKIP）: `sandboxed_process` を `kill -9` → アプリのプロセス（pid）が変わらない・一覧が再び出る・`WailsActivity` のログに復旧が出る。
5. `crash` バッファにアプリのクラッシュが無い・`Render process crash wasn't handled` が無い・最後までプロセスが動いている。

画面のスクリーンショット・ダンプ・logcat は `E2E_OUT`（既定 `/tmp/hitomado-e2e-out`）に残る。

**回し方**:

```sh
wails3 task android:e2e:apk ARCH=amd64          # x86_64 エミュレーター用。実機（arm64）は ARCH=arm64（既定はホストの arch）
wails3 task android:e2e:run                      # 接続中の端末（ANDROID_SERIAL で指定）。bin/hitomado-e2e.apk を入れて通す
```

**実機で回す手順**（arm64。`adb` が使え、開発者オプション・USB デバッグが有効な端末）:

1. `wails3 task android:e2e:apk ARCH=arm64` で `bin/hitomado-e2e.apk` を作る。
2. `adb devices` で端末が見えることを確認し、複数台なら `export ANDROID_SERIAL=<serial>`。
3. `E2E_KILL_RENDERER=0 wails3 task android:e2e:run`（市販の実機は `adb root` が使えない。レンダラの kill は、手順 4 で手動で行う）。
4. レンダラ落ちを実機で試すとき: 一覧が出た状態で、`chrome://crash` に相当するものは WebView には無いので、`adb shell am kill` は使えない。Android Studio の Profiler か、`adb shell` で root が取れるデバッグ用端末で `ps -A | grep sandboxed_process` の pid を `kill -9` する。**実機ではまだ実行していない**。
5. 後始末: `adb uninstall jp.ousias.hitomado`（E2E 用 APK は release と同じ署名ではないので、release と同じ端末に入れる前に必ず消す。データは端末から消える）。

### 17.4 CI

- **`.github/workflows/android-e2e.yml`（`workflow_dispatch` のみ）**: x86_64・API 35 のエミュレーターを `reactivecircus/android-emulator-runner` で起動して `run.sh` を実行する。ubuntu-latest は KVM が使える（udev のルールで権限を付ける）。
- **`ci.yml`（PR の CI）には入れていない**。このホストは KVM が使えず、ワークフローをローカルで動かして確かめられない。確認できないジョブを PR の必須の CI に入れると、赤くなって PR が止まるおそれがあるため。`actionlint` で構文だけ確認した。**Actions で実際に動かして緑になったことは未確認**。緑になったら `ci.yml` への組み込み（`pull_request` かつ変更パスの条件つき）を別 Issue で検討する。

### 17.5 性能の計測

**同期（Go のベンチマーク。ホストで実測）**: `internal/sync/syncertests/bench_test.go`。初回同期（INBOX の最新 200 スレッド＝`initialPageSize`、メタデータの取得と SQLite への保存）を、

```sh
go test -run '^$' -bench 'InitialSync' -benchtime=3x ./internal/sync/syncertests
```

で測る。

| ベンチ | 内容 | 結果（Linux x86_64、Ryzen 9 5950X、3 回の平均） |
|--------|------|------|
| `BenchmarkInitialSyncHTTP` | 本物の `gmail.Client`（クォータ制御つき）を、`httptest` の偽 Gmail サーバー（`threads.list`・`threads.get`・`profile`・`labels` を実 API の形で返す）に向ける | **約 7.2 秒 / 200 スレッド** |
| `BenchmarkInitialSyncStore` | ネットワーク・クォータ制御を除き、メモリ上のフェイクの Gmail から同期エンジン→実 SQLite に保存するまで | **約 0.15 秒 / 200 スレッド** |

前者はクライアントの本番どおりの流量制限（ユーザーあたり 250 units/秒、`threads.get` は 10 units → 約 25 スレッド/秒）が支配的で、実 Gmail に対しても同じ上限が掛かる（ネットワークの遅延は含まない）。後者から、**エンジン＋SQLite の分は 200 スレッドで 0.15 秒（ホスト）**で、同期の時間はほぼ API の流量制限で決まる。**これはホスト（デスクトップ CPU）の数値で、Android の実機の値ではない**。実機の SQLite・CPU での数値は未計測。

**起動時間（エミュレーター。ソフトウェアエミュレーションでの参考値であり、実機の数値ではない）**: `run.sh` が `adb shell am start -W` の `TotalTime`／`Status`（`METRIC am_start_total_ms`）と、起動から一覧の件名が画面に出るまでの秒（`METRIC start_to_list_s`）を出す。実機では `adb shell am start -W -n jp.ousias.hitomado/com.wails.app.MainActivity` の `TotalTime`（コールドスタート。`am force-stop` のあと）と、`adb logcat` の `Displayed` で測れる。

### 17.6 確認の状況（2026-11-04）

**エミュレーター**: x86_64・API 35・`google_apis`（WebView 124.0.6367.219）の自分用 AVD（`-accel off -no-window -gpu swiftshader_indirect`、RAM 3 GB、ポート 5592。確認後に停止・AVD を削除した）。このホストは KVM が使えず、ソフトウェアエミュレーション（TCG）。ホストは他の作業でも高負荷（load average 4〜29）で、システム自体が遅い。**以下の数値は実機の値ではなく、ソフトウェアエミュレーションでの参考値**。

| 項目 | 結果 |
|------|------|
| E2E 用 APK のビルドと起動 | `wails3 task android:e2e:apk ARCH=amd64` → 28 MB の APK。インストールでき、**アプリのプロセスは起動し、Go が `index.html` を配信した**（`Serving asset: /index.html` → `[AssetFileServerFS] Handling request` → `Serving /index.html with type text/html (744 bytes)`）。ログに E2E のビルドタグ（`-tags=android,debug,e2e`）が出る |
| `am start -W` | `Status: timeout`（`WaitTime` 34〜64 秒で待ち切れない）。**`TotalTime` は取れなかった**。コールドスタートから `Loading URL` までは約 1 分（`nativeInit`〜`Wails bridge initialized` が約 55 秒）、`index.html` の配信は起動の約 60〜75 秒後 |
| 一覧の表示（シードの 3 スレッド） | **確認できなかった**。この環境では WebView のレンダラが、`index.html` を受け取った後・スクリプトを要求する前に落ちる（下記） |
| 返信下書き | **確認できなかった**（一覧に到達できないため）。Go 側は `go test -tags e2e ./internal/e2e/...`（実エンジンの同期で一覧 3 件・`RecordingGmail` の書き出し）と `internal/app/apptests/thread_draft_test.go` で確認 |
| **レンダラが落ちてもアプリ全体が終了しない** | **確認した（自然に再現）**。1 回目の実行で、`sandboxed_process` のレンダラが `SIGTRAP`（`CrRendererMain`、`libmonochrome_64.so` 内）で**4 回続けて**落ちた。そのたびに `WebView renderer crashed`／`Recreating the WebView and reloading the app` が出て WebView を作り直し、**アプリのプロセス（pid 2759）は終了せず**、4 回目に上限（5 分に 3 回）に達して `WebView renderer kept dying; the recovery limit is reached; showing the error screen`（エラー画面）。ログに `Render process crash wasn't handled` は出なかった（テンプレートのままなら 1 回目でアプリ全体が終了していた）。アプリの pid は 20 分以上同じだった |
| 作り直したあとにアプリが表示される | **確認できなかった**。この AVD ではレンダラが毎回、同じ場所で落ちる（4 回とも起動から約 80 秒、コードは同一）。クラッシュダンプの生成（crashpad）にも約 1 分かかり、その間システムの「応答なし」ダイアログが前面に出た。**原因は未特定**（Chromium のリリースビルドでシンボルなし。アプリを入れ直した再実行でも同じ場所で落ちた。CPU を `-qemu -cpu max` に変えた再起動の 2 回目の実行では、約 7 分の間レンダラの落ち自体は出なかったが、システムアプリの「応答なし」で画面が隠れ、ページの読み込みも進まなかったので、解消したとは言えない。KVM つきの CI（`android-e2e.yml`）か実機で再確認が要る） |
| 手動のレンダラ `kill -9` の実験 | **実行できなかった**（上記のとおり、レンダラが自力で落ち続けるため、`kill` で復帰を確かめる状態に到達しなかった）。`run.sh` の手順 4 に実装してあるが、実行して確認していない |
| 初回の 10 秒タイムアウトからの読み直し | 今回の実行では 10 秒のタイムアウトは出なかった。`onReceivedError` の読み直しは実行して確認していない |
| `android:e2e:run`（`run.sh` の全体） | インストール・起動・プロセスの存在までは PASS。一覧以降は上記の理由で通っていない（1 回目の実行は一覧の待ちが 900 秒で打ち切り。2 回目はエミュレーターの「応答なし」ダイアログ（Pixel Launcher）で画面が隠れたため中断。2 回目のあとに、このダイアログで「待つ」を選ぶ処理を `run.sh` に足したが、**その後は再実行していない**） |

**この環境で E2E の最後まで通せなかったこと**を正確に記すと、「起動（プロセス・Go の配信）まで」は通り、「一覧・下書き・復帰」は**エミュレーターでは未確認**。実機（arm64）・KVM つきのエミュレーターでの通しは**未確認**で、受け入れ基準「実機で起動・一覧表示・返信下書きまで通る」は実機が要る。

**自動で確認したもの**: `task android:test`（JVM。`RecoveryBudget`・`RenderGoneReason` を含む）、`go test ./...`・`go test -tags e2e ./internal/e2e/...`、`golangci-lint`（ホスト・`e2e` タグ・`android`）、`task android:package`＋`android:verify`（release APK に E2E の印が無いことを含む）、`actionlint`、`shellcheck`。

### 17.7 残課題

- **実機（arm64）・KVM つき環境で、E2E を最後まで通す**（一覧・返信下書き・レンダラ `kill` からの復帰）。Actions の `android-e2e.yml` を手動実行して緑になることも未確認。
- このホストの KVM なしエミュレーターでレンダラが落ち続ける原因の特定（実機・KVM つきでも起きるかの確認。起きるなら、`onRenderProcessGone` の上限を超えてエラー画面になる＝アプリは終了しないが使えない）。
- WebView を作り直したときの未保存の入力（約 3 秒分）と、Go に残る作成セッション（17.1）。
- Activity が OS に破棄・再生成された場合（`configChanges` に含まれない変更）に、テンプレートの `onCreate` が新しい `WailsBridge` の `nativeInit` を再度呼ぶ点（Wails のテンプレートの挙動で、この Issue では変えていない）。
- 実機での起動時間・同期時間の計測（17.5 はホストとエミュレーターのみ）。

## 18. AI の既定のプロバイダと ChatGPT ログイン（#104）

**Android でも Windows と同じく、AI の設定が未保存のときの既定のプロバイダを ChatGPT プランにし、「AI で下書き」の初回の案内も同じに出す**（[F-08](../requirements/functional.md)・[画面の流れ](../components/overview.md#391-ai-設定と初回の案内プロバイダ選択同意モデル一覧の流れ104105)）。Android 専用の既定・案内は作らない。

調べたこと（2026-11-04。コードの確認で、実機は使っていない）:

| 項目 | 結果 |
|------|------|
| ChatGPT ログインの実装（`internal/siwc`） | OS ごとの分岐（ビルドタグ・`runtime.GOOS`）が無い。`127.0.0.1` の一時ポートでコールバックを受け取り（`callback.go`）、PKCE・`state` を使う。Google OAuth（§14）と同じ方式で、Android でも同じコードが動く |
| ブラウザを開く | `run.go` が `platform.NewBrowser(wapp).Open` を渡す。Android では Intent（ACTION_VIEW）で既定のブラウザを開く（`internal/platform/open_android.go`）。`xdg-open` は使わない |
| トークンの保管 | `secrets.Store`。Android は EncryptedSharedPreferences（§11）。ChatGPT のトークン（4096 バイトを超える JWT を含む）は §11 で往復を確認している（自動テスト） |
| 通信先 | Windows と同じ（`auth.openai.com`・`api.openai.com`）。Android のネットワーク権限は既存の `INTERNET` で足りる |

したがって、Android でも ChatGPT ログインは**コード上は使える**ので、既定も案内も Windows と同じにする。

- **未確認（実機）**: 実機の Chrome などで OpenAI のログイン画面が開き、`http://127.0.0.1:<ポート>/auth/callback` へ戻って完了するか（§14.1 と同じく、本物の OpenAI と実機のブラウザは使っていない）。ブラウザが前面に出ている間にアプリのプロセスが止められたときは、ログインの待受が消えて完了できず、時間切れかキャンセルとして終わる（§14.3 と同じ理由。Google OAuth のような「途中で止められた」印と案内は、ChatGPT ログインには無い。再度「Continue with ChatGPT」を押せば最初からやり直せる。トークン・登録は完了するまで書き込まれない）。
- **ログインできないとき**: ログインのエラーを表示し、案内の「別のプロバイダを選ぶ」（設定画面の AI タブ）で Anthropic・OpenAI 互換の API キーを使える。既定が ChatGPT でも、保存済みのプロバイダ（anthropic・openai・chatgpt）の設定は変えない。
- Android の画面（§16）でも、初回の案内・同意の画面は同じダイアログで、1 画面ずつの幅のときも中央のモーダル（幅は画面幅 − 32px）で出る（`BaseDialog`。外側のクリックで閉じる）。
