# Android ビルド

> 関連: [環境構築](setup.md) / [プロジェクト概要](../overview.md) / [アーキテクチャ設計](../architecture/overview.md)

本書は Android 版（`jp.ousias.hitomado`）の APK／AAB をビルドする手順と、ビルド基盤の方針（SDK の版・署名・版番号・バインディング・manifest・CI）を定める（Issue #82）。Wails v3 の Android テンプレート（`build/android/`）をもとにしている。ストレージの保存先（#85）、通知（#88）、スマホ向けの画面（#87）、`modernc.org/libc` の android/amd64 問題は §12（#83）、Google OAuth のリダイレクト方式は §14（#86）で決めた。

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
- 36 では、エッジ・ツー・エッジの強制など targetSdk 36 の挙動変更が入る。UI への影響は §16（スマホ向けの画面。safe area は Activity が insets を当てる）で扱う。

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

- **x86_64（`ARCH=amd64`）は、パッチ済みの `modernc.org/libc` でビルドする**（§12）。パッチなしだと SQLite の最初のファイル操作で `SIGSYS` で落ちる。arm64（実機）はこの問題の対象外で、アプリ内の SQLite の実行は**実機で未確認**（§12.3）。
- Android の最小実装（アプリが起動してクラッシュしないための土台）: `dataDir` は `getFilesDir()` 配下（`datadir_android.go`。保存先は §13）、資格情報は EncryptedSharedPreferences（`internal/secrets/keyring_android.go`・`SecureStore.java`。§11）、通知サービスは未登録で `Android.Notify` を直接呼ぶ（`internal/platform/toast_android.go`。#88）、保存ダイアログは Storage Access Framework（§13）。
- `build/android/` は Wails のテンプレートを取り込んだもので、`.linterly.yml` の対象外。Gradle は `build/android/gradlew`（Gradle 9.2.1）を使う。
- `build/android/overlay.json`・`build/android/gen/`・`jniLibs/*/libwails.so` などの生成物は git 管理外（`.gitignore`）。

## 10. Android で動かさない機能

配布は Google Play で、画面もタッチ前提のため、Android では次の機能を**コード上も動かさず、設定画面にも出さない**（Issue #89。Windows 版の動きは変えない）。

| 機能 | Android | どう止めるか |
|------|---------|--------------|
| MCP サーバー（[docs/mcp.md](../mcp.md)） | 動かさない | `MCPService.Start` が待ち受けない（DB に PC で保存した「オン」の設定があっても）。`MCPService` はバインディングへ登録しない。設定画面の MCP タブを出さない |
| システムトレイ・× でトレイへ格納 | 作らない | `run.go` が `platform.NewTray` を呼ばない。ウィンドウの閉じるフックも付けない（`platform.NewMainWindow`）。通知のバッジ更新は何もしない（`Notifier` のトレイは `nil`） |
| 多重起動の防止 | 設定しない | `application.Options.SingleInstance` を `nil` にする（`singleInstanceOptions`）。Android の起動は OS が管理する |
| ウィンドウの位置・サイズ・最大化の保存復元 | 保存も復元もしない | `WindowKeeper` を作らない（`window.state` を読まず、書かない） |
| アプリ内の自動更新（定期確認・手動確認） | 動かさない | 更新確認ループを起動しない。`AppService` は保存値にかかわらず無効を返し、`CheckForUpdate` は通信しない。`SetUpdateCheckEnabled` は `UNSUPPORTED`。設定画面の更新確認の項目を出さない |

### 仕組み（機能ベース）

- 機能の有無は `internal/capability` の `Capabilities`（`SupportsMCP`・`SupportsTray`・`SupportsAutoUpdate`・`SupportsWindowState`・`SingleInstance`）で表す。値を決めるのは、ビルドタグで分けた薄い 2 ファイル（`current_android.go`＝すべて無効、`current_other.go`＝すべて有効）だけで、判定の中身はタグなしの純粋な関数・値（`Desktop`／`Android`）なので、ホストでテストできる（Android のビルドはホストで実行できない）。
- `run.go` が `capability.Current()` を 1 回だけ読み、`app.Deps.Capabilities` と各部品の生成へ渡す。`Deps.Capabilities` のゼロ値は全機能が無効（設定し忘れて機能が動いてしまうことを避ける）。
- フロントへは `PlatformService.GetCapabilities`（[バインディング仕様 §7.3](../api/endpoints.md#73-platformservice)）で渡す。画面は OS 名を比べず、`platformStore`（`frontend/src/stores/platform.ts`）の `supportsMcp` などで出し分ける（スマホ向けの画面 §16 も `touchUi` で切り替える）。読み込み前・失敗時はデスクトップ（全機能あり）の値。
- **バインディングは OS で変えない**（§7）。`PlatformService` は全 OS で同じシグネチャで、値だけが変わる。MCP は登録の有無だけが変わるが、型は同じで、バインディングは既定のタグ（`purego`）で 1 つだけ生成する。
- 通知の扱いは #88、添付・ログの保存先は §13。ここでは対象外。

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
| API レベル | minSdk 21 のうち、暗号化ストアを使えるのは **API 23 以上**。API 21〜22 は `unavailable` で保存・読み出しとも失敗する（資格情報を平文で置くフォールバックは作らない） |
| 削除 | 無いキーの削除はエラーにしない（`secrets.Store` の契約） |

### 11.2 security-crypto の版（alpha06 → 1.1.0）

| 版 | 状態 | 内容 |
|----|------|------|
| 1.0.0（2021-04） | 安定。**minSdk 23** | `MasterKey` クラスが無い（`MasterKeys`〔deprecated〕のみ）。minSdk 21 のこのアプリでは、マニフェストのマージで minSdk の衝突になる |
| 1.1.0-alpha06（2023-04。旧） | alpha | `MasterKey` あり、minSdk 21。Tink 1.8.0 |
| 1.1.0-alpha07／beta01（2025） | — | **全 API を deprecated にしただけ**（機能の変更なし。プラットフォームの API と Android Keystore の直接利用へ移行する方針） |
| **1.1.0（2025-07-30）** | **安定（採用）** | alpha07 以降と同じ。minSdk 21、`MasterKey` あり |

- **1.1.0 を採用する**。alpha を外せること、minSdk 21 を保てること（1.0.0 は minSdk 23 になり、`MasterKey` も無い）、保存形式が alpha06 と同じで互換が切れないことが理由。出典: [AndroidX Security のリリースノート](https://developer.android.com/jetpack/androidx/releases/security)。
- **deprecated であることの扱い**: 1.1.0 は全 API が deprecated（置き換えは Android Keystore を直接使った AES-GCM）だが、安定版として公開済みで、ここでの使い方（`MasterKey` ＋ `EncryptedSharedPreferences.create`）は動く。`SecureStore` の本番の保存先（`AndroidBackend`）に `@SuppressWarnings("deprecation")` を付け、deprecated な API の使用をこの 1 クラスに閉じ込めてある。Keystore の直接利用へ移す場合も、`SecureStore.Backend` を差し替えるだけで済む（呼び出し側の Go・WailsBridge は変えない）。移行は、security-crypto に不具合・非互換が出たとき、または targetSdk の引き上げで動かなくなったときに行う。今は行わない（自前の暗号は検証の負担が大きい）。
- 版の更新は `build/android/app/build.gradle` の 1 行。

### 11.3 Keystore の鍵が失われたとき

バックアップ復元・端末の変更・画面ロックのリセット・鍵の無効化などで、保存済みの値を復号できなくなることがある（`InvalidKeyException`／`KeyPermanentlyInvalidatedException`／`AEADBadTagException`／`UnrecoverableKeyException`／`KeyStoreException`／`ProviderException`、Tink のキーセットの破損〔`InvalidProtocolBufferException`〕。EncryptedSharedPreferences は復号の失敗を `SecurityException` などに包むため、`cause` の連鎖をたどって判定する）。

| 層 | 動作 |
|----|------|
| Java（`SecureStore`） | 暗号化ストア（ファイルと Keystore の鍵）を消して作り直し、**起動は落とさない**。読み出しは `key_lost` を返す。書き込み・削除で見つけたときは、作り直したうえで続きを実行する（保存は 1 回だけ再試行）。Keystore が一時的に使えないだけの失敗（`KeyStoreException.isTransientFailure()`）や、ディスクの `IOException` だけの失敗は鍵の喪失とみなさず、ストアも消さない（`failed`。資格情報を消すのは確実なときだけ） |
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
- この Issue では、コード（`SecureStore` が `Context` を引数に取り Activity に依存しないこと）と JVM 単体テスト（`SecureStoreTest.concurrentUseFromBackgroundThreadsIsSerialized`。Activity・Context なしのスレッドから同じ `SecureStore` を並行に使える）で示す。Worker からの実際の呼び出しは #88 で入れる。

### 11.5 ログに出さない

- Go: `Store` のエラーは `secrets: <操作> "<キー名>": <Java の失敗コード>` だけで、値を含まない（キー名は `hitomado/<種別>/<ID>` で秘密ではない）。`slog` に値を渡している箇所は無い（`internal/` を grep して確認）。
- Java: `SecureStore`／`WailsBridge` の失敗コードは `key_lost`／`unavailable`／`failed`／`empty key` だけで、例外のメッセージ（値を含みうる）は返さず、logcat にも例外のクラス名だけを出す。
- **Wails のテンプレートが debug ビルドで logcat に出していた JS からの呼び出しの本文を、長さだけに変えた**（`WailsBridge.handleMessage`／`handleRuntimeCall`、`WailsJSBridge.invoke`／`invokeAsync`）。バインディング呼び出しの引数（`SetAIApiKey` の API キー、`AddOAuthClient` の Client Secret）が本文に入るため。release ビルド（`BuildConfig.DEBUG` が偽）は元から出さない。`MainActivity` の `Wails API call`（パスとクエリだけ）、WebView のコンソールのメッセージ（debug のみ）は、フロントが `console.error(err)` を出す 1 か所（`errorReporter.ts`。AppError のメッセージで、値は含まない）を除き、値を出す箇所が無い。
- Go 側の Wails のフレームワークのログ（`[JS message]`、`runtime call returned status`）は `production` タグで無効になる（release のビルドは `-tags production,android`）。debug ビルド（`android:run`）では `[JS message]`（ウィンドウのメッセージ）が logcat に出る。

### 11.6 確認の手順（実機・エミュレーター）

自動で確かめたのは、Go と Java の単体テスト・ビルド・`android:verify` まで。次は実機・エミュレーターが要る（未確認）:

1. 往復: アカウントを接続（または設定で AI の API キーを登録）→ アプリを終了して再起動 → 接続・キーが残っている。`adb shell run-as jp.ousias.hitomado cat shared_prefs/hitomado_secrets.xml`（debug ビルド）に、トークンが**平文で無い**（キーも値も暗号化されている）。
2. ログ: `adb logcat -d | grep -i -E "token|ya29|1//|sk-"` でトークン・キーが出ない（debug ビルドで、接続・キー登録・同期のあと）。
3. 鍵の喪失: debug ビルドで接続 → `run-as` で `shared_prefs/hitomado_secrets.xml` を端末の外へ退避 → `adb shell pm clear jp.ousias.hitomado`（Keystore の鍵も消える）→ 一度起動して終了 → 退避したファイルを `run-as` で書き戻す → 起動。アカウントが「要再接続」になり、再接続で復旧する。logcat に `Keystore key lost; resetting secure storage (<例外クラス名>)` が出る。
4. Worker: #88 の実装後に、Activity が無い状態（`adb shell am force-stop` のあとの定期実行）でトークンを読めること。

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
| 前面サービス（`WailsForegroundService`、`dataSync`）でログイン中だけプロセスを生かす | しない。通知・前面サービスの扱いは #88 で決める（§6）。プロセスが止められる確率を下げるだけで、止められた後の扱いは結局必要。#88 で前面サービスを入れる場合の候補として残す |
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
| `frontend/test/accountsInterrupted.test.ts` | 検出時にやり直しのトーストを 1 回だけ出す。呼び出しの失敗で起動を止めない |

### 14.6 実機で確認する手順（未確認）

**この Issue では、実機と本物の Google アカウントを使っていないため、次は未確認。** 自動テストで確かめたのは偽 IdP 越しの Go のフローとプロセス停止の検出までで、本物の Google に対するフローと、Android の挙動は確認していない。

1. README の手順で、**デスクトップ アプリ種別**の OAuth クライアントを作り（同意画面が外部・「テスト」ならテストユーザーに自分の Gmail を登録）、Client ID と Client Secret を実機のアプリの設定 → OAuth クライアントに登録する。
2. 設定 → アカウントを追加 → 認可を開始。ブラウザが開き、Google の同意画面が出る（`redirect_uri` が拒否される `invalid_request` や `redirect_uri_mismatch` にならない）。
3. 同意すると、ブラウザに「認可の操作が完了しました。このタブを閉じて Hitomado に戻ってください」が出る（127.0.0.1 へ届いた）。アプリへ戻ると、アカウントが追加されて同期が始まる。
4. 追加したアカウントのスレッドを返信で開き、送信元がそのアドレスに固定され、変更できないこと。
5. 別の鍵（debug／release）の APK に入れ替えたあとも、同じ Client ID で認可できること（SHA-1 を登録していないため）。
6. プロセス停止: 認可を開始してブラウザが出たら、`adb shell am kill jp.ousias.hitomado`（バックグラウンドのプロセスだけを終了する）でアプリを止め、ブラウザで同意（`127.0.0.1` への接続は失敗する）。アプリを開くと「ログインの途中でアプリが終了したため…やり直してください」のトーストが出る。もう一度アカウントを追加して完了できる。debug ビルドなら `adb shell run-as jp.ousias.hitomado cat shared_prefs/hitomado_secrets.xml` に平文のトークン・`state` が無いこと。
7. Google がモバイルのループバックを拒否した場合（§14.1 のリスク）: 認可画面に `invalid_request` が出る。この結果を Issue に記録し、(b) の `AuthorizationClient` 方式を別 Issue で起こす。

## 16. スマホ向けの画面（#87）

画面のコードは Windows 版と共通（同じ Vue）。幅による 3 ペイン／2 ペイン／1 画面ずつの切り替え、戻る、タッチ、のフロント側の設計は [コンポーネント設計 §5.1](../components/overview.md#51-画面幅によるレイアウトの切り替えandroid)、画面の仕様は [機能要件 §5.1](../requirements/functional.md#51-androidスマホの画面)。ここでは Android 側（Go・Java）の取り決めを書く。

### 16.1 OS による切り替え（`TouchUI`）

- `capability.Capabilities.TouchUI`（JSON `touchUi`）を足した。`Android()` だけ `true`、`Desktop()`・ゼロ値は `false`。フロントは `platformStore.touchUi`（OS 名ではなく機能）で、幅による切り替えと `styles/touch.css`（タップ領域 44px 以上）を有効にする。Windows 版は `false` なので、幅に関わらず今の 3 ペインのまま。
- **最小幅 960px の制限を Android では外す**: 制限は 2 か所にあった。①Go の `internal/platform` の `MinWidth`・`MinHeight`（ウィンドウ作成時の `MinWidth`／`MinHeight`、保存した状態の補正 `windowstate.go`）、②フロントの `.shell { min-width: 960px }`（`AppShell.vue`）。①は `platform.MinWindowSize(caps)` が `TouchUI` の OS では 0（制限なし）を返す。保存した状態の補正は Android では `WindowKeeper` を作らないので効かない。②は `layout-wide`（3 ペイン）のときだけ付ける。なお Wails の Android 実装（`webview_window_android.go`）の `setMinSize` は何もしないので、①は意図を明示するためのもの。

### 16.2 safe area（エッジ・ツー・エッジ）

- targetSdk 36 では、Activity のウィンドウがステータスバー・ナビゲーションバーの背後まで広がる（エッジ・ツー・エッジが強制される）。テンプレートの `MainActivity` は insets を扱っておらず、そのままでは WebView の上端・下端がバーに隠れる。
- **Activity 側で insets を当てる方式を選んだ**（`MainActivity.applyWindowInsets()`）。system bars・display cutout・IME の insets を、WebView を入れた `main_container` の padding に当てる。CSS の `env(safe-area-inset-*)` と `viewport-fit=cover` は使わない: Android の WebView では `env()` の値が端末・WebView のバージョンで 0 のままになることがあり、信頼できないため。Java で 1 か所に当てれば、フロントの CSS は safe area を知らなくてよい（`index.html` の viewport も変えない）。IME の insets も同じ padding に含めるので、ソフトキーボードが出たときも入力欄が隠れない（`adjustResize` は 36 のエッジ・ツー・エッジでは効かないため）。
- バーの背景色は `app_background`（`values`＝`#FFFFFF`、`values-night`＝`#14171C`。フロントの `--color-bg` のライト／ダークと同じ値）。バーのアイコンの明暗は `updateSystemBarIcons()` が uiMode に合わせる。フロントの配色を変える（`tokens.css`）ときはここも合わせる。

### 16.3 戻るボタン

- `MainActivity` は `OnBackPressedCallback`（`getOnBackPressedDispatcher()`）で戻るを受ける。テンプレートの `onBackPressed()` の上書きは、targetSdk 36 では呼ばれないので削除した。
- コールバックは `webView.evaluateJavascript` で `window.hitomadoHandleBack()` を呼ぶ。フロントの `lib/androidBack.ts` が `AppShell` で登録する関数で、一つ前の画面へ戻したら `true` を返す（オーバーレイ → ドロワー → 返信・作成 → スレッド → 検索結果の順）。`true` ならアプリは何もしない。`false`・未登録（起動中）・失敗のときは、WebView の履歴があればそれに戻り（`canGoBack`）、無ければ `moveTaskToBack(true)` でアプリをバックグラウンドへ送る（終了しない）。
- Wails のイベント（Java → Go → `Events.Emit`）も `window.history`（`pushState`＋`popstate`）も使わない。前者は Go を経由する分遅れて「消費したか」を Java へ返せず、後者は履歴の積み方と `canGoBack` が一致しないとアプリが終了するため。1 つの関数の戻り値で「消費したか」を同期的に受け取る方が確実。

### 16.4 確認の状況

- ブラウザ（vite dev・Wails ランタイムをモック）で 390x844・800x900・1440x900 の画面を確認した（スクリーンショットは作業ログ）。`bun test` は、1 画面ずつの遷移（一覧 → スレッド → 返信 → 戻る）、幅を変えても選択・下書きが残ること、戻るの順序、受け口の登録を確認する。
- **未確認**: 実機・エミュレーターでの表示（safe area・戻るボタン・ソフトキーボード・Fold の折り畳み）。Java は `task android:package`／`android:verify` でビルド・検査できるが、動作はエミュレーターか実機で確認が要る。
