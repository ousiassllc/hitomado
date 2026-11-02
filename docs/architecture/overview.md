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
| デスクトップ基盤 | **Wails v3**（v3.0.0 正式版） | システムトレイ・ネイティブ通知（クリック応答つき）・単一インスタンス制御を標準 API で扱える（F-10）。Go バインディングの TypeScript 自動生成（`wails3 generate bindings`） |
| バックエンド言語 | Go | Gmail API 公式クライアント・単一バイナリ配布 |
| メール API | Gmail API（`google.golang.org/api/gmail/v1`） | `history` による差分同期、`threads`／`drafts`／`messages.send` を使う。IMAP は採用しない（OAuth スコープが粗く、スレッド・ラベル・下書きの対応が弱い） |
| OAuth | `golang.org/x/oauth2`＋Google エンドポイント（PKCE・ループバック） | NF-S03 |
| 資格情報ストア | `github.com/zalando/go-keyring`（Windows 資格情報マネージャー） | NF-S01 |
| ローカル DB | SQLite（`modernc.org/sqlite`、純 Go・CGO なし） | Windows でのビルドが容易（C コンパイラ不要） |
| クエリ生成・マイグレーション | **sqlc**（型安全なクエリ生成）＋ **goose**（SQL マイグレーションを `embed`） | |
| MIME 構築 | `github.com/jhillyerd/enmime`（Builder） | `multipart/alternative`・添付・`In-Reply-To`／`References` |
| レート制限 | `golang.org/x/time/rate` | アカウントごとのクォータ制御（NF 同期） |
| ログ | 標準 `log/slog`＋ファイルローテーション | NF-S10 |
| AI | 自前の薄い `Provider` インターフェース。Anthropic と OpenAI 互換の 2 実装 | NF §12 |
| フロントエンド | **Vue 3＋TypeScript**（Composition API／`<script setup>`）、Vite、Pinia | |
| リッチテキスト | Tiptap（ProseMirror）。太字・リンク・箇条書き | F-11 |
| 仮想スクロール | `@tanstack/vue-virtual` | NF-P04 |
| サニタイズ | DOMPurify＋`sandbox` iframe（`srcdoc`、`allow-same-origin` のみ・スクリプト不可） | NF-S04 |
| テスト | Go: 標準 `testing`／`testify`。フロント: **`bun test`**（DOM は happy-dom、コンポーネントは Vue Test Utils。`.vue` の読み込みは bun のプラグイン設定が必要で、実装時に検証する。[環境構築 §5.3](../environment/setup.md)） | NF §10 |
| CI／配布 | GitHub Actions（Windows ランナーでビルド）、GitHub Releases | NF-D01〜D04 |

> 補足: Wails v3 は正式版が出て間もない。API は公式ドキュメント（v3.wails.io）を一次情報とし、実装前にバージョンを固定（`go.mod`）する。

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
  INF <-->|HTTPS| AI[AI プロバイダ 利用者のキー]
  INF <-->|HTTPS| GH[GitHub Releases API]
```

## 4. レイヤー構造

| レイヤー | 責務 | 依存してよい先 |
|----------|------|----------------|
| **UI**（`frontend/`） | 表示・入力・フォーカス・ショートカット。ビジネス判断をしない | バインディング（生成された TS クライアント）のみ |
| **サービス層**（`internal/app`） | Wails に公開するバインディング。入力検証・DTO 変換・ドメインとインフラの調停 | ドメイン層、インフラ層（インターフェース越し） |
| **ドメイン層**（`internal/domain`） | 返信宛先の決定、送信元ポリシー、署名挿入、引用生成、MIME 組立の入力モデル。**I/O を持たない純粋なロジック** | なし（標準ライブラリのみ） |
| **同期層**（`internal/sync`） | アカウント別スケジューラ、`history` 差分の適用、リセット同期、新着検出 | ドメイン層、インフラ層 |
| **インフラ層**（`internal/gmail`／`oauth`／`store`／`secrets`／`ai`／`notify`／`update`） | 外部との I/O。すべてインターフェースで抽象化し、テストではフェイクを注入する | 標準ライブラリ、外部ライブラリ |

### ディレクトリ構成（目安）

```
hitomado/
├─ main.go                 # Wails アプリ起動・サービス登録・トレイ
├─ internal/
│  ├─ app/                 # バインディング（サービス層）
│  ├─ domain/              # 純粋ロジック（replyto, signature, quote, sender policy）
│  ├─ sync/                # 同期スケジューラ
│  ├─ gmail/               # Gmail クライアント（interface＋実装）
│  ├─ oauth/               # ループバック＋PKCE フロー
│  ├─ store/               # sqlc 生成コード・クエリ・goose マイグレーション
│  ├─ secrets/             # 資格情報ストア（interface＋keyring 実装）
│  ├─ ai/                  # Provider interface、anthropic、openai-compat
│  ├─ notify/              # トースト・トレイ
│  └─ update/              # GitHub Releases 確認
├─ frontend/               # Vue 3 + TS（src/components, src/stores, src/lib）
└─ docs/
```

## 5. 実行モデル

- **プロセス**: 単一プロセス。多重起動は Wails v3 の単一インスタンス機能で禁止し、2 つ目の起動は既存ウィンドウを前面に出す
- **ウィンドウ**: メインウィンドウ 1 つ。エディタはメインウィンドウ内のペイン（返信・転送はスレッド下部、新規作成は右ペイン全面）として表示する。独立ウィンドウは v0 では設けない
- **トレイ**: ウィンドウの × は非表示化（トレイへ格納）。トレイメニュー「開く」「終了」。「終了」でのみ同期スケジューラを停止してプロセスを終了する
- **同期**: アカウントごとに 1 ゴルーチン。`time.Ticker`（60 秒）＋手動更新シグナル。`context` でキャンセル可能。アカウント追加・削除に応じて起動・停止する
- **レート制限**: アカウントごとの `rate.Limiter`（クォータユニット単位）。429／5xx は指数バックオフ
- **フロントへの通知**: Go から Wails のイベントを発行し、フロントはイベント受信時にバインディングを再呼び出しして再描画する（イベントに本文等の機密は載せない）

### 発行イベント

| イベント名 | ペイロード | 契機 |
|-----------|------------|------|
| `sync:updated` | `{ accountId, changedThreadIds[] }` | 差分同期でキャッシュが更新された |
| `account:status` | `{ accountId, status, message? }` | `active`／`syncing`／`needs_reauth`／`error` が変化した |
| `notify:open-thread` | `{ accountId, threadId }` | 通知クリック |
| `update:available` | `{ version, url }` | 新バージョン検出 |

## 6. データの置き場所

| データ | 保存先 | 備考 |
|--------|--------|------|
| リフレッシュトークン・Client Secret・AI API キー | Windows 資格情報マネージャー | `hitomado/<種別>/<ID>` 形式のキー名 |
| アカウント・OAuth クライアント（Secret を除く）・署名・設定・同期状態・スレッド／メッセージのメタデータ・スニペット・連絡先候補 | SQLite（`%APPDATA%\Hitomado\hitomado.db`、WAL） | 本文・添付は保存しない（NF-S08） |
| メッセージ本文・添付データ | Go プロセスのメモリ（LRU・直近 50 スレッド） | NF-S09 |
| 下書き | **Gmail の下書き**（`users.drafts`） | ローカルに保存しない。例外: 送信元未選択の新規作成はメモリのみ |
| ログ | `%APPDATA%\Hitomado\logs\` | NF-S10 |

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
    end
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
    FE->>U: 送信確認ダイアログ（送信元を大きく表示）
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

### 7.4 AI 下書き

```mermaid
sequenceDiagram
  participant FE as フロント
  participant AI as AIService
  participant GM as gmail.Client
  participant PV as ai.Provider
  FE->>AI: GenerateReplyDraft(accountId, threadId)
  AI->>AI: 同意済み・キー設定済みを確認
  AI->>GM: threads.get(format=full)（メモリ LRU を優先）
  AI->>PV: 生成要求（直近 5 メッセージ・20,000 文字以内）
  PV-->>AI: 本文テキスト
  AI-->>FE: 下書き本文（フロントがエディタに挿入。送信は行わない）
```

## 8. 主要な設計判断

| 判断 | 採用 | 理由・トレードオフ |
|------|------|--------------------|
| 本文キャッシュ | しない（メモリ LRU のみ） | ディスクに本文を残さずプライバシーを優先。開くたびに通信し、オフラインでは本文を読めない |
| 受信箱の「続きを読む」 | アカウントごとに `threads.list` の `pageToken` を保持して追加取得 | ローカルキャッシュ外の古いスレッドも辿れる。`nextPageToken` の永続化が必要（ER 参照） |
| 検索 | 結果を DB に保存せず DTO のみで返す（カーソルはアカウントごとの `pageToken` を埋め込んだ不透明文字列で、サーバー側に状態を持たない） | ローカル全文検索を持たない方針（F-09）。結果への操作は Gmail に反映し、ローカルに行があれば更新する |
| 下書き | Gmail 下書きのみ（ローカル保存なし） | 二重管理を避ける。編集中の異常終了は直近の自動保存分までとなる |
| 送信前確認 | `PrepareSend` が確認トークンを発行し、`Send` がトークンと内容ハッシュを検証する（Go 側でも強制） | 確認ダイアログを経ない送信、確認後に宛先・本文を変えた送信をバックエンドで拒否できる。API は 2 段階になる。**送信元の整合性（`SENDER_MISMATCH`）も同時に検証**し、`From` は `accounts.email` のみから生成する |
| 添付の受け渡し | ファイルパスをバインディングで渡し、Go が読む | 大きなデータをブリッジで往復しない。ドラッグ＆ドロップは Wails のファイルドロップイベントからパスを得る |
| インライン画像（`cid:`） | Go が取得して `data:` URL に埋め込んで表示用 HTML を返す | CSP を `img-src data:` に限定できる。リモート画像は既定でブロック |
| OAuth クライアント | 複数登録・アカウントごとに選択 | Workspace の「内部」アプリは他社アカウントで使えないため（F-01） |
| 作成セッション | `ComposeService` が返信・転送・新規のセッション（送信元・スレッド・元メッセージ・添付・下書き ID）をメモリに保持する。フロントは本文などの入力内容のみ都度渡す | 送信元の整合・転送の元添付引き継ぎ・`In-Reply-To`／`References` を Go 側で完結できる。セッションの管理と破棄（最終操作から 24 時間）が必要。契約は [バインディング仕様 §5・§9](../api/endpoints.md#5-composeservice) |

## 9. ビルド・CI

- ローカル: `wails3 dev`（開発）、`wails3 build`（Windows 向け成果物）。`sqlc generate` と `wails3 generate bindings` の生成物はコミットする（差分が出ていないことを CI で検査する）
- GitHub Actions: lint（ubuntu）、test（ubuntu／windows のマトリクス。Go のテスト・vet、フロントは `bun test`）、build（`windows-latest` で `wails3 build`）。タグ push 時に成果物を Releases に添付。詳細は [環境構築 §4](../environment/setup.md)
- マイグレーションは `internal/store/migrations/*.sql`（goose）を `embed` し、起動時に適用する。失敗時は起動中止（NF 可用性）

## 改訂履歴

| 版 | 日付 | 変更内容 | 変更理由 |
|----|------|---------|---------|
| 1.0 | 2026-11-02 | 新規作成 | 初版 |
| 1.1 | 2026-11-02 | §2 サニタイズ欄を NF-S04 の最終形に合わせた。§7.3 を `PrepareSend`→確認トークン→`Send` に書き換え。§8 の送信前確認を Go 側強制に変更、検索カーソルの記述を修正、作成セッションを追記 | バインディング仕様の確認トークン方式・作成セッション方式との整合 |
| 1.2 | 2026-11-02 | §2 のフロントのテストを Vitest から `bun test` に変更。§9 の CI 記述を ubuntu／windows 構成に更新 | 環境構築（bun 統一・CI マトリクス）との整合 |
