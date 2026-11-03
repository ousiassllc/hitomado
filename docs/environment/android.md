# Android ビルド

> 関連: [環境構築](setup.md) / [プロジェクト概要](../overview.md) / [アーキテクチャ設計](../architecture/overview.md)

本書は Android 版（`jp.ousias.hitomado`）の APK／AAB をビルドする手順と、ビルド基盤の方針（SDK の版・署名・版番号・バインディング・manifest・CI）を定める（Issue #82）。Wails v3 の Android テンプレート（`build/android/`）をもとにしている。ストレージの保存先（#85）、通知（#88）、`modernc.org/libc` の android/amd64 問題は §12（#83）で決めた。

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

- **x86_64（`ARCH=amd64`）は、パッチ済みの `modernc.org/libc` でビルドする**（§12）。パッチなしだと SQLite の最初のファイル操作で `SIGSYS` で落ちる。arm64（実機）はこの問題の対象外で、アプリ内の SQLite の実行は**実機で未確認**（§12.3）。
- Android の最小実装（アプリが起動してクラッシュしないための土台）: `dataDir` は `getFilesDir()` 配下（`datadir_android.go`。保存先の本実装は #85）、資格情報は EncryptedSharedPreferences（`internal/secrets/keyring_android.go`・`SecureStore.java`。§11）、通知サービスは未登録で `Android.Notify` を直接呼ぶ（`internal/platform/toast_android.go`。#88）、保存ダイアログは `files/exports` へ保存（`savefile_android.go`。#85）。
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
- フロントへは `PlatformService.GetCapabilities`（[バインディング仕様 §7.3](../api/endpoints.md#73-platformservice)）で渡す。画面は OS 名を比べず、`platformStore`（`frontend/src/stores/platform.ts`）の `supportsMcp` などで出し分ける（モバイル UI の Issue #87 も同じ値を使う）。読み込み前・失敗時はデスクトップ（全機能あり）の値。
- **バインディングは OS で変えない**（§7）。`PlatformService` は全 OS で同じシグネチャで、値だけが変わる。MCP は登録の有無だけが変わるが、型は同じで、バインディングは既定のタグ（`purego`）で 1 つだけ生成する。
- 通知の扱い・保存先は #88・#85。ここでは対象外。

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

