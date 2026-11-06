# アーキテクチャ設計

> 関連: [プロジェクト概要](../overview.md) / [機能要件](../requirements/functional.md) / [非機能要件](../requirements/non-functional.md) / [ER](er.md) / [バインディング仕様](../api/endpoints.md) / [コンポーネント設計](../components/overview.md)

## 1. 設計方針

- **サーバーレス**: 公開者のサーバーを持たない。すべての通信は利用者の PC から Google／AI プロバイダ／GitHub API へ直接行う
- **薄いフロント・厚い Go**: 認証・同期・MIME 生成・送信元の強制など、安全性に関わる処理はすべて Go 側に置く。フロントエンドは表示と入力に専念し、外部へ直接通信しない（CSP で禁止）
- **送信元の強制は Go 側が最終防衛線**: UI の制御を迂回しても、返信・転送の送信元は受信アカウント以外にならない
- **アカウント単位の隔離**: 同期・レート制限・エラー状態はアカウントごとに独立させる
- **機密はローカルに平文で残さない**: 秘密情報は OS 資格情報ストア、メール本文・添付はディスクに保存しない

## 2. 技術選定

| 領域 | 採用 | 理由 |
|------|------|------|
| デスクトップ基盤 | **Wails v3**（v3.1.0。版は `go.mod` で固定） | システムトレイ・ネイティブ通知（クリック応答つき）・単一インスタンス制御を標準 API で扱える（F-10）。Go バインディングの TypeScript 自動生成（`wails3 generate bindings`） |
| バックエンド言語 | Go | Gmail API 公式クライアント・単一バイナリ配布 |
| メール API | Gmail API（`google.golang.org/api/gmail/v1`） | `history` による差分同期、`threads`／`drafts`／`messages.send` を使う。IMAP は採用しない（OAuth スコープが粗く、スレッド・ラベル・下書きの対応が弱い） |
| OAuth | `golang.org/x/oauth2`＋Google エンドポイント（PKCE・ループバック） | NF-S03 |
| 資格情報ストア | `github.com/zalando/go-keyring`（Windows 資格情報マネージャー）、Android は EncryptedSharedPreferences（`androidx.security:security-crypto` 1.1.0。[Android ビルド §10](../environment/android.md#11-資格情報の保管84)） | NF-S01 |
| ローカル DB | SQLite（`modernc.org/sqlite`、純 Go・CGO なし） | Windows でのビルドが容易（C コンパイラ不要） |
| クエリ生成・マイグレーション | **sqlc**（型安全なクエリ生成）＋ **goose**（SQL マイグレーションを `embed`） | |
| MIME 構築 | `github.com/jhillyerd/enmime`（Builder） | `multipart/alternative`・添付・`In-Reply-To`／`References` |
| レート制限 | `golang.org/x/time/rate` | アカウントごとのクォータ制御（NF 同期） |
| ログ | 標準 `log/slog`（JSON Lines）＋ `lumberjack` によるローテーション。書き込み前に秘密・メール内容を伏せる Handler を通す | NF-S10 |
| MCP | 公式 Go SDK `github.com/modelcontextprotocol/go-sdk`（Streamable HTTP）。既定オフ・Bearer トークン必須・送信ツールなし | NF-S13〜NF-S15、[docs/mcp.md](../mcp.md) |
| AI | 自前の薄い `Provider` インターフェース。Anthropic・OpenAI 互換（Chat Completions）・ChatGPT プラン（Responses API）の 3 実装と、モデル一覧の取得（`ListModels`） | NF §12 |
| ChatGPT ログイン | `internal/siwc`（Sign in with ChatGPT の OSS 向けフロー。動的登録・PKCE・ID トークンの検証は `github.com/coreos/go-oidc/v3`） | NF §2、[API §6.2](../api/endpoints.md#62-chatgpt-プランsign-in-with-chatgpt) |
| フロントエンド | **Vue 3＋TypeScript**（Composition API／`<script setup>`）、Vite、Pinia | |
| リッチテキスト | Tiptap（ProseMirror）。太字・リンク・箇条書き | F-11 |
| 仮想スクロール | `@tanstack/vue-virtual` | NF-P04 |
| サニタイズ | DOMPurify＋`sandbox` iframe（`srcdoc`、`allow-same-origin` のみ・スクリプト不可） | NF-S04 |
| テスト | Go: 標準 `testing`／`testify`。フロント: **`bun test`**（DOM は happy-dom、コンポーネントは Vue Test Utils。`.vue` の読み込みは `frontend/bunfig.toml` の preload で行い、`bun test` で SFC を import できる。検証結果は [環境構築 §5.3](../environment/setup.md)） | NF §10 |
| CI／配布 | GitHub Actions（Windows ランナーでビルド）、GitHub Releases | NF-D01〜D04 |

> 補足: Wails v3 の API は公式ドキュメント（v3.wails.io）を一次情報とし、バージョンは `go.mod` で固定している。

## 3. システム構成

```mermaid
graph TD
  subgraph PC[利用者の Windows PC]
    subgraph FE[WebView2 内のフロントエンド Vue]
      V[画面・ストア Pinia]
    end
    subgraph BE[Go バックエンド（単一プロセス）]
      SV[サービス層<br/>Wails バインディング]
      DM[ドメイン層<br/>返信規則・MIME・送信元ポリシー]
      SY[同期スケジューラ]
      INF[インフラ層<br/>Gmail / OAuth / Store / Secrets / AI / Notify / Update]
    end
    DB[(SQLite<br/>メタデータ・スニペット)]
    KS[(Windows 資格情報<br/>マネージャー)]
    TR[システムトレイ・トースト通知]
  end
  V <-->|バインディング呼出し・イベント| SV
  SV --> DM
  SV --> INF
  SY --> INF
  SY --> DM
  INF --> DB
  INF --> KS
  INF --> TR
  INF <-->|HTTPS| GG[Google OAuth / Gmail API]
  INF <-->|HTTPS| AI[AI プロバイダ 利用者のキー／ChatGPT プラン]
  INF <-->|HTTPS| OA[auth.openai.com ChatGPT ログイン]
  INF <-->|HTTPS| GH[GitHub Releases API と Release の添付ファイル 自動アップデート]
```

## 4. レイヤー構造

| レイヤー | 責務 | 依存してよい先 |
|----------|------|----------------|
| **UI**（`frontend/`） | 表示・入力・フォーカス・ショートカット。ビジネス判断をしない | バインディング（生成された TS クライアント）のみ |
| **サービス層**（`internal/app`） | Wails に公開するバインディング。入力検証・DTO 変換・ドメインとインフラの調停 | ドメイン層、インフラ層（インターフェース越し） |
| **ドメイン層**（`internal/domain`） | 返信宛先の決定、送信元ポリシー、署名挿入、引用生成、MIME 組立の入力モデル。**I/O を持たない純粋なロジック** | なし（標準ライブラリのみ） |
| **同期層**（`internal/sync`） | アカウント別スケジューラ、`history` 差分の適用、リセット同期、新着検出 | ドメイン層、インフラ層 |
| **インフラ層**（`internal/gmail`／`oauth`／`store`／`secrets`／`ai`／`notify`／`update`／`platform`） | 外部との I/O。すべてインターフェースで抽象化し、テストではフェイクを注入する。`platform` は Wails の機能（ウィンドウ・トレイ・通知・ダイアログ・イベント発行）への接続層で、`notify` などのインターフェースの Wails 実装を持つ | 標準ライブラリ、外部ライブラリ |

横断のパッケージ: `internal/events`（Go からフロントへ発行するイベントの名前とペイロード）、`internal/clock`（時刻の注入）、`internal/logging`（ログ出力の組み立て。機密を出さない）。いずれもレイヤーには属さず、各層から使う。

### ディレクトリ構成（目安）

```
hitomado/
├─ main.go                 # エントリポイント（run() を呼び、失敗時は致命的エラーを表示して終了）
├─ run.go                  # composition root（Wails アプリ作成・単一インスタンス・依存の組み立て・サービス登録・トレイ生成。機能の有無は capability に従う）
├─ internal/
│  ├─ app/                 # バインディング（サービス層）
│  ├─ domain/              # 純粋ロジック（reply, quote, signature, senderpolicy, confirm, contacts, palette, textconv, model）
│  ├─ sync/                # 同期スケジューラ
│  ├─ gmail/               # Gmail クライアント（interface＋実装）
│  ├─ oauth/               # ループバック＋PKCE フロー
│  ├─ store/               # sqlc 生成コード・クエリ・goose マイグレーション
│  ├─ secrets/             # 資格情報ストア（interface＋keyring 実装）
│  ├─ ai/                  # Provider interface、anthropic、openai-compat
│  ├─ notify/              # 新着通知・トレイバッジの Notifier インターフェース（Wails 実装は platform）
│  ├─ platform/            # Wails 接続層（メインウィンドウ・トレイ・トースト通知・ファイル／ブラウザのダイアログ・イベント発行・終了処理）
│  ├─ events/              # フロントへ発行するイベントの名前とペイロード
│  ├─ clock/               # 時刻の注入（System／Fake）
│  ├─ logging/             # ログ出力（slog＋ローテーション。機密を出さない）
│  └─ update/              # GitHub Releases 確認、Windows 版の自動アップデート（ダウンロード・SHA-256 照合・入れ替えの指示）
├─ frontend/               # Vue 3 + TS（src/components, src/stores, src/lib）
└─ docs/
```

## 5. 実行モデル

- **プロセス**: 単一プロセス。多重起動は Wails v3 の単一インスタンス機能で禁止し、2 つ目の起動は既存ウィンドウを前面に出す。**OS ごとの機能差**（Android では MCP サーバー・トレイ・多重起動の防止・ウィンドウ状態の保存復元・アプリ内の自動更新を動かさない）は `internal/capability` の `Capabilities`（`run.go` が `capability.Current()` で決める。ビルドタグ `android`／`!android`）で表し、同じ値を `PlatformService.GetCapabilities` でフロントへ渡す（画面は OS 名ではなくこの値で出し分ける。[Android ビルド §10](environment/android.md#10-android-で動かさない機能)）
- **ウィンドウ**: メインウィンドウ 1 つ。エディタはメインウィンドウ内のペイン（返信・転送はスレッド下部、新規作成は右ペイン全面）として表示する。独立ウィンドウは v0 では設けない
- **トレイ**: ウィンドウの × は非表示化（トレイへ格納）。トレイメニュー「開く」「終了」。「終了」でのみ同期スケジューラを停止してプロセスを終了する
- **同期**: アカウントごとに 1 ゴルーチン。`time.Ticker`（60 秒）＋手動更新シグナル。`context` でキャンセル可能。アカウント追加・削除に応じて起動・停止する
- **レート制限**: アカウントごとの `rate.Limiter`（クォータユニット単位）。429／5xx は指数バックオフ。ただし `messages.send`／`drafts.create` は二重実行を避けるため 5xx を再試行しない（429 は再試行する）
- **フロントへの通知**: Go から Wails のイベントを発行し、フロントはイベント受信時にバインディングを再呼び出しして再描画する（イベントに本文等の機密は載せない）

### 5.1 自動アップデート（Windows 版。NF-D03）

Windows 版は新しい版を自動でダウンロードして入れ替える。実装は `internal/update`（ダウンロード・照合・入れ替えの指示）と `internal/app/updatecore`（設定・イベント・終了時の入れ替え）。

- **通信先**: 更新確認は `api.github.com`（`releases/latest`。応答の `assets` から添付ファイルの URL を得る）。ダウンロードは `github.com/ousiassllc/hitomado/releases/download/vX.Y.Z/<ファイル名>`（`hitomado-vX.Y.Z-windows-amd64-setup.exe` と `SHA256SUMS.txt`）で、配信元（`release-assets.githubusercontent.com`。以前は `objects.githubusercontent.com`）へ 302 でリダイレクトされる。接続できるのは HTTPS で、`github.com` と `*.githubusercontent.com` だけ（リダイレクト先も同じ検査。リダイレクトは 5 回まで）。署名つきのリダイレクト先 URL のクエリはログ・エラー文面に残さない
- **ダウンロードと照合**（`update.Stager`）: ①添付に `hitomado-vX.Y.Z-windows-amd64-setup.exe`（`X.Y.Z` は `tag_name` から。数字 3 つだけの版）と `SHA256SUMS.txt` が揃っているか確認（無ければ何も取らずに失敗）→ ②`SHA256SUMS.txt`（64 KiB まで）から該当の行（`<64 桁の 16 進>  <ファイル名>`）を読む → ③インストーラーを保存先 `%APPDATA%\Hitomado\updates`（権限 0700）の一時ファイル（`download-*.tmp`）へ保存しながら SHA-256 を計算（サイズ上限 200 MiB・宣言サイズ〔`assets[].size`〕との一致・全体のタイムアウト 10 分）→ ④ハッシュが一致したときだけ確定名（`hitomado-vX.Y.Z-windows-amd64-setup.exe`）へ移す。一致しなければ一時ファイルを消して `ErrChecksumMismatch`（確定名のファイルは作らない・入れ替えの対象にしない）。同じ版の照合済みファイルが既にあれば取り直さない（ハッシュが変わっていれば消して取り直す）。`SHA256SUMS.txt` は同じ Release の添付で、署名されていない（NF-D02）。このハッシュ照合は、転送の破損と配信経路の差し替えを検出するもので、Release 自体が乗っ取られた場合は防げない（信頼の根は GitHub と公開者の Release で、手動インストールと同じ）
- **状態**（`update.Manager`）: 「ダウンロード中」「入れ替え待ち（照合済み）」「失敗」「なし」を持ち、変わるたびに `update:status` を発行する（§発行イベント）。新しい版が見つかる（起動時・24 時間ごと・手動確認）たびに、自動アップデートが有効ならバックグラウンドでダウンロードを始める（確認の応答は待たせない）。同じ版が入れ替え待ちなら何もしない。より新しい版が見つかれば古いファイルを消す。起動時に、途中のダウンロードと、今の版以下のインストーラーを保存先から消す
- **入れ替えのタイミング（既定）**: ダウンロードと照合が済んだら、設定画面（一般）と通知領域（バナー）に「再起動して更新」を出す。**押さなくても、次にアプリを終了するときに入れ替える**（どちらも下の安全条件つき）。設定でオフなら、どちらも行わない
- **安全条件**: 送信（`Send`）・下書きの保存（`SaveDraft`）・下書きの削除（`DiscardCompose`）は、進行中を `deps.Busy`（カウンタ）に記録する。入れ替えを指示する関数（`Manager.RequestRestart`／`ApplyOnExit`）は、注入された「いま安全か」（`Busy.Active() == false`）を見て、進行中なら `ErrBusy` を返して入れ替えない（「再起動して更新」は `VALIDATION`、終了時は見送って次の終了に回す）。入れ替えの直前に、保存済みのファイルの SHA-256 をもう一度照合し、一致しなければ破棄して入れ替えない
- **入れ替えの方法（NSIS インストーラーのサイレント実行）**: 実行中の `hitomado.exe` は上書きできず、インストーラーは起動中だと終了コード 5 で中止する（[setup.md §4.3](../environment/setup.md#43-windows-インストーラー)）。そのため、アプリが終了処理（同期・MCP・更新確認ループの停止、ウィンドウ状態の保存）を済ませたあと、`wapp.Quit()` の直前に、インストーラーを**アプリから分離した子プロセス**（`DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP`・ウィンドウなし。親が終了しても生き続ける）として起動し、アプリは終了する。引数は `/S /WAIT=60`（「再起動して更新」のときは `/RELAUNCH` を足す）。`/D=` は付けない（前回のインストール先を引き継ぐ）。`/S` はサイレント（管理者権限は不要）、`/WAIT=秒` はサイレント実行のとき、起動中の `hitomado.exe` が終了するまで最大その秒数（0.5 秒ごとに再確認。上限 300）待ってから上書きする（省略時は従来どおり待たず、終了コード 5 で中止する。アプリの終了処理の終わりと、インストーラーの上書きの開始のあいだの競合を避ける）、`/RELAUNCH` はインストールの成功後に（`.onInstSuccess`）インストール先の `hitomado.exe` を起動する。**インストール後の起動し直しは NSIS 側で行う**（既存の `project.nsi` の最小限の拡張。別の起動用ヘルパーは作らない。インストーラーはアプリが終了してから動くため、起動し直しがアプリより長生きでき、失敗時にアプリが残っていない場合も扱える）。終了時の入れ替え（`/RELAUNCH` なし）は、更新したあと次に利用者が自分で起動するまで起動しない。プロセスの分離起動はビルドタグで分ける（`launch_windows.go`／`launch_other.go`。Windows 以外では `ErrUnsupported`）
- **インストーラーの失敗**: 入れ替えを指示するとき、起動した版を保存先の `last-attempt` に記録する。次の起動で記録の版が今の版より新しければ（版が上がらなかった = インストーラーが完了しなかった）、ログに警告を残し、画面に「前回の更新の入れ替えが完了しませんでした」と出して、終了時の自動の入れ替えは止める（「再起動して更新」を押したときだけ再試行する）。版が上がっていれば記録を消す。起動に失敗（実行ファイルが無い等）したときは記録を残さず、ログに残して、今の版のまま終了する
- **対象外**: 版が `0.0.0`（dev・ローカルビルド）は確認も自動アップデートもしない（`Manager` を作らない）。Android 版（NF-D05）と、入れ替えを行えないビルド（Windows 以外）も `Manager` を作らず、設定画面にも出さない（`AppInfo.autoUpdateSupported` が偽）。

### 発行イベント

| イベント名 | ペイロード | 契機 |
|-----------|------------|------|
| `sync:updated` | `{ accountId, changedThreadIds[] }` | 差分同期でキャッシュが更新された |
| `account:status` | `{ accountId, status, message? }` | `active`／`syncing`／`needs_reauth`／`error` が変化した |
| `notify:open-thread` | `{ accountId, threadId }` | 通知クリック |
| `update:available` | `{ version, url }` | 新バージョン検出 |
| `update:status` | `{ stage, version?, message? }` | 自動アップデートの進みの変化（`stage`: `none`／`downloading`／`ready`〔入れ替え待ち〕／`failed`。`message` は失敗・注意の文面） |
| `app:before-quit` | なし | トレイの「終了」（フロントが下書きを保存して `AckBeforeQuit`） |
| `files-dropped` | `{ files }` | ファイルドロップ（添付） |

## 6. データの置き場所

| データ | 保存先 | 備考 |
|--------|--------|------|
| リフレッシュトークン・Client Secret・AI API キー・ChatGPT ログインのトークン | Windows 資格情報マネージャー（Android は EncryptedSharedPreferences。鍵は Android Keystore） | `hitomado/<種別>/<ID>` 形式のキー名。Android で Keystore の鍵が失われたときは、保存済みの値が空になり、`secrets.ErrKeyLost`（`ErrNotFound` も満たす）で「要再接続」へ案内する |
| アカウント・OAuth クライアント（Secret を除く）・署名・設定・同期状態・スレッド／メッセージのメタデータ・スニペット・連絡先候補 | SQLite（`%APPDATA%\Hitomado\hitomado.db`、WAL） | 本文・添付は保存しない（NF-S08） |
| メッセージ本文・添付データ | Go プロセスのメモリ（LRU・直近 50 スレッド） | NF-S09 |
| 下書き | **Gmail の下書き**（`users.drafts`） | ローカルに保存しない。例外: 送信元未選択の新規作成はメモリのみ |
| ログ | `%APPDATA%\Hitomado\logs\`（JSON Lines。5 MB×5 世代・30 日） | NF-S10 |
| MCP トークン | Windows 資格情報マネージャー（`hitomado/mcp_token`。Android は上と同じ暗号化ストア） | NF-S13 |

## 7. 通信フロー

### 7.1 アカウント追加（BYO OAuth）

```mermaid
sequenceDiagram
  participant U as 利用者
  participant FE as フロント
  participant AS as AccountService
  participant OA as oauth
  participant BR as システムブラウザ
  participant GO as Google
  participant KS as 資格情報ストア
  participant DB as SQLite

  U->>FE: アカウント追加（OAuth クライアント選択）
  FE->>AS: StartAddAccount(oauthClientId)
  AS->>OA: ループバック待受を開始（127.0.0.1:ランダム）＋PKCE/state 生成
  OA->>BR: 認可 URL を開く
  BR->>GO: 同意
  GO-->>OA: リダイレクト（code, state）
  OA->>GO: トークン交換（code_verifier）
  GO-->>OA: access / refresh token
  OA->>GO: userinfo（email, name）
  AS->>KS: refresh token を保存
  AS->>DB: accounts を作成（表示名・色）
  AS-->>FE: Account
  AS->>AS: 初回同期を開始
```

Android も同じ流れ（ループバック＋PKCE）で、ブラウザは `Android.OpenURL`（Intent）で開く。ブラウザに出ている間にプロセスが止められると待受が消えて認可は完了できないため、`oauth.Tracker` が認可の開始から終了までの印（時刻のみ）を資格情報ストアに残し、残っていれば次の起動で `AccountService.TakeInterruptedAuth` が一度だけ検出して、画面が「やり直し」を案内する（[Android ビルド §14](../environment/android.md#14-google-oauth-のリダイレクト方式86)）。Windows は `Tracker` を持たない。

### 7.2 差分同期と新着通知

```mermaid
sequenceDiagram
  participant SC as sync.Scheduler（アカウントごと）
  participant GM as gmail.Client
  participant DB as store
  participant NT as notify
  participant FE as フロント

  loop 60秒ごと（手動更新でも起動）
    SC->>GM: history.list(startHistoryId)
    alt 成功
      GM-->>SC: 変更（追加・ラベル変更・削除）
      SC->>GM: 変更スレッドの threads.get(format=metadata)
      SC->>DB: スレッド・メッセージを upsert、history_id 更新
      SC->>NT: 未読の INBOX 新着があれば通知（初回・リセット同期は除く）
      SC->>FE: sync:updated
    else historyId 失効(404)
      SC->>GM: INBOX 最新 200 スレッドを再取得
      SC->>DB: 置換（通知なし）
    else invalid_grant
      SC->>DB: status = needs_reauth
      SC->>FE: account:status
    else invalid_client／unauthorized_client（OAuth クライアントの資格情報の拒否）
      SC->>DB: status = needs_reauth（理由: OAuth クライアントの設定を確認）
      SC->>FE: account:status（message つき。自動再試行は止める）
    end
  end
```

#### Android のバックグラウンド同期（#88）

Android は前面にいないアプリを止めるため、アプリを閉じている間は WorkManager の定期実行（15 分間隔・ネットワーク接続が条件）が `SyncWorker` を起こし、Go の `internal/headless`（Wails に依存せず、store・gmail・sync だけを組む）で「1 回だけ同期して新着を返す」。通知は Java の `NotificationManager`（アカウント別チャネル・グループ化）が出し、タップで `MainActivity` を開いて `notify:open-thread` でスレッドを開く。アプリ本体と同じプロセスで Worker が動くときは、Go の `Gate` が二重の同期を避ける。Pub/Sub・FCM・前面サービスは使わない。詳細は [Android ビルド §15](../environment/android.md#15-バックグラウンド同期と新着通知88)。

```mermaid
sequenceDiagram
  participant WM as WorkManager（15 分）
  participant SW as SyncWorker（Java）
  participant HL as headless.SyncOnce（Go）
  participant GM as gmail.Client
  participant DB as store
  participant NM as NotificationManager

  WM->>SW: 起動（プロセスが無ければ OS が起こす）
  SW->>HL: JNI（libwails.so。資格情報は SecureStore を呼び戻して読む）
  alt アプリ本体が同じプロセスで動いている
    HL->>HL: アプリ本体のスケジューラに「今すぐ 1 回更新」を頼む（通知はアプリ本体の経路）
  else
    HL->>GM: アカウントごとに差分同期（60 秒ループと同じ cycle）
    GM->>DB: 反映（新着の判定は DB の ApplyThread）
    HL-->>SW: 新着・アカウントごとの結果（JSON）
    SW->>NM: スレッドごとの通知＋アカウントのサマリー、要再接続は 1 回だけ
  end
```

### 7.3 返信送信（送信元強制・確認トークン）

```mermaid
sequenceDiagram
  participant U as 利用者
  participant FE as フロント
  participant CS as ComposeService
  participant DM as domain（SenderPolicy・reply）
  participant GM as gmail.Client
  participant DB as store

  U->>FE: 返信を書いて送信
  FE->>CS: PrepareSend(sessionId, content, {archiveAfterSend})
  CS->>DB: スレッドの所有アカウントを取得
  CS->>DM: 送信元ポリシー検証（session.accountId == スレッドのアカウント）
  alt 不一致
    DM-->>CS: ErrSenderMismatch
    CS-->>FE: SENDER_MISMATCH（確認ダイアログも出さない）
  else 一致
    CS-->>FE: SendPreview（確認トークン・警告）
    FE->>U: 送信確認ダイアログ（送信元・宛先・本文を表示）
    U->>FE: 「<送信元> から送信」を確定
    FE->>CS: Send(sessionId, confirmToken, content)
    CS->>CS: トークン・内容ハッシュを検証
    CS->>DM: 宛先・件名・ヘッダ・引用・署名を確定
    CS->>GM: messages.send(raw MIME, threadId)
    GM-->>CS: messageId
    opt 送信してアーカイブ
      CS->>GM: threads.modify(INBOX 除去)
    end
    CS->>GM: drafts.delete（下書きがあれば）
    CS->>DB: 連絡先候補を加算、スレッドを更新
    CS-->>FE: SendResult
  end
```

契約の詳細（トークン仕様・エラーコード）は [バインディング仕様 §5.2](../api/endpoints.md#52-送信前確認と送信確認トークン) を参照。

`messages.send` がリクエストを送った後に応答を受け取れず失敗した場合は、`gmail.Client` が Gmail の送信済みを確認し（独自ヘッダ `X-Hitomado-Send-Id` と `in:sent` 検索）、見つかれば成功として扱う。見つからない・確認できないときは `SEND_UNCERTAIN` を返し、再送を促さずアーカイブ・下書き削除もしない（二重送信を避けるため）。接続前の失敗と送信前の期限切れは `OFFLINE`（再送可）、送信前のキャンセルは `CANCELLED`。詳細は同 §5.2 を参照。

### 7.4 AI 下書き

```mermaid
sequenceDiagram
  participant FE as フロント
  participant AI as AIService
  participant CS as ComposeService
  participant GM as gmail.Client
  participant PV as ai.Provider
  FE->>AI: GenerateReplyDraft(sessionId, requestId, instruction?)
  AI->>CS: 作成セッションから送信元アカウントとスレッドを取得（返信・全員に返信のみ。new／forward は VALIDATION）
  AI->>AI: 同意済み・キー設定済み（ChatGPT プランはログイン済み。期限が近ければトークンを更新）を確認
  AI->>GM: threads.get(format=full)（メモリ LRU を優先）
  AI->>PV: 生成要求（直近 5 メッセージ・20,000 文字以内＋あれば利用者の指示。指示は別枠で 2,000 文字まで）
  Note over AI,PV: 設定にエフォートがあれば要求に載せる（Anthropic: output_config.effort／OpenAI 互換: reasoning_effort／ChatGPT: reasoning.effort）。無ければ載せない
  PV-->>AI: 本文テキスト
  AI-->>FE: 下書き本文（フロントがエディタに挿入。送信は行わない）
```

ChatGPT プランでは、ログイン（`SignInChatGPT`）で得たトークンを `internal/siwc` が資格情報ストアに保持し、`PV` は Responses API（`store: false`・`stream: true`）を呼ぶ。生成中に 401 を受けたときは 1 度だけトークンを更新してやり直し、更新できなければ再ログインを案内する。複数の ChatGPT アカウントはアカウントごと（`client_id` ごと）にトークンと登録を分けて保存し、使用中のアカウントのトークンで生成する。ログイン・更新・ログアウトの手順は [バインディング仕様 §6.2](../api/endpoints.md#62-chatgpt-プランsign-in-with-chatgpt)。

生成中の中止はフロントが `CancelGenerate(requestId)` を呼び、`GenerateReplyDraft` は `CANCELLED` で reject される。契約は [バインディング仕様 §6](../api/endpoints.md#6-aiservice)。

### 7.5 スレッドの AI 要約

```mermaid
sequenceDiagram
  participant FE as フロント
  participant AI as AIService
  participant GM as gmail.Client
  participant PV as ai.Provider
  FE->>AI: SummarizeThread(ref, requestId)
  AI->>AI: 同意済み・キー設定済み（ChatGPT プランはログイン済み。期限が近ければトークンを更新）を確認
  AI->>GM: threads.get(format=full)（メモリ LRU を優先）
  AI->>AI: 下書きを除き、全メッセージを古い順にテキスト化（合計 40,000 文字を目安。超えたら古いものから外す）
  AI->>PV: 要約の要求（本文は信頼しない入力の枠。エフォートは設定にあれば載せる）
  PV-->>AI: 要約テキスト
  AI-->>FE: ThreadAIResult（text・includedCount・totalCount）
  Note over FE: フロントがメモリにだけ保持して AI パネルに表示する。Go・DB・ログには残さない
```

スレッドは `ref` だけで指定する（作成セッションを使わない）ので、返信を開いていなくても呼べる。AI の設定・同意・トークンの更新・3xx を追従しない扱い・中止（`CancelGenerate`）は §7.4 と同じ。契約は [バインディング仕様 §6](../api/endpoints.md#6-aiservice)。

## 8. 主要な設計判断

| 判断 | 採用 | 理由・トレードオフ |
|------|------|--------------------|
| 本文キャッシュ | しない（メモリ LRU のみ） | ディスクに本文を残さずプライバシーを優先。開くたびに通信し、オフラインでは本文を読めない |
| 受信箱の「続きを読む」 | アカウントごとに `threads.list` の `pageToken` を保持して追加取得 | ローカルキャッシュ外の古いスレッドも辿れる。`nextPageToken` の永続化が必要（ER 参照） |
| 検索 | 結果を DB に保存せず DTO のみで返す（カーソルはアカウントごとの `pageToken` を埋め込んだ不透明文字列で、サーバー側に状態を持たない） | ローカル全文検索を持たない方針（F-09）。結果への操作は Gmail に反映し、ローカルに行があれば更新する |
| 下書き | Gmail 下書きのみ（ローカル保存なし） | 二重管理を避ける。編集中の異常終了は直近の自動保存分までとなる |
| 送信前確認 | `PrepareSend` が確認トークンを発行し、`Send` がトークンと内容ハッシュを検証する（Go 側でも強制） | 確認ダイアログを経ない送信、確認後に宛先・本文を変えた送信をバックエンドで拒否できる。API は 2 段階になる。**送信元の整合性（`SENDER_MISMATCH`）も同時に検証**し、`From` は `accounts.email` のみから生成する |
| 添付の受け渡し | ファイルパスをバインディングで渡し、Go が読む | 大きなデータをブリッジで往復しない。ドラッグ＆ドロップは Wails のファイルドロップイベントからパスを得る |
| インライン画像（`cid:`） | Go が取得して `data:` URL に埋め込んで表示用 HTML を返す。画像 1 枚 5 MiB 超は取得せず、1 メッセージの `data:` URL 合計 10 MiB（`appcore.MaxInlineDataBytes`）を超える参照は `cid:` のまま残し、表示されない（通知なし。[F-03](../requirements/functional.md)） | 既定の表示は `img-src data:` のみで足りる。リモート画像は既定でブロックし、「画像を表示」を押したメッセージだけ WebView が HTTPS で直接取得する（**外部通信の例外**。`srcdoc` の iframe は親の CSP を継承するため、ホストの CSP に `img-src https:` が必要。取得先はメール本文が指定したホストで、本アプリ側のサーバーへの通信ではない） |
| OAuth クライアント | 複数登録・アカウントごとに選択 | Workspace の「内部」アプリは他社アカウントで使えないため（F-01） |
| 作成セッション | `ComposeService` が返信・転送・新規のセッション（送信元・スレッド・元メッセージ・添付・下書き ID）をメモリに保持する。フロントは本文などの入力内容のみ都度渡す | 送信元の整合・転送の元添付引き継ぎ・`In-Reply-To`／`References` を Go 側で完結できる。セッションの管理と破棄（最終操作から 24 時間）が必要。契約は [バインディング仕様 §5・§9](../api/endpoints.md#5-composeservice) |

## 9. ビルド・CI

- ローカル: `wails3 dev`（開発）、`wails3 package VERSION=X.Y.Z`（NSIS インストーラー `bin/hitomado-vX.Y.Z-windows-amd64-setup.exe` を作る。`task package` も同じ）。`sqlc generate` と `wails3 generate bindings` の生成物はコミットする（差分が出ていないことを CI で検査する）
- GitHub Actions: lint（ubuntu）、test（ubuntu／windows のマトリクス。Go のテスト・vet、フロントは `bun test`）、build（`windows-latest` で `wails3 package` によりインストーラーを作り、インストール・上書き・アンインストールを検証する）、installer-arm64（`windows-11-arm` で同じ x64 のインストーラーと exe を検証する）。`main` への push（PR のマージ）ごとに CI を通したうえで patch を 1 つ上げたタグを自動で作り、インストーラーを Releases に添付する。詳細は [環境構築 §4](../environment/setup.md)
- マイグレーションは `internal/store/migrations/*.sql`（goose）を `embed` し、起動時に適用する。失敗時は起動中止（NF 可用性）

## 改訂履歴

| 版 | 日付 | 変更内容 | 変更理由 |
|----|------|---------|---------|
| 1.0 | 2026-11-02 | 新規作成 | 初版 |
| 1.1 | 2026-11-02 | §2 サニタイズ欄を NF-S04 の最終形に合わせた。§7.3 を `PrepareSend`→確認トークン→`Send` に書き換え。§8 の送信前確認を Go 側強制に変更、検索カーソルの記述を修正、作成セッションを追記 | バインディング仕様の確認トークン方式・作成セッション方式との整合 |
| 1.2 | 2026-11-02 | §2 のフロントのテストを Vitest から `bun test` に変更。§9 の CI 記述を ubuntu／windows 構成に更新 | 環境構築（bun 統一・CI マトリクス）との整合 |
| 1.3 | 2026-11-02 | 発行イベントに `app:before-quit`／`files-dropped` を追加 | バインディング仕様 1.1 との整合 |
| 1.4 | 2026-11-03 | §2 の Wails を v3.1.0 に、§9 のビルド・CI を `wails3 package`（インストーラー）に更新。§4 のレイヤー表・ディレクトリ構成に `run.go`（composition root）、`internal/platform`、`events`／`clock`／`logging` を追加し、`notify` の説明を実装に合わせた | #34、#35 |
| 1.5 | 2026-11-03 | §5 のレート制限に `messages.send`／`drafts.create` が 5xx を再試行しない例外を追記（履歴に未記録だった分） | 二重送信・重複作成を避ける実装との整合（#46。記録漏れの補正 #53） |
| 1.6 | 2026-11-03 | §2 のテスト行を `frontend/bunfig.toml` の preload による検証済みの記述に、補足を「バージョンは `go.mod` で固定している」に改めた | 環境構築 §5.3 の検証結果・`go.mod`（v3.1.0）との整合（#56） |
| 1.7 | 2026-11-03 | §4 のディレクトリ構成の `domain/` の説明を実際のパッケージ名に直した（`replyto` → `reply`、`confirm`・`contacts`・`palette`・`textconv`・`model` を追加） | 実装（`internal/domain/*`）との整合（#59） |
| 1.8 | 2026-11-03 | §7.3 に、応答を受け取れなかった送信の扱い（送信済みの確認→成功扱い／`SEND_UNCERTAIN`）への参照を追記 | バインディング仕様 §5.2 との整合（#55） |
| 1.9 | 2026-11-03 | §7.3 の `SEND_UNCERTAIN` の要約に、送信前の期限切れは `OFFLINE`・送信前のキャンセルは `CANCELLED` を追記 | バインディング仕様 §5.2 の振り分けとの整合（#66） |
| 1.10 | 2026-11-03 | §7.4 の AI 下書きのシーケンスを `GenerateReplyDraft(sessionId, requestId)` に直し、作成セッションから送信元とスレッドを引くこと・`CancelGenerate(requestId)` での中止を追記 | バインディング仕様 §6・実装（`internal/app/aicore`）との整合（#67） |
| 1.11 | 2026-11-03 | §7.2 の同期シーケンスに、`invalid_client`／`unauthorized_client` による `needs_reauth`（理由つき・自動再試行を止める）を追記 | 実装（`internal/sync/cycle.go`）・バインディング仕様 §10 との整合（#43、#69） |
| 1.12 | 2026-11-03 | 技術選定とデータ保存先に MCP・ログの形式・MCP トークンを追加 | #14 |
| 1.13 | 2026-11-03 | §8 のインライン画像に、1 枚 5 MiB・1 メッセージ合計 10 MiB の上限と超過時の動作を追記 | 実装（`internal/gmail/message.go`、`internal/app/appcore/html.go`）との整合（#6） |
| 1.14 | 2026-11-03 | §5 の AI に ChatGPT プラン（`internal/siwc`・Responses API）とモデル一覧を、§6 の保存先に ChatGPT ログインのトークンを、§7.4 に ChatGPT プランでの認証・更新を追記 | Sign in with ChatGPT（#75）、モデル一覧（#76） |
| 1.15 | 2026-11-03 | §5 に、OS ごとの機能差を `internal/capability`（`Capabilities`・`PlatformService`）で表し、Android では MCP・トレイ・多重起動の防止・ウィンドウ状態・自動更新を動かさない旨を追記 | Android 版で使わない機能の無効化（#89） |
| 1.16 | 2026-11-03 | §7.1 に、Android の認可（`Tracker` による、プロセスが止められた認可の検出）を追記 | #86 |
| 1.17 | 2026-11-03 | §7.2 に Android のバックグラウンド同期（WorkManager・`internal/headless`・Java の通知・二重同期の排他）を追記 | #88 |
| 1.18 | 2026-11-04 | §5 の ChatGPT プランに、アカウントごとにトークンと登録を分けて保存し、使用中のアカウントで生成する旨を追記 | 複数の ChatGPT アカウント（#79） |
| 1.19 | 2026-11-04 | `internal/update` に Windows 版の自動アップデート（ダウンロード・`SHA256SUMS.txt` との照合・入れ替えの指示）を追加し、§5.1 に通信先（`github.com` とリダイレクト先）・安全条件・NSIS のサイレント実行（`/S /WAIT=60 /RELAUNCH`）・インストーラーの失敗の扱いを追記。通信先の図と発行イベント（`update:status`）を更新 | #103 |
| 1.20 | 2026-11-05 | §7.4 の `GenerateReplyDraft` に任意の `instruction` を追加（プロバイダへはスレッド本文とは別枠の「利用者の指示」として渡す） | #109 |
| 1.21 | 2026-11-05 | §7.4 に、`ai.Provider` が設定されたエフォートをプロバイダごとの項目（`output_config.effort`／`reasoning_effort`／`reasoning.effort`）で要求に載せることを追記（設定が無ければ従来の要求） | #111 |
| 1.22 | 2026-11-06 | §9 の GitHub Actions に、ARM64 の Windows 11 実機（`windows-11-arm`）で x64 のインストーラーと exe を検証する `installer-arm64` を追記 | #13 |
| 1.23 | 2026-11-06 | §7.5（スレッドの AI 要約: `SummarizeThread` の通信フロー。スレッドは `ThreadRef` で指定し、全メッセージ・合計 40,000 文字を目安に渡す。保存しない）を追加 | #206 |
