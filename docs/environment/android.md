# Android ビルド

> 関連: [環境構築](setup.md) / [プロジェクト概要](../overview.md) / [アーキテクチャ設計](../architecture/overview.md)

本書は Android 版（`jp.ousias.hitomado`）の APK／AAB をビルドする手順と、ビルド基盤の方針（SDK の版・署名・版番号・バインディング・manifest・CI）を定める（Issue #82）。Wails v3 の Android テンプレート（`build/android/`）をもとにしている。ストレージの保存先（#85）、資格情報の強化（#84）、通知（#88）、`modernc.org/libc` の android/amd64 問題（#83）は別の Issue が扱う。本書の対象外。

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
| `task android:lint` | android タグ側の Go lint（§7） | — |
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
- 36 では、エッジ・ツー・エッジの強制など targetSdk 36 の挙動変更が入る。UI への影響は #83 以降のモバイル UI で確認する（ここではビルドが通ることまで）。

`minSdk` は 21（テンプレートの既定）。`Taskfile.yml` の `MIN_SDK` は NDK の clang の API レベル（`aarch64-linux-android21-clang`）で、`app/build.gradle` の `minSdk` と同じ値にしておく。

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

- 置く権限: `INTERNET`、`ACCESS_NETWORK_STATE`（ネットワーク変化の通知）、`VIBRATE`、`POST_NOTIFICATIONS`、`FOREGROUND_SERVICE`、`FOREGROUND_SERVICE_DATA_SYNC`（最後の 3 つは Wails のテンプレート由来。**通知・前面サービスの扱いは #88 で決める**ため、ここでは増やさず消しもしない）。
- 削除した権限: `CAMERA`、`ACCESS_FINE_LOCATION`、`ACCESS_COARSE_LOCATION`、`USE_BIOMETRIC`。`androidx.biometric`（`WailsBridge.java` が参照する）がマージ時に `USE_BIOMETRIC`／`USE_FINGERPRINT` を足すため、`tools:node="remove"` で取り除く。カメラ用の `<queries>` も削除した。
- `allowBackup="false"`、`fullBackupContent="false"`、`dataExtractionRules`（`res/xml/data_extraction_rules.xml`。クラウドバックアップ・端末間移行ともすべて除外）。DB（メール本文のメタデータを含む）と、Android Keystore の鍵に紐づく暗号化済み資格情報は、別の端末に復元しても使えないため。

`task android:verify` は `aapt2 dump badging`／`dump xmltree` と `apksigner verify --print-certs` で、packageName、versionName、versionCode、minSdk（21）、targetSdk（36）、権限が**許可リスト**に収まること（新しい権限が入ると失敗する。意図して増やすときは `verify-apk.sh` の許可リストと manifest を一緒に直す）、`allowBackup=false`、`dataExtractionRules` の有無、debuggable でないこと、署名の種類（`EXPECT_SIGNING=debug|release` を付けるとその種類でなければ失敗）を確認する。

AAB の検査には `bundletool` が要る（SDK に含まれない）。`https://github.com/google/bundletool/releases` の `bundletool-all-<版>.jar`（CI は 1.18.3）を取得して `java -jar bundletool-all-1.18.3.jar validate --bundle bin/hitomado.aab` を実行する。`dump manifest --bundle` で manifest を見られる。

## 7. バインディングと生成物の方針

`wails3 generate bindings -tags android` は、Android で通知サービスを登録しない（`notifsvc_android.go`）ため `frontend/bindings/…/services/notifications/*` を消す。`task check` と CI の「Generated files are up to date」（`wails3 generate bindings -f '-tags purego'` → `git diff --exit-code`）を壊さないよう、**バインディングはデフォルトのタグ（`purego`）でしか生成しない**。

- Android のビルド（`android:*`）はバインディングを再生成しない。テンプレートの `generate:android:bindings` タスクは削除し、`android:build` のフロントのビルド（`build:frontend`）は共通の `common:build:frontend`（バインディングを再生成する）を使わず、`bun run build` だけを実行する。
- フロントは通知サービスのバインディングを参照していない（`frontend/src` に参照なし）。Android で通知サービスが無くてもフロントのビルドは通る。
- 生成物（`frontend/bindings`）を変えるのは `task generate`（既定のタグ）だけ。Android 用の生成物は git 管理に入れない。

## 8. CI（`ci.yml`）

- `lint` に `Go lint (android)` を追加（`GOOS=android GOARCH=arm64 CGO_ENABLED=0 golangci-lint run`）。`*_android.go`・`!android` 側の型検査と lint が対象になる。手元では `task android:lint`（`build/android/gen` は overlay の生成物で単体では型検査に失敗するため、先に消してから実行し、次の `android:build` が作り直す）。
- `android` ジョブ（ubuntu-latest、**pull_request のみ**。`release.yml` が呼ぶとき（push）は動かさない）: JDK 17、SDK platform 36・build-tools・NDK を `sdkmanager` で入れ、wails3 は `WAILS_VERSION`。デバッグ鍵（フォールバック）の APK を検査したあと、使い捨ての鍵（`keytool` で作り、パスワードは `::add-mask::` で伏せ、ジョブの中だけに存在）で release APK（arm64）と AAB（arm64＋x86_64）を作って `android:verify`・`bundletool validate` し、`hitomado-android` のアーティファクトに上げる（署名は使い捨ての鍵。実鍵は使わない）。
- Gradle のキャッシュは明示していない（初回は Gradle 本体の取得を含む）。

## 9. 既知の事項

- **x86_64（`ARCH=amd64`）はビルドできるが、アプリ内では起動しない**: `modernc.org/libc` の android/amd64 実装が旧式の syscall を直接呼び、Android の seccomp に拒否される（`SIGSYS`）。#83 が扱う。arm64（実機）はこの問題の対象外と見ているが、実機では未確認。x86_64 のエミュレーターでの起動確認は #83 の後。
- Android の最小実装（アプリが起動してクラッシュしないための土台）: `dataDir` は `getFilesDir()` 配下（`datadir_android.go`。保存先の本実装は #85）、資格情報は EncryptedSharedPreferences（`internal/secrets/keyring_android.go`。強化は #84）、通知サービスは未登録で `Android.Notify` を直接呼ぶ（`internal/platform/toast_android.go`。#88）、保存ダイアログは `files/exports` へ保存（`savefile_android.go`。#85）。
- `build/android/` は Wails のテンプレートを取り込んだもので、`.linterly.yml` の対象外。Gradle は `build/android/gradlew`（Gradle 9.2.1）を使う。
- `build/android/overlay.json`・`build/android/gen/`・`jniLibs/*/libwails.so` などの生成物は git 管理外（`.gitignore`）。
