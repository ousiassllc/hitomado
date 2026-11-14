# バインディング仕様（API 仕様）

> 関連: [プロジェクト概要](../overview.md) / [機能要件](../requirements/functional.md) / [アーキテクチャ設計](../architecture/overview.md) / [ER](../architecture/er.md) / [コンポーネント設計](../components/overview.md)

Hitomado は HTTP サーバーを持たない。フロントエンド（Vue）と Go バックエンドの境界は **Wails v3 のバインディング**（Go のサービスメソッドを TypeScript から呼ぶ）であり、本書がその契約を定める。外部 API（Gmail API・OAuth・AI・GitHub）の利用方法も §10 にまとめる。

## 1. 規約

### 1.1 サービスと生成物

- 各サービスは `internal/app` の Go 構造体で、Wails の `application.NewService` に登録する。メソッドは `context.Context` を第 1 引数に取ってよい（Wails が注入する）
- TypeScript クライアントは `wails3 generate bindings` で生成し、コミットする（CI で差分なしを検査）。フロントは生成されたクライアントのみを経由して呼ぶ。`fetch`／`XMLHttpRequest` による外部通信は CSP で禁止する
- 型名は Go と TS で同名。JSON フィールドは `camelCase`。日時は **Unix ミリ秒（number）**。ID は文字列

| サービス | 責務 |
|----------|------|
| `AccountService` | OAuth クライアント・アカウントの管理、署名、通知設定 |
| `InboxService` | 統合受信箱、検索、未読件数、スレッド操作、ラベル |
| `ThreadService` | スレッド詳細、添付保存、リンクを開く |
| `ComposeService` | 作成セッション、下書き、送信前確認、送信、宛先候補 |
| `AIService` | AI 設定・同意・下書き生成・スレッドの要約・スレッドへの質問・Inbox Zero（アーカイブ候補の判定） |
| `AppService` | アプリ設定（受信箱フィルタ等）、バージョン、更新確認 |
| `LogService` | フロントのエラーの記録、ログフォルダを開く、ログの書き出し |
| `MCPService` | 内蔵 MCP サーバーの設定・トークン・状態 |
| `PlatformService`（`internal/capability`） | 実行中の OS で提供する機能 |
| `NotifyService`（`internal/app/notifycore`） | 通知の許可状態・OS の通知設定・通知のタップで開くスレッド |

### 1.2 エラー

エラーは Go の `error` を返し、フロントでは reject される。Wails の `MarshalError` オプションに `apperr.Marshal` を指定しており（`run.go`）、`AppError` の JSON を返す。フロントでは reject 値の `error.cause` にその JSON 文字列が入る。フロントは共通ラッパー `parseBindingError`（`frontend/src/lib/api.ts`）で `cause` → `message` → 文字列そのものの順に JSON として解釈し、`AppError` に変換する。どれも `AppError` として読めないときは `INTERNAL`（`retryable: false`）にフォールバックする。なお `(*Error).Error()` の文字列は `"<code>: <message>"` で JSON ではない。

```ts
type AppError = {
  code: ErrorCode;        // 下表
  message: string;        // 利用者に表示してよい日本語。機密（トークン・本文）を含めない
  retryable: boolean;     // 再試行で成功し得るか
  accountId?: string;     // 特定アカウントに起因する場合
  details?: Record<string, unknown>; // コード固有の補足（例: 添付の上限バイト数）
};
```

| コード | 意味 | `retryable` | 主な発生元 |
|--------|------|-------------|------------|
| `VALIDATION` | 引数不正 | false | 全般 |
| `NOT_FOUND` | アカウント・スレッド・メッセージ・セッション等が存在しない | false | 全般 |
| `SESSION_NOT_FOUND` | 作成セッションが存在しない・破棄済み | false | ComposeService, AIService |
| `OFFLINE` | ネットワーク不通 | true | Gmail／AI／GitHub 呼び出し |
| `RATE_LIMITED` | 429（バックオフで解消せず） | true | Gmail／AI |
| `OAUTH_CLIENT_REJECTED` | OAuth クライアントの資格情報が Google に拒否された（`invalid_client`／`unauthorized_client`／トークン更新・認可コード交換の 401）。Client ID／Secret の設定を確認する | false | Gmail 呼び出し, StartAddAccount, Reconnect |
| `GMAIL_ERROR` | Gmail API のその他のエラー。`message` に HTTP ステータスと Google が返した reason（`forbidden` など。文字種を制限したもの）を添える。Google Cloud のプロジェクトで Gmail API が無効（reason が `SERVICE_DISABLED`／`accessNotConfigured`）のときは、有効にする案内の固定文言にし、`details.reason = "gmail_api_disabled"` を持つ | 状況による | Gmail 呼び出し |
| `NEEDS_REAUTH` | トークン失効（`invalid_grant`）、認証切れ（401。アクセストークンを取り直して 1 回再試行しても 401）、権限（スコープ）不足（403 で reason が `ACCESS_TOKEN_SCOPE_INSUFFICIENT`／`insufficientPermissions`）。当該アカウントを「要再接続」にする。権限不足のときは、再接続して権限を許可し直す案内の固定文言にし、`details.reason = "scope_insufficient"` を持つ | false | Gmail 呼び出し |
| `ACCOUNT_DUPLICATE` | 同じアドレスのアカウントが登録済み | false | StartAddAccount |
| `OAUTH_CLIENT_IN_USE` | OAuth クライアントを使うアカウントが存在する | false | DeleteOAuthClient |
| `OAUTH_CANCELLED` | 利用者が認可をキャンセルした | false | StartAddAccount, SignInChatGPT |
| `OAUTH_DENIED` | Google／OpenAI 側で拒否された（アクセス拒否・スコープ不足。ChatGPT は ChatGPT プランの利用が許可されなかった場合も） | false | StartAddAccount, SignInChatGPT |
| `OAUTH_TIMEOUT` | 認可が 5 分以内に完了しなかった | true | StartAddAccount, SignInChatGPT |
| `SENDER_REQUIRED` | 送信元アカウントが未選択 | false | ComposeService |
| `SENDER_LOCKED` | 返信・転送で送信元を変更しようとした | false | SetComposeSender |
| `SENDER_MISMATCH` | 送信元と受信アカウントの不整合を検知（防御的検査） | false | PrepareSend, Send |
| `NO_RECIPIENTS` | 宛先（To／Cc／Bcc）が 1 件もない | false | PrepareSend |
| `INVALID_ADDRESS` | メールアドレスの形式不正（`details.emails`） | false | PrepareSend |
| `ATTACHMENT_TOO_LARGE` | 添付合計が 25MB を超える（`details.limitBytes`） | false | AddAttachments |
| `ATTACHMENT_IO` | 添付ファイルの読み込み・保存に失敗（`details.filename`。メッセージにはファイル名を含めない） | false | AddAttachments, SaveAttachment, Send |
| `CONFIRM_REQUIRED` | 確認トークンがない・未知 | false | Send |
| `CONFIRM_EXPIRED` | 確認トークンの期限切れ（5 分）または使用済み | false | Send |
| `CONFIRM_MISMATCH` | 確認時と送信時で送信内容が異なる | false | Send |
| `SEND_UNCERTAIN` | リクエストを送った後に `Send` の応答を受け取れず（切断・タイムアウト・期限切れ・呼び出し側のキャンセル）、Gmail 側の送信済みに同じ送信を確認できなかった。送信済みの可能性があるため、送信済みフォルダーを確認してから、必要な場合だけ再送する。接続前の失敗（名前解決・接続拒否・トークン更新の通信失敗）や、リクエストを送る前の期限切れは本文を送れていないので、これではなく `OFFLINE`（再試行可）。送る前のキャンセルは `CANCELLED` | false | Send |
| `AI_NOT_CONFIGURED` | API キー・プロバイダ・モデル未設定。ChatGPT プランでは、未ログイン、または再ログインが必要（更新できない・トークンが失効した） | false | AIService |
| `AI_CONSENT_REQUIRED` | 当該プロバイダへの送信に未同意 | false | AIService |
| `AI_FAILED` | AI プロバイダのエラー（キー不正・タイムアウト・リダイレクト応答（3xx。追従しない）等）。ChatGPT プランでは、利用枠の超過（`subscription_sharing_usage_limit_exceeded`。`details.reason = "usage_limit_exceeded"` を持ち、画面は「Manage usage」を主なアクションにした案内を出す）・このアカウントでは使えない（`subscription_sharing_user_not_eligible`）・ログインの検証失敗も含む。エフォートを載せた要求をプロバイダが拒否したとき（`ai.ErrRejected`）は `details.reason = "effort_rejected"` を持つ（`message` が設定でエフォートを「既定」に戻す案内になる。画面は `message` をそのまま出す）。`details.reason` を持つ `AI_FAILED` は `usage_limit_exceeded` と `effort_rejected` の 2 つだけで、ほかは持たない | 状況による | AIService |
| `CANCELLED` | 呼び出し側がキャンセルした | false | AIService |
| `UNSUPPORTED` | この OS では提供しない機能の呼び出し（Android での自動アップデートの設定。更新確認を持たない機能構成での更新確認の設定） | false | AppService |
| `INTERNAL` | 想定外の内部エラー | false | 全般 |

### 1.3 共通型

```ts
type ThreadRef = { accountId: string; threadId: string };
type MessageRef = ThreadRef & { messageId: string };
type Address = { name?: string; email: string };

type Account = {
  id: string;
  oauthClientId: string;
  email: string;            // 返信・転送の送信元（変更不可）
  displayName: string;
  color: string;            // パレットキー
  signatureHtml: string;
  notifyEnabled: boolean;
  status: 'active' | 'needs_reauth' | 'error';
  statusMessage?: string;
  sortOrder: number;
};

type OAuthClient = { id: string; name: string; clientId: string; accountCount: number; hasSecret: boolean };

type Label = { id: string; name: string; colorBg?: string; colorFg?: string };

type ThreadSummary = ThreadRef & {
  subject: string;
  snippet: string;
  lastMessageAt: number;
  lastFrom: Address;
  messageCount: number;
  isUnread: boolean;
  hasAttachment: boolean;
  inInbox: boolean;
  labels: Label[];          // ユーザー作成ラベルのみ
};

type ThreadPage = {
  items: ThreadSummary[];
  nextCursor?: string;      // 不透明文字列。undefined なら末尾。items が 0 件でも返ることがある（絞り込みで全件落ちても Gmail 側に続きがあるとき）
  failures: { accountId: string; error: AppError }[]; // 一部アカウントの取得失敗（他アカウントの結果は返す）
};
```

## 2. AccountService

| メソッド | 引数 | 戻り値 | 内容 |
|----------|------|--------|------|
| `ListOAuthClients()` | — | `OAuthClient[]` | 登録済みクライアント（Secret は返さない） |
| `AddOAuthClient(name, clientId, clientSecret)` | string ×3 | `OAuthClient` | 登録。Secret は資格情報ストアへ。`client_id` 重複は `VALIDATION` |
| `UpdateOAuthClient(id, name, clientId, clientSecret?)` | | `OAuthClient` | Secret は指定時のみ更新。保存に失敗しても Client ID と Secret の組は変わらない |
| `DeleteOAuthClient(id)` | string | — | 使用中は `OAUTH_CLIENT_IN_USE`。Secret も削除（Secret の削除に失敗したらクライアントは残しエラーを返す） |
| `ListAccounts()` | — | `Account[]` | `sortOrder` 昇順 |
| `StartAddAccount(oauthClientId)` | string | `Account` | 認可フローを開始し、**完了またはキャンセル・タイムアウトまでブロック**する。システムブラウザを開く。完了後に初回同期を開始 |
| `CancelAddAccount()` | — | — | 進行中の認可を中断（`StartAddAccount` は `OAUTH_CANCELLED` で reject） |
| `TakeInterruptedAuth()` | — | `boolean` | 前回の起動が Google の認可の途中で止められていたか（Android がブラウザに出ている間にプロセスを終了させた場合）を**一度だけ** `true` で返す（2 回目以降と、検出しない構成〔Windows〕は `false`）。画面は起動時に呼び、`true` なら「ログインはやり直し」を案内する。[Android ビルド §14.3](../environment/android.md#143-プロセスが止められたとき決定) |
| `Reconnect(accountId)` | string | `Account` | `needs_reauth` のアカウントを再認可。**認可されたアドレスが `accounts.email` と異なる場合は拒否**（`VALIDATION`） |
| `RemoveAccount(accountId)` | string | — | 同期ワーカーを停止（実行中の周期の終了を待つ）→DB カスケード削除→資格情報削除＋トークン revoke（失敗しても削除は成功扱い）。DB 削除に失敗したときは同期を再開してエラーを返す |
| `UpdateAccount(accountId, patch)` | `{ displayName?, color?, notifyEnabled? }` | `Account` | 表示名・色・通知設定 |
| `SetSignature(accountId, signatureHtml)` | string ×2 | `Account` | 署名。許可タグ（`p, br, strong, b, em, i, u, a[href], ul, ol, li`）以外はサーバー側で除去 |
| `FetchGmailSignature(accountId)` | string | `{ signatureHtml: string; found: boolean; sanitized: boolean }` | Gmail の設定（`users.settings.sendAs.get`。`sendAsEmail` は `accounts.email`）から署名を取得して返す。**保存しない**（保存は `SetSignature`）。`signatureHtml` は `SetSignature` と同じ許可タグへ整形したもの（`div` は `p` に、画像・表・`style`・`font` などは除く）。`sanitized` は整形で内容を落としたか（画面が「取り込めない部分は除きました」を出す契機）。Gmail の署名が空なら `found = false`・`signatureHtml = ""`。未登録の `accountId` は `NOT_FOUND`。失敗は §10 の共通の変換（`OFFLINE`・`NEEDS_REAUTH`・`GMAIL_ERROR` など）。署名の本文はログに残さない（NF-S10）。MCP からは呼べない |
| `ReorderAccounts(accountIds)` | `string[]` | `Account[]` | 並び順を更新。全アカウント ID を過不足なく指定 |

- 色の候補（パレットキー）は定数として TS 側と Go 側で共有し、未知のキーは `VALIDATION`
- 認可フロー・保存先は [アーキテクチャ設計 §7.1](../architecture/overview.md#71-アカウント追加byo-oauth)、[ER](../architecture/er.md) を参照

## 3. InboxService

```ts
type InboxQuery = {
  accountIds: string[];     // 空配列なら「すべて」
  unreadOnly: boolean;
  tab?: "all" | "primary" | "social" | "promotions" | "new"; // 受信箱のタブ。省略・"all" は絞り込まない。未知の値は VALIDATION
  limit: number;            // 1〜100、既定 50
  cursor?: string;          // 前回の nextCursor
};

type SearchQuery = {
  q: string;                // Gmail 検索クエリ（そのまま渡す）
  accountIds: string[];     // 空配列なら「すべて」
  limit: number;
  cursor?: string;
};
```

| メソッド | 引数 | 戻り値 | 内容 |
|----------|------|--------|------|
| `ListInbox(query)` | `InboxQuery` | `ThreadPage` | ローカル DB の `INBOX` スレッドを `last_message_at` 降順で返す（キーセットページング）。ローカルの行が尽き、かつ対象アカウントの `inbox_exhausted = 0` なら、`inbox_page_token` で Gmail から追加取得して保存し、続きを返す。絞り込み（未読のみ・`tab` 等）で追加取得分がすべて落ちても Gmail 側に続きがあるときは、`items` が 0 件でも `nextCursor` を返す。**`tab` の絞り込み**（F-02）: `primary` は `CATEGORY_PERSONAL` を持つ、またはどの `CATEGORY_*` も持たない `INBOX` スレッド、`social`／`promotions` は `CATEGORY_SOCIAL`／`CATEGORY_PROMOTIONS` を持つスレッド、`new` は `is_unread = 1`（`unreadOnly` と同じ。両方指定は AND）。ラベルはスレッド内のいずれかのメッセージの `message_labels`。**ローカルのキャッシュ（`threads`／`message_labels`）にだけ掛け、Gmail への追加取得は絞り込まない `INBOX` の続き（アカウントごと 1 本の `inbox_page_token`）を進める**（理由: 追加取得の経路は `threads.list(labelIds=INBOX)` のページ送りだけで、`q`／`labelIds` を変えるとタブごとにトークンと取得済みの範囲を別管理する必要があり、同期状態（`account_sync_state`）が複雑になるため。カテゴリのラベルはメッセージを取得するまで分からず、`INBOX` を取得して初めて判定できる）。カーソルは絞り込みに依存しない位置（`last_message_at`・アカウント・スレッド ID）で、タブごとに独立して辿れる。タブを切り替えたら呼び出し側がカーソルを捨てて先頭から取り直す（別のタブのカーソルを渡しても位置としては有効で、そのタブの絞り込みで続きを返す） |
| `Refresh(accountIds)` | `string[]`（空＝全） | `{ results: { accountId: string; ok: boolean; error?: AppError }[] }` | 差分同期を今すぐ実行し完了を待つ（直近の手動更新から 5 秒未満の呼び出しは直近の結果を返す） |
| `GetUnreadCounts()` | — | `Record<accountId, number>` | `threads` の `in_inbox = 1 AND is_unread = 1` の件数 |
| `Search(query)` | `SearchQuery` | `ThreadPage` | 各アカウントで `threads.list(q)`＋`threads.get(metadata)` を並列実行し、`lastMessageAt` 降順に統合する。結果は DB に保存しない。カーソルはアカウントごとの `pageToken` を含む不透明文字列。クエリが空文字は `VALIDATION` |
| `MarkRead(refs, read)` | `ThreadRef[]`, boolean | `ThreadSummary[]` | `UNREAD` の付与・除去（`threads.modify`）。更新後のサマリを返す |
| `Archive(refs)` | `ThreadRef[]` | `ThreadSummary[]` | `INBOX` を除去 |
| `Trash(refs)` | `ThreadRef[]` | `ThreadSummary[]` | `threads.trash` |
| `Unarchive(refs)` | `ThreadRef[]` | `ThreadSummary[]` | `Archive` の取り消し。`INBOX` を付け直す（`threads.modify`）。UI のスナックバー「元に戻す」が呼ぶ |
| `Untrash(refs, backToInbox)` | `ThreadRef[]`, boolean | `ThreadSummary[]` | `Trash` の取り消し（`threads.untrash`）。`backToInbox` が true なら `INBOX` も付け直す（`Trash` は `INBOX` を除去し、`untrash` が戻すとは限らないため。ゴミ箱へ移す前に受信箱にあったスレッドだけ true）。UI のスナックバー「元に戻す」が呼ぶ |
| `ListLabels(accountId)` | string | `Label[]` | ユーザー作成ラベル（キャッシュ。`labels.list` で更新）【Could】 |
| `ModifyLabels(ref, addLabelIds, removeLabelIds)` | | `ThreadSummary` | ユーザー作成ラベルの付与・除去（`threads.modify`）【Could】。システムラベルの指定は `VALIDATION` |

- 複数 ref を渡す操作は、アカウントごとにまとめて実行する。**1 件でも失敗したら、失敗した ref を `details.failedRefs` に載せた `GMAIL_ERROR`（または `NEEDS_REAUTH`／`OFFLINE`）で reject する**。成功した分は Gmail・DB に反映済みなので、フロントは reject を受けたら `ListInbox` を再取得して画面を揃える
- 操作時に Gmail が 404（スレッドが他端末で削除済み）を返した ref は DB から削除し、`NOT_FOUND`（「スレッドが見つかりません」）として `failedRefs` に含める。reject の `code` は、失敗がすべて `NOT_FOUND` のときだけ `NOT_FOUND`、他の失敗と混在するときは `NOT_FOUND` 以外（`RATE_LIMITED` 等）を優先する。`failedRefs` には失敗した ref だけが入り、成功した ref は含まれない
- 操作後は DB を更新し、`threads` の集計値を再計算する（[ER §5](../architecture/er.md)）

## 4. ThreadService

```ts
type Attachment = { id: string; filename: string; mimeType: string; size: number };

type ThreadMessage = {
  id: string;
  from: Address;
  to: Address[];
  cc: Address[];
  date: number;
  isUnread: boolean;
  bodyHtml?: string;        // 未サニタイズ。cid 画像は data: URL に置換済み。サニタイズ・リモート画像ブロックはフロント
  bodyText?: string;        // HTML がない場合のみ
  hasRemoteImages: boolean; // 本文にリモート画像参照がある
  attachments: Attachment[];
};

type ThreadDetail = {
  account: Account;         // 受信アカウント
  threadId: string;
  subject: string;
  messages: ThreadMessage[]; // 下書き（DRAFT ラベル）を除く。時系列昇順
  draftId?: string;         // このスレッドの Gmail 下書き（あれば）。スレッド内に DRAFT ラベルのメッセージがあるときだけ drafts.list で探す
  labels: Label[];
};
```

| メソッド | 引数 | 戻り値 | 内容 |
|----------|------|--------|------|
| `GetThread(ref)` | `ThreadRef` | `ThreadDetail` | `threads.get(format=full)`。メモリ LRU に保持。**状態は変更しない**（既読化は、スレッドを開いて表示した後にフロントが `MarkRead` を呼ぶ。F-03） |
| `SaveAttachment(ref, attachmentId, suggestedName)` | `MessageRef`, string ×2 | `{ saved: boolean; path?: string }` | ネイティブの保存ダイアログ（Android は Storage Access Framework の保存先選択。[Android ビルド §13](../environment/android.md#13-添付の保存ログの書き出しファイルを開く85)）を開き、選択先に書き込む。キャンセルは `saved=false`。Android では保存先選択のあとに添付を取得し、選ばれた場所へコピーしてから返す。`path` は Windows では保存したパス、Android では選ばれた保存先のファイル名。ファイル名は `suggestedName` を OS の禁止文字で正規化する。保存先と同じディレクトリの一時ファイルへ書いて置き換え、置換後のファイルに Windows の Mark-of-the-Web（`Zone.Identifier` の `ZoneId=3`）を付ける（付けられなくても保存は成功） |
| `OpenExternal(url)` | string | — | システムブラウザ／メーラーで開く。許可スキームは `http`／`https`／`mailto` のみ（それ以外は `VALIDATION`、NF-S07） |

- 本文の取得は毎回 Gmail に問い合わせ、ディスクには保存しない（NF-S08／S09）。メモリ LRU にあれば再利用し、`history` で当該スレッドが変更されたら破棄する
- `bodyHtml` のサニタイズとリモート画像ブロックは [コンポーネント設計](../components/overview.md) の `MessageBody` が担う

## 5. ComposeService

返信・転送・新規を**作成セッション**として Go 側で保持する。セッションはメモリ上のみ。送信元・スレッド・元メッセージ・添付・Gmail 下書き ID を Go が持ち、フロントは本文などの**入力内容（`ComposeContent`）を都度渡す**。

```ts
type ComposeMode = 'reply' | 'reply_all' | 'forward' | 'new';

type OpenComposeRequest = {
  mode: ComposeMode;
  ref?: ThreadRef;           // reply / reply_all / forward で必須
  sourceMessageId?: string;  // 省略時は最新メッセージ（自分の送信を除く）
  accountId?: string;        // new のみ。未指定なら送信元未選択
  draftId?: string;          // 既存の Gmail 下書きから復元する
};

type ComposeContent = {
  to: Address[];
  cc: Address[];
  bcc: Address[];
  subject: string;
  bodyHtml: string;          // 署名ブロックは <div data-hitomado-signature> で囲む
};

type ComposeSession = {
  sessionId: string;
  mode: ComposeMode;
  from?: Address;            // 送信元（未選択なら undefined）
  accountId?: string;
  senderLocked: boolean;     // reply / reply_all / forward は true
  content: ComposeContent;   // 宛先・件名・本文（署名＋引用込み）の初期値
  attachments: ComposeAttachment[]; // 転送の元添付・復元した下書きの添付を含む
  draftId?: string;
  attachmentError?: AppError; // 下書きの添付が 25MB 超で復元できなかったとき（ATTACHMENT_TOO_LARGE）。本文・宛先は復元済みで attachments は空
};

type ComposeAttachment = {
  id: string;
  filename: string;
  size: number;
  source: 'local' | 'forwarded'; // 復元した下書きの添付は 'local'
};

type SendPreview = {
  confirmToken: string;      // 5 分有効・1 回限り
  expiresAt: number;
  from: Address & { accountId: string; color: string };
  to: Address[]; cc: Address[]; bcc: Address[];
  subject: string;
  attachmentCount: number;
  bodyPreview: string;       // 本文（プレーンテキスト。改行を保つ。署名・引用は含めない。約 600 文字）
  warnings: SendWarning[];
};

type SendWarning =
  | { code: 'OTHER_OWN_ACCOUNT'; message: string; emails: string[] } // 宛先に他の自アカウントのアドレスがある
  | { code: 'EMPTY_SUBJECT'; message: string }
  | { code: 'EMPTY_BODY'; message: string };

type SendResult = { messageId: string; threadId: string; archived: boolean; archiveError?: AppError };
```

| メソッド | 引数 | 戻り値 | 内容 |
|----------|------|--------|------|
| `OpenCompose(req)` | `OpenComposeRequest` | `ComposeSession` | セッションを作成し、宛先・件名・本文（署名・引用込み）を計算して返す（§5.1）。`draftId` 指定時は Gmail 下書きの内容（宛先・Cc・Bcc・件名・本文・**添付**）で上書きする。添付は復元時に取得して Go のメモリに保持する（下書きの更新でメッセージ ID が変わるため）。取得に失敗したときは `ATTACHMENT_IO` などで失敗し、合計が 25MB を超えるときは添付のみ復元せず `attachmentError` を返す |
| `SetComposeSender(sessionId, accountId)` | string ×2 | `ComposeSession` | **`mode = new` のみ**。それ以外は `SENDER_LOCKED`。署名の差し替えはフロントが `Account.signatureHtml` で行う（F-07） |
| `AddAttachments(sessionId, paths)` | string, `string[]` | `ComposeAttachment[]` | ローカルファイルを添付に追加（Go がパスを保持。内容は送信時に読む）。Content-Type は拡張子から `exportfile.MIMEForName` の固定表で決め（OS の対応表・レジストリには依存しない）、表に無ければ `application/octet-stream`。合計 25MB 超は `ATTACHMENT_TOO_LARGE`（追加分は全て不採用） |
| `RemoveAttachment(sessionId, attachmentId)` | string ×2 | `ComposeAttachment[]` | 転送の元添付も削除可 |
| `SaveDraft(sessionId, content)` | string, `ComposeContent` | `{ draftId: string; savedAt: number }` | Gmail の下書きを作成・更新（`drafts.create`／`update`）。**添付（ローカル・転送元・復元済み）も下書きの MIME に含める**（取得・読み込みに失敗したら `ATTACHMENT_IO`。添付なしの下書きで上書きしない）。送信元未選択は `SENDER_REQUIRED`（メモリのみで保持するため呼ばない） |
| `PrepareSend(sessionId, content, options)` | string, `ComposeContent`, `{ archiveAfterSend: boolean }` | `SendPreview` | 送信前検証（§5.2）を行い、確認ダイアログ用の内容と**確認トークン**を返す |
| `Send(sessionId, confirmToken, content)` | string ×2, `ComposeContent` | `SendResult` | トークンを検証して送信する（§5.2）。成功時はセッションを閉じ、下書きを削除、連絡先候補を加算する |
| `CloseCompose(sessionId)` | string | — | セッションを破棄（下書きは残す）。未保存の変更の保存はフロントが先に `SaveDraft` する |
| `DiscardCompose(sessionId)` | string | — | セッションを破棄し、Gmail 下書きがあれば削除する |
| `SuggestContacts(accountId, prefix, limit)` | string, string, number | `Address[]` | `contacts` から、メールアドレスまたは表示名の前方一致で候補を返す。順位は [ER §3.8](../architecture/er.md) |

### 5.1 OpenCompose の初期値

| モード | 送信元 | 宛先 | 件名 | 本文 |
|--------|--------|------|------|------|
| `reply` | スレッドのアカウント（固定） | 元メッセージの `Reply-To`、なければ `From`。元が自分の送信なら元の `To` | `Re: ` を付与（既にあれば重複させない） | 署名＋引用 |
| `reply_all` | 同上 | 返信宛先＋元の `To`／`Cc` から**受信アカウントのアドレスを除外** | 同上 | 署名＋引用 |
| `forward` | 同上 | 空 | `Fwd: ` を付与 | 署名＋元メッセージの引用（ヘッダ付き）。元の添付を `source = 'forwarded'` として引き継ぐ |
| `new` | `accountId` 指定があればそのアカウント、なければ未選択 | 空 | 空 | 署名（送信元ありの場合） |

- `In-Reply-To`／`References`／`threadId` は Go がセッションに保持し、送信時に設定する（フロントに渡さない）
- `from` は `accounts.email` のみから決まる。`ComposeContent` に `from` フィールドは存在しない（フロントから送信元を指定する経路を作らない）
- 引用・署名の挿入規則は [機能要件 F-05／F-07](../requirements/functional.md) に従う

### 5.2 送信前確認と送信（確認トークン）

```mermaid
sequenceDiagram
  participant FE as フロント
  participant CS as ComposeService
  participant DM as domain
  participant GM as gmail.Client

  FE->>CS: PrepareSend(sessionId, content, {archiveAfterSend})
  CS->>CS: 検証（送信元あり・宛先1件以上・アドレス形式・添付サイズ）
  CS->>DM: SenderPolicy.Check(session.accountId, thread の所有アカウント)
  DM-->>CS: OK or SENDER_MISMATCH
  CS->>CS: 内容ハッシュ H を計算（送信元・宛先・件名・本文・添付一覧・archive）
  CS-->>FE: SendPreview（confirmToken, warnings）
  FE->>FE: 送信確認ダイアログを表示
  Note over FE: キャンセル時はトークンを破棄（再度 PrepareSend が必要）
  FE->>CS: Send(sessionId, confirmToken, content)
  CS->>CS: トークン検証（未知/期限/使用済み/ハッシュ不一致）
  CS->>GM: messages.send（raw MIME, threadId）
  GM-->>CS: messageId
  opt archiveAfterSend
    CS->>GM: threads.modify（INBOX 除去）
  end
  CS-->>FE: SendResult
```

- 設定「送信前に確認画面を出す」（`AppService.SetSendConfirmEnabled`。既定オン）がオフのとき、フロントは確認ダイアログを出さずに、`PrepareSend` の応答の確認トークンですぐ `Send` を呼ぶ。バックエンドは設定を見ず、確認トークンなしの送信を拒否し、内容ハッシュも照合する。`PrepareSend` の `warnings` が 1 件でもあるときは、設定がオフでも確認ダイアログを出す（[機能要件 F-06](../requirements/functional.md)）
- **確認トークン**: 暗号論的乱数 128bit。セッションに紐づけてメモリのみで保持する。有効期限 5 分、1 回限り（使用で失効）。`PrepareSend` を再度呼ぶと以前のトークンは無効になる
- **内容ハッシュ**: `PrepareSend` が受け取った `ComposeContent`、送信元アカウント ID、添付（ID・サイズ）、`archiveAfterSend` を正規化して SHA-256 を取り、トークンに紐づける。`Send` は渡された `content` で同じ計算を行い、**不一致なら `CONFIRM_MISMATCH`**（確認後に宛先や本文を変えて送信することを防ぐ）
- `Send` はトークンがない・未知なら `CONFIRM_REQUIRED`、期限切れ・使用済みなら `CONFIRM_EXPIRED`
- 検証順序: 送信元（`SENDER_REQUIRED`）→ 宛先（`NO_RECIPIENTS`）→ アドレス形式（`INVALID_ADDRESS`）→ 送信元整合（`SENDER_MISMATCH`）。件名・本文が空のときは**エラーにせず警告**（`EMPTY_SUBJECT`／`EMPTY_BODY`）
- 送信元の整合（`SENDER_MISMATCH`）: セッションのアカウントと、スレッドの所有アカウント（`threads.account_id`／セッション作成時の `ThreadRef.accountId`）が一致すること。`From` ヘッダは `accounts.email` から生成し、本文・ヘッダに由来する値を使わない
- 送信は `archiveAfterSend` が真でも**送信成功後**にアーカイブする。アーカイブ失敗時は `SendResult.archived = false`、`archiveError` に理由を入れ、送信自体は成功として扱う
- 送信が失敗したとき（`messages.send` は二重送信を避けるため 5xx を再試行しない）は、確認トークンは使用済みになるが、作成セッションと本文はそのまま残る。利用者は内容を保ったまま、`PrepareSend`（F-06 の確認）からやり直して再送できる
- 送信の転送（`messages.send`・添付の取得）には総時間の上限を掛けず、呼び出し側 `ctx` の期限と応答ヘッダー待ちで打ち切る
- `Send` がリクエストを送った後に応答を受け取れず失敗したとき（切断・タイムアウト・期限切れ・呼び出し側のキャンセル）は、再送を許す前に Gmail の送信済みを確認する。Gmail API は送信時に `Message-ID` を付け替えることがあるため、MIME に付ける独自ヘッダ `X-Hitomado-Send-Id`（送信ごとの ULID。`Message-ID` のローカル部と同じ値）を主な手掛かりにする。`users.messages.list`（`q=in:sent after:<送信開始の 10 分前の UNIX 秒>`、新しい順に最大 20 件）で候補を取り、各候補を `users.messages.get`（`format=metadata`、`metadataHeaders=X-Hitomado-Send-Id,Message-ID`）で見て、`X-Hitomado-Send-Id` か `Message-ID` が一致すれば成功として返す
- 上記の確認で見つからなかった・確認自体に失敗したときは、**未送信とは決めつけず** `SEND_UNCERTAIN`（`OFFLINE` にしない）を返す。独自ヘッダが送信済みに残る保証も、検索インデックスが送信直後に追い付く保証も無く、「見つからない」は「送られていない」の根拠にならないため、再送を促さない（利用者が送信済みフォルダーを確認する）。`OFFLINE`（再送可）のままにするのは、リクエストを送れていないと言える接続前の失敗（名前解決失敗・接続拒否・アクセストークン更新の通信失敗）と、リクエストを送る前（レート制限の待ち中）に `ctx` の期限が切れた場合だけで、この場合は確認の検索もしない。リクエストを送る前に呼び出し側がキャンセルした場合は `OFFLINE` ではなく `CANCELLED`（`retryable: false`）を返す。リクエストを送った後のキャンセルは上記のとおり確認の対象で、確認は呼び出し側のキャンセルとは切り離して（最大 30 秒）行い、見つからなければ `SEND_UNCERTAIN` になる（`CANCELLED` にはならない）
- MIME: `multipart/alternative`（`text/plain`＋`text/html`）。添付があれば `multipart/mixed`。ヘッダは `From`、`To`、`Cc`、`Bcc`、`Subject`（RFC 2047）、`In-Reply-To`、`References`、`Date`、`Message-ID`、`X-Hitomado-Send-Id`

#### 5.2.1 送信済み確認の実 Gmail での確認結果（#72）

リポジトリ内のテストは Gmail 互換の fake サーバーで、実 Gmail では確かめていない。実アカウントでの確認は未実施で、結果は下の表に環境・日付つきで記録する（記録がない項目は未確認として扱い、仕様の前提を「確認済み」と読まない）。

| 項目 | 確認方法 | 結果（環境・日付） |
|------|----------|--------------------|
| 送信済みのコピーに `X-Hitomado-Send-Id` が残り、`get(format=metadata, metadataHeaders=X-Hitomado-Send-Id)` で読める | 実アカウントで送信し、送信済みのコピーをその形で取得する | 未確認 |
| 送信直後の `in:sent after:<時刻>` に現れるまでの遅延（現れない間は `SEND_UNCERTAIN`） | 送信直後から数秒おきに `users.messages.list` を呼び、初めて現れるまでを測る | 未確認 |
| `confirmSendCandidates`（20 件）・`confirmSendMargin`（10 分）（`internal/gmail/send_confirm.go`）の妥当さと list／get のクォータ消費 | 送信の多いアカウントで、確認 1 回の list 1 回＋get 最大 20 回が上限・クォータに収まるか | 未確認 |
| Gmail が `Message-ID` を付け替える前提（Google Issue Tracker 446748944） | 送信前の `Message-ID` と、送信済みのコピーの `Message-ID` を比べる | 未確認 |

動かない項目があれば、確認方法の見直しを別 Issue にする。メールアドレス・本文・トークンは記録に残さない。

## 6. AIService

```ts
type AIProvider = 'anthropic' | 'openai' | 'chatgpt';   // openai は OpenAI 互換（baseUrl 指定可）。chatgpt は Sign in with ChatGPT（利用者自身の ChatGPT プラン）

type AIConfig = {
  provider: AIProvider;
  model: string;
  baseUrl?: string;           // openai のみ。未指定は OpenAI 既定
  effort?: string;            // 下書きの生成に渡すモデルのエフォート（reasoning effort）。未指定は「モデルの既定」（プロバイダへ送らない）。`anthropic` は `low`／`medium`／`high`／`xhigh`／`max`、`openai`／`chatgpt` は `low`／`medium`／`high`（#111）
  configured: boolean;        // AI の設定（`SetAIConfig`）を一度でも保存したか。偽のあいだ、provider は既定の `chatgpt`・model は空（画面は「AI で下書き」で初回の案内を出す）。壊れた保存値・未知のプロバイダの保存値も偽（既定として扱う）
  hasApiKey: boolean;         // 現在のプロバイダの資格情報が設定済みか（anthropic／openai は API キー、chatgpt は ChatGPT へのログイン）。キー・トークン自体は返さない
  apiKeyProviders: (AIProvider | 'typesafe')[];       // API キーを登録済みのプロバイダ（anthropic／openai／typesafe（TypeSafe AI。#314）。chatgpt は含めず chatgptSignedIn で見る）。保存前に選んだプロバイダのキーの有無を出すのに使う。無ければ空配列で null にしない
  consentedProviders: (AIProvider | 'typesafe')[];    // 現在保存している設定の送信先に同意済みのプロバイダ（保存前に選んだ送信先の同意は、`ListAIModels` の `consent_required` で分かる）。`typesafe`（TypeSafe AI。Inbox Zero の判定。#314）は送信先が固定（api.typesafe.ai）で、選択中のプロバイダや Base URL を変えても（`SetAIConfig`）失効しない
  chatgptSignedIn: boolean;   // 選択中のプロバイダに関わらず、使用中の ChatGPT アカウントがログイン済みか
  chatgptEmail?: string;      // 使用中のアカウントのメールアドレス（表示用。検証済み ID トークンのもの）
  chatgptAccounts: ChatGPTAccount[];   // 保存済みの ChatGPT アカウント（登録順。無ければ空配列で null にしない。トークンは含まない）
};

// ChatGPT アカウント = クライアント登録 1 件。同じメールアドレスでも、別の登録なら別のアカウントとして扱う
type ChatGPTAccount = {
  id: string;                 // 登録の識別子（発行された client_id）
  email?: string;
  signedIn: boolean;          // ログイン中のセッションがあるか（ログアウト済みでも登録は残る）
  active: boolean;            // 下書きの生成に使うアカウントか（高々 1 件。ログアウト済みでも、選択は変わらないので true のまま残る。`signedIn` と合わせて判断する）
  planNoticePending: boolean; // 初めてログインして ChatGPT プランの利用が有効になったのに、「You're using your ChatGPT plan」の確認をまだ済ませていないか（下記。2 回目以降のログインでは立たない）
};

// efforts: そのモデルが受け付けるエフォートの段階（弱い順）。`anthropic` は Models API の `capabilities.effort` から作る（`supported` が偽、または取れなければ省略＝そのモデルにエフォート設定は出さない）。`openai`（OpenAI 互換）と `chatgpt` の一覧はモデルごとの段階を返さないので、常に省略する（画面は共通の `low`／`medium`／`high` を出す。§6.1）
type AIModel = { id: string; displayName?: string; efforts?: string[] };
type AIModelList = {
  provider: AIProvider;
  models: AIModel[];          // 取得できないときは空（null にしない）
  reason?: 'no_credentials' | 'consent_required' | 'invalid_base_url' | 'unsupported' | 'unauthorized' | 'rate_limited' | 'network' | 'failed';
  message?: string;           // reason の短い説明（利用者に表示してよい。キー・応答本文・URL は含めない）
};
type ChatGPTSignOutResult = { config: AIConfig; revocationConfirmed: boolean };

// スレッドの AI 要約・質問の結果（F-17・F-18）。`text` は日本語のプレーンテキスト（フロントは文字のまま表示し、Markdown・HTML としては解釈しない）。`totalCount` はスレッドの（下書きを除く）メッセージ数、`includedCount` は入力に含めた数（長いスレッドで古いものを外すと `includedCount < totalCount`）
type ThreadAIResult = { text: string; includedCount: number; totalCount: number };

// スレッドへの AI 質問の会話 1 件（F-18）。`user` は利用者の質問、`assistant` は AI の回答。Go は会話を保持せず、呼び出しごとにフロントが渡す
type ThreadAITurn = { role: 'user' | 'assistant'; text: string };

// Inbox Zero（#314。§6.3）
type InboxZeroReasons = {
  ad: boolean;            // 広告・宣伝（TypeSafe AI が判定）
  notification: boolean;  // 通知だけのメール（TypeSafe AI が判定）
  newsletter: boolean;    // 未読のニュースレター（TypeSafe AI が判定。スレッドが未読のときだけ）
  replied: boolean;       // 返信済みのメール（端末内で判定。TypeSafe AI は使わない）
};
type InboxZeroSettings = { reasons: InboxZeroReasons };
type InboxZeroCandidate = {
  ref: ThreadRef;
  from: string;           // 最新の（下書きでない）メッセージの差出人。表示名が無ければメールアドレス
  subject: string;
  reason: 'ad' | 'notification' | 'newsletter' | 'replied';
};
type InboxZeroResult = {
  candidates: InboxZeroCandidate[];  // 渡された順。無ければ空配列で null にしない
  examined: number;                  // 調べたスレッド数（重複を除く）
  skipped: number;                   // TypeSafe AI の判定に失敗して候補にしなかったスレッド数
};
```

| メソッド | 引数 | 戻り値 | 内容 |
|----------|------|--------|------|
| `GetAIConfig()` | — | `AIConfig` | 設定を保存していないときは `provider = "chatgpt"`・`model = ""`・`configured = false`（既定のプロバイダは Go の `defaultProvider`。保存済みの `anthropic`／`openai`／`chatgpt` は変えない。#104） |
| `SetAIConfig(provider, model, baseUrl?, effort?)` | | `AIConfig` | `app_settings.ai.config` を更新。`baseUrl` は `openai` のみ有効で、`https://` の URL か、ホストが `localhost`／`127.0.0.1`／`::1` に完全一致する `http://` の URL に限る。それ以外（外部ホストへの `http://`、userinfo・クエリ・フラグメント付き等）は `VALIDATION`（メッセージに URL は含めない）。Base URL が変わって送信先（scheme＋ホスト）が変わると、そのプロバイダの同意は無効になる。`effort`（#111）は省略・空で「モデルの既定」（保存するときも `effort` キーを書かない）。指定できる値は `AIConfig.effort` に挙げたプロバイダごとの集合で、それ以外は `VALIDATION`。モデル名との組み合わせ（そのモデルが受け付けるか）は保存時に確かめない（確かめるのは画面。生成時にプロバイダが拒否したら §6 の `AI_FAILED`）。プロバイダ・モデルを変えて保存するときは、画面が `effort` を渡し直す（渡さなければ「既定」に戻る。前の値を黙って引き継がない） |
| `SetAIApiKey(provider, apiKey)` | string ×2 | `AIConfig` | 資格情報ストアへ保存。`provider` は `anthropic`／`openai`／`typesafe`（TypeSafe AI（Jev）。文章を書かない分類用のモデルで、Inbox Zero の判定（`FindInboxZeroCandidates`）だけに使う。返信下書きのプロバイダではないので `SetAIConfig`・`ListAIModels` には使えない（`VALIDATION`）。#314）。`chatgpt` は API キーを使わないので `VALIDATION` |
| `ClearAIApiKey(provider)` | string | `AIConfig` | キーを削除（`typesafe` も同じ）。`chatgpt` は `VALIDATION`（ログアウトは `SignOutChatGPT`） |
| `RecordAIConsent(provider, baseUrl?)` | string ×2 | `AIConfig` | 「スレッド内容が当該プロバイダ・送信先へ送信される」ことの同意を保存（`app_settings.ai.consent`）。同意は「プロバイダ＋送信先」単位で、`openai`（api.openai.com）と `chatgpt`（api.openai.com）は別の同意として取る。**保存済みの設定ではなく、渡された送信先に対する同意を記録する**ので、設定を保存する前に、選んだプロバイダ・Base URL に同意できる（#105）。`baseUrl` は `openai` のみ有効で、空は既定の送信先。`SetAIConfig` と同じ規則で検証し、使えない Base URL は `VALIDATION`。ほかの送信先の同意は消さない。返す `AIConfig.consentedProviders` は、保存済みの設定の送信先に対するもの。`provider = "typesafe"`（TypeSafe AI。Inbox Zero の判定。#314）にも同意を取れる（送信先は api.typesafe.ai に固定。`baseUrl` は無視して保存しない）。`typesafe` の同意は選択中のプロバイダに関わらず `consentedProviders` に含み、`SetAIConfig` で別のプロバイダ・Base URL に変えても消えない。同意の文面（フロント）は「件名・差出人・本文の先頭約 500 文字が TypeSafe AI へ送られる」ことを示す |
| `ListAIModels(provider, baseUrl?)` | string ×2 | `AIModelList` | 渡された送信先（`openai` は `baseUrl`。空は既定）から、使えるモデルの一覧を取得する。**保存済みの設定に依らず、保存前でも取得できる**（#105。下記）。MCP からは呼べない |
| `SignInChatGPT(accountId)` | string | `AIConfig` | OpenAI の公式画面（システムブラウザ）で ChatGPT にログインする。完了・中止・時間切れまでブロックする（下記）。`accountId` が空なら別の ChatGPT アカウントとして新しく登録する（追加）。保存済みの `id` を渡すと、その登録の `client_id` で再ログインする（別のアカウントでログインされたら `VALIDATION`、未登録の ID は `NOT_FOUND`）。ID トークンを検証して保存を終えるまで、使用中のアカウントとほかのアカウントのトークン・登録は変えず、成功したアカウントが使用中になる |
| `SelectChatGPTAccount(accountId)` | string | `AIConfig` | 下書きの生成に使う ChatGPT アカウントを切り替える。通信しない。ログイン中のアカウントだけ選べる（ログアウト済みは `AI_NOT_CONFIGURED`・`details.reason = "account_signed_out"`・メッセージは「このアカウントはログアウト済みです。「再ログイン」からもう一度ログインしてください。」、未登録は `NOT_FOUND`） |
| `CancelSignInChatGPT()` | — | — | 進行中のログインを中止（`SignInChatGPT` は `OAUTH_CANCELLED` で reject） |
| `SignOutChatGPT(accountId)` | string | `ChatGPTSignOutResult` | 指定した ChatGPT アカウントだけ、OpenAI 側のセッションを失効させてから、そのローカルのトークンを消す（ほかのアカウントのトークンと登録は残り、このアカウントの登録も残る）。失効を確認できなくてもローカルは消え、`revocationConfirmed = false` を返す（利用者には ChatGPT の設定から連携を解除できると伝える）。使用中のアカウントをログアウトしても使用中の選択は変えず（ほかのアカウントへ黙って切り替えない。`ChatGPTAccount` は `active = true`・`signedIn = false` のまま）、そのあいだ下書きは `AI_NOT_CONFIGURED`（ログインが要る）になるので、再ログインするか `SelectChatGPTAccount` でログイン中のほかのアカウントを選ぶ。使用中の選択が外れるのは `RemoveChatGPTAccount` で使用中だった登録を忘れたときだけ。未登録の ID は `NOT_FOUND` |
| `AckChatGPTPlanNotice(accountId)` | string | `AIConfig` | 「You're using your ChatGPT plan」の確認を済みにする（`planNoticePending` が下りる。何度呼んでも同じ。未登録の ID は `NOT_FOUND`） |
| `RemoveChatGPTAccount(accountId)` | string | `AIConfig` | ログアウト済みの ChatGPT アカウントの登録（`client_id`・本人情報）を忘れる。ログイン中のアカウントは `VALIDATION`（先にログアウト）。未登録の ID は `NOT_FOUND` |
| `GenerateReplyDraft(sessionId, requestId, instruction?)` | string ×3 | `{ bodyHtml: string }` | 返信系セッション（`reply`／`reply_all`）のスレッドから下書きを生成する。署名・引用は含めない。`new`／`forward` は `VALIDATION`。`instruction` は利用者が書いた「AI への指示」（省略・空・空白だけは指示なし＝従来の下書き。前後の空白は除く。2,000 文字（rune 数）を超えると、通信せず `VALIDATION`）。保存はしない（ログ・DB・資格情報ストアのいずれにも残さない） |
| `SummarizeThread(ref, requestId)` | `ThreadRef`, string | `ThreadAIResult` | 開いているスレッドの要約を生成する（F-17。#206）。`ref` のスレッドを `threads.get(format=full)`（メモリ LRU を優先）で読み、全メッセージを入力にする（下記「スレッド AI の入力」）。`text` は経緯・現在の状況・受信アカウントの利用者に求められている対応を含む短い箇条書き。保存しない。スレッドが無ければ `NOT_FOUND`、アカウントが要再接続なら `NEEDS_REAUTH` |
| `AskAboutThread(ref, requestId, question, history)` | `ThreadRef`, string, string, `ThreadAITurn[]` | `ThreadAIResult` | 開いているスレッドの内容を文脈に、利用者の質問へ答える（F-18。#207）。入力は「スレッド AI の入力」（`SummarizeThread` と同じ全メッセージ）に、`history`（そのスレッドで今までに交わした質問と回答）と今回の `question` を加えたもの。`text` は回答（日本語のプレーンテキスト）。保存しない。`question` は前後の空白を除いて空なら `VALIDATION`、2,000 文字（rune 数）を超えると通信せず `VALIDATION`。`history` は `null`・空でもよい。形が正しくないとき（先頭が `assistant`・`user` と `assistant` が交互でない・最後が `user`・`text` が空）、20 件（10 往復）を超えるとき、`text` の合計が 20,000 文字を超えるときは、通信せず `VALIDATION`（古い往復を外すのはフロントの責務で、Go は黙って切り詰めない）。スレッドが無ければ `NOT_FOUND`、アカウントが要再接続なら `NEEDS_REAUTH` |
| `CancelGenerate(requestId)` | string | — | 生成を中止（`GenerateReplyDraft`・`SummarizeThread`・`AskAboutThread`・`FindInboxZeroCandidates` は `CANCELLED` で reject）。**生成が始まる前に届いても効く**（#352）: 各メソッドは、前提の検査（ChatGPT のトークン更新の通信を含む）の前に `requestId` を登録するので、検査の最中の中止はその通信を中断して `CANCELLED` になる。まだ登録されていない `requestId` の中止は 1 分間だけ「中止済み」と覚え、同じ `requestId` の開始を、プロバイダ（TypeSafe AI を含む）へ何も送らず `CANCELLED` にする（記録は 1 回で消費し、1 分で捨てる。`requestId` は呼び出しごとに一意） |
| `GetInboxZeroSettings()` | — | `InboxZeroSettings` | Inbox Zero の「不要なメール」の理由ごとのオン・オフ（`app_settings.inbox_zero.reasons`）。未設定・壊れた保存値は 4 つとも `true`（#314） |
| `SetInboxZeroReasons(reasons)` | `InboxZeroReasons` | `InboxZeroSettings` | 理由ごとのオン・オフを保存して返す。全部オフも保存できる（そのとき `FindInboxZeroCandidates` が `VALIDATION`）。通信しない |
| `FindInboxZeroCandidates(refs, requestId)` | `ThreadRef[]`, string | `InboxZeroResult` | `refs`（フロントが読み込み済みの、現在のタブの一覧）のうち、オンの理由に当てはまる「アーカイブ候補」を返す（§6.3）。アーカイブはしない（フロントが `InboxService.Archive`／`Unarchive` を使う）。MCP からは呼べない |

- `GenerateReplyDraft` の前提: キー設定済み（`AI_NOT_CONFIGURED`。`chatgpt` ではログイン済み）・同意済み（`AI_CONSENT_REQUIRED`）。タイムアウトは 60 秒（`AI_FAILED`）。プロバイダの応答が 3xx のときは、同意した送信先以外へ内容を再送しないよう追従せず `AI_FAILED` とする（Base URL にはリダイレクト後の最終的な URL を指定する）
- 送信するコンテキスト: 直近 5 メッセージ・合計 20,000 文字以内（NF §12）。HTML はテキスト化して送る。機密のログ出力は禁止（NF-S10）
- 自動送信・自動生成は行わない。本メソッドはボタン押下からのみ呼ばれる
- `instruction` の扱い（#109）: プロンプトでは、スレッド本文とは別の「利用者からの指示」として渡す（スレッド本文に含まれる指示より優先し、「出力は返信の本文だけ・署名と引用を含めない」という形式の規則は上書きさせない）。送る先・同意の単位（プロバイダ＋送信先）・コンテキストの上限（直近 5 メッセージ・20,000 文字）は変えず、指示は 20,000 文字の枠に含めない（別に 2,000 文字まで）
- `instruction` の置き場所（#109）: 利用者本人が書いた信頼できる入力なので、**system 指示側の専用ブロック（「利用者からの指示」）に入れる**。スレッド本文（信頼しない入力）が入る user メッセージには混ぜない（混ぜると、既存の「スレッド内の文章に含まれる指示は、この指示より優先しない」という区別が崩れる）。`instruction` が空・空白だけのときは専用ブロックごと省き、プロンプトは従来と完全に同じになる
- `effort` の渡し方（#111）: 設定されているときだけ、生成の要求に載せる（未設定なら載せず、従来と同じ要求になる）。`anthropic` は Messages API の `output_config.effort`、`openai` は Chat Completions の `reasoning_effort`、`chatgpt` は Responses API の `reasoning.effort`。**effort はメール内容ではなくパラメータなので、同意の単位・送信先は変わらない**。プロバイダが拒否したとき（400 など）は `AI_FAILED`（`details.reason = "effort_rejected"`。メッセージは「このモデルは選んだエフォートに対応していません。設定でエフォートを「既定」に戻してください。」。応答本文は含めない）
- `SummarizeThread` の前提と失敗（#206）: `GenerateReplyDraft` と同じ。キー設定済み（`AI_NOT_CONFIGURED`）・同意済み（`AI_CONSENT_REQUIRED`）・タイムアウト 60 秒（`AI_FAILED`）・プロバイダの 3xx は追従せず `AI_FAILED`・`CancelGenerate` で `CANCELLED`。**ボタン押下からのみ呼ばれる**（自動呼び出し・先読みはしない）。MCP からは呼べない。同意の単位（プロバイダ＋送信先）・`effort` の渡し方も同じで、新しい同意・設定は設けない
- `AskAboutThread` の前提と失敗（#207）: `SummarizeThread` と同じ（`AI_NOT_CONFIGURED`・`AI_CONSENT_REQUIRED`・タイムアウト 60 秒の `AI_FAILED`・3xx は追従せず `AI_FAILED`・`CancelGenerate` で `CANCELLED`・`effort`）。**質問の送信操作からのみ呼ばれる**。MCP からは呼べない。同意の単位は変わらず、新しい同意・設定は設けない。**回答がスレッドから分からないことは失敗にしない**（AI が「スレッドからは分かりません」と答えた本文を `text` として返す。`AI_FAILED` にしない）
- `AskAboutThread` のプロンプト（#207）: 役割の指示（system 側。スレッドに書かれていることだけを根拠に答える・分からなければ「スレッドからは分かりません」と答える・推測や外部の情報で埋めない・日本語のプレーンテキストで答える・メールの送信や操作はしない）、スレッドの内容（**信頼しない入力として明示した先頭の user メッセージ**。スレッド内の指示には従わない）、会話（`history` を user／assistant の順にそのままメッセージとして並べる）、今回の `question`（最後の user メッセージ）の順に組む。`question` と `history` の `user` は利用者本人が書いた入力、`history` の `assistant` は過去の回答であり、どれもスレッド本文の枠には混ぜない。`SummarizeThread` の要約は文脈に入れない（原文から答える）。メッセージの並びと system の置き場所はプロバイダごとの API に従う（Anthropic Messages: `system`＋`messages`／OpenAI 互換 Chat Completions: `messages`／ChatGPT プラン Responses API: `instructions`＋`input`）
- **スレッド AI の入力**（`SummarizeThread`・F-18 の `AskAboutThread` で共通）: 対象は `ref` の 1 スレッドの全メッセージ。古い順に、差出人・日時・本文（HTML はテキスト化。`textconv`）を渡す。`DRAFT` ラベルのメッセージは含めない。添付はファイル名とサイズだけで、中身は送らない。合計 40,000 文字（NF §12）を超えるときは古いメッセージから外して新しい側を残し（最新の 1 件だけで超える場合は、その本文の先頭を切り詰める）、`includedCount`／`totalCount` で知らせる。`GenerateReplyDraft` の「直近 5 メッセージ・20,000 文字」とは別の枠で、互いに影響しない
- **プロンプトの区別**（#206）: スレッド本文は信頼しない入力として、利用者の操作（要約の指示・質問）とは別の枠に入れ、スレッド内の文章に含まれる指示（「この指示に従ってください」等）には従わない。出力の形式（日本語のプレーンテキスト・箇条書き）はスレッド内の文章で変えさせない
- **保存しない**: 要約の本文・質問・回答・会話・スレッドの内容は、ログ・DB・資格情報ストアのいずれにも残さない。エラー文面にも含めない（NF-S10）

### 6.3 Inbox Zero（`FindInboxZeroCandidates`。#314）

受信箱の一覧のうち「もう読まなくてよさそうなメール」を、利用者が確認してからまとめてアーカイブするための候補を見つける。**判定だけを行い、アーカイブはしない**。ボタン押下からのみ呼ばれる（自動実行しない）。

- **プロバイダの分担**: 返信下書き・要約・質問は従来どおり既定のプロバイダ（§6）。Inbox Zero の判定は TypeSafe AI（Jev）だけで、Jev は下書きのプロバイダにはならない（`SetAIConfig`・`ListAIModels` の `typesafe` は `VALIDATION` のまま）
- **理由（4 つ。各オン・オフ。`GetInboxZeroSettings`／`SetInboxZeroReasons`）**: オンの理由だけを使う
  - `replied`（返信済み）: **端末内だけで判定する**。スレッドに自分のメッセージがある（下書きを除くメッセージのうち、Gmail ラベル `SENT` を持つもの、または `From` のアドレスがそのアカウントのメールアドレスと一致するもの。大文字小文字を区別しない）。Jev には聞かない。`replied` だけがオンのときは、API キーも同意も要らず、Jev のクライアントは作らない
  - `ad`（広告・宣伝）／`notification`（通知だけのメール）／`newsletter`（未読のニュースレター）: Jev が判定する。`newsletter` はさらに、スレッドが未読（`UNREAD` ラベルのメッセージが 1 通でもある）のときだけ候補にする
  - 返信済みと判定したスレッドは Jev に送らない（`replied` がオフのときは、通常どおり Jev の判定に従う）。1 スレッドの理由は 1 つ（`replied` が最優先）
- **Jev の呼び出し**: `POST https://api.typesafe.ai/v1/systemone`（`Authorization: Bearer <API キー>`）。本文は `{"state": "...", "model": "jev-latest", "questions": {"category": {"type": "choice", "instructions": "...", "criteria": {"ad": "...", "notification": "...", "newsletter": "...", "other": "..."}}}}`（選択肢の説明は日本語で、`other` は「通常のメール・個人宛て・重要なもの・判断できない」）。応答の `answers.category.choice`（選ばれた選択肢）と `answers.category.probabilities`（全選択肢の確率）を使う（[Choice](https://docs.typesafe.ai/primitives/choice.md)）。選択肢は、オフの理由も含めて常に全部を送る（オフの理由のメールを `other` に寄せず、その理由として選ばせたうえで候補から外すため）
- **候補にする条件**: 選ばれた選択肢の確率 `probabilities[choice]` が **0.8 以上**（確率の分布から算出される `confidence` ではなく、実際に行動する選択肢の確率で判断する）、かつ選ばれた選択肢が有効な理由（`other` ではない・オンである・`newsletter` は未読）。1 位の理由がオフ・対象外のスレッドは候補にしない（2 位の選択肢には落とさない）
- **Jev へ送る内容**: スレッドの最新の（下書きでない）メッセージの差出人・件名・本文の先頭約 500 文字（HTML は `textconv` でテキスト化）だけ。ほかのメッセージ・宛先・添付・アカウント情報は送らない。1 スレッド 1 リクエスト。初回の利用前に同意が要る（`RecordAIConsent("typesafe", "")`）
- **対象**: `refs`（重複は除く）は最大 100 件。超えると `VALIDATION`。1 回の呼び出しの中で、スレッドは `threads.get(format=full)`（メモリ LRU を優先。§10）で読む
- **並行・タイムアウト**: Jev への同時リクエストは 4 件まで。各リクエストのタイムアウトは 60 秒（AI の他の呼び出しと同じ）。`CancelGenerate(requestId)`・呼び出し側の中止で、実行中の判定を止めて `CANCELLED`
- **前提の検査（通信・スレッドの読み込みの前）**: `requestId` が空・`refs` が 101 件以上・理由がすべてオフ → `VALIDATION`。Jev の理由（`ad`／`notification`／`newsletter`）が 1 つでもオンで、TypeSafe AI の API キーが未登録 → `AI_NOT_CONFIGURED`（メッセージは「TypeSafe AI のキー」の登録を案内する）。同意が未取得 → `AI_CONSENT_REQUIRED`
- **失敗の扱い**: Jev が 401／403（キー不正）→ `AI_FAILED`、429 → `RATE_LIMITED`（`retryable`）で全体を止める（途中までの結果は返さない）。ほかの 1 スレッドの失敗（5xx・不正な応答・タイムアウト・通信エラー、読み込み後に消えたスレッド）は、そのスレッドを候補にせず `skipped` に数えて続ける。Gmail のスレッド取得の失敗（認証切れなど）は §1.2 の対応するコードで全体を止める
- **保存しない・ログに出さない**: API キー・件名・差出人・本文は、ログ・エラー文面・結果のどこにも残さない（NF-S10）。結果（`candidates`）は保存しない

### 6.1 モデル一覧（`ListAIModels`）

- 取得先: `openai` は引数の `baseUrl`（空なら既定）の `GET {base}/models`、`anthropic` は `GET https://api.anthropic.com/v1/models`（`limit=1000`、`has_more` の間 `after_id` で最大 5 ページ）、`chatgpt` は `GET https://api.openai.com/v1/models`（ログインのアクセストークン。`visibility == "list"` のモデルだけを、サーバーの順で）。**取得先は保存済みの設定（`app_settings.ai.config`）ではなく、引数の `provider`・`baseUrl` で決める**（`baseUrl` は `openai` 以外では無視する）ので、設定を保存する前に、選んだプロバイダ（と Base URL）の一覧を取得できる（#105）。キーは資格情報ストアから読み、フロントには渡さない（キーはプロバイダごとに保存されるので、保存前に選んだプロバイダのキーも読める）
- 呼ぶ契機（フロントの責務。定期的には呼ばない）: プロバイダ（OpenAI 互換は Base URL が決まったとき）を選んだとき、API キーの登録・変更のあと、ChatGPT のログインのあと、同意の画面で「同意する」を押したあと、設定画面の「一覧を更新」。流れは [コンポーネント設計 §3.9.1](../components/overview.md#391-ai-設定と初回の案内プロバイダ選択同意モデル一覧の流れ104105)
- 同意の判定: 引数の「プロバイダ＋送信先」（`openai` は `baseUrl` の scheme とホスト。`RecordAIConsent` と同じ単位）に同意していなければ、取得の通信を送らず `consent_required` を返す。画面はこれを、同意の画面（F-08）を出す契機にする（同意の画面を出すかどうかの判断も、UI ではなくこの結果で決める）。同意は `RecordAIConsent(provider, baseUrl)` で保存してから、`ListAIModels` を呼び直す
- 一覧が取れなくてもエラーにしない（`models` が空で `reason`／`message` を返し、画面は手入力へ切り替える）。**同意前の送信先（`consent_required`）・使えない Base URL（`invalid_base_url`）・キー未登録／未ログイン（`no_credentials`）には取得の通信を送らない**（判定の順は `invalid_base_url` → `consent_required` → `no_credentials`）。https のみ（loopback の http を除く）・リダイレクトに従わない・本文の上限、という `GenerateReplyDraft` と同じ規則を適用する。Models API が無い（404／405／501）は `unsupported`、401／403 は `unauthorized`、429 は `rate_limited`、通信失敗・時間切れは `network`。呼び出し側のキャンセルだけが `CANCELLED`
- エフォートの段階（#111）: `anthropic` はこの一覧の各モデルの `capabilities.effort`（`low`／`medium`／`high`／`xhigh`／`max` のうち `supported` なもの。`xhigh` は無いモデルがある）を `AIModel.efforts` に入れる（出典: [Effort](https://platform.claude.com/docs/en/build-with-claude/effort)・[List Models](https://platform.claude.com/docs/en/api/models/list)）。`openai` と `chatgpt` は、一覧がモデルごとの対応する段階を返さない（OpenAI の `GET /models` は ID などだけ。ChatGPT プランの一覧に段階が含まれるかは未確認）ので `efforts` は省略し、画面は `low`／`medium`／`high` を出す（`none`・`minimal`・`xhigh` などはモデルによって 400 になるため既定の選択肢にしない）。OpenAI 互換の送信先（ローカルの LLM サーバーなど）は `reasoning_effort` を受け付けないことがあるので、既定は「送らない」のままにする
- 保存済みのモデルが一覧に無くても、設定は変えない（画面はそのモデルを選択肢に残す）。一覧は使えることの保証ではなく、選んだモデルが使えるかは下書きの生成が成功して初めて分かる（ChatGPT プランのモデル一覧は特に、アカウントの権限の確認ではない）。一覧の取得は設定も同意も変えない（保存するのは `SetAIConfig`）
- ログには件数（成功）と理由コード（失敗）だけを残す。キー・トークン・応答の中身・モデル名は残さない

### 6.2 ChatGPT プラン（Sign in with ChatGPT）

OpenAI の公式ドキュメント（[Token sharing for open source](https://developers.openai.com/siwc/token-sharing-open-source)）のオープンソース向けフローに従う。規約（[Sign in with ChatGPT Terms](https://openai.com/policies/sign-in-with-chatgpt-terms/)）の要件として、**トークンは公式のサインインフローだけで取得する／パスワードもセッション Cookie も受け取らない／OpenAI やほかのアプリを名乗らない／Codex CLI など他アプリの認証情報は読まない・使わない／プラン枠やトークンを共有しない**。

- **ログイン**（`SignInChatGPT`）: ブラウザで `https://auth.openai.com/api/accounts/authorize`（エンドポイントは `/.well-known/openid-configuration` から取得）を開く。PKCE（S256）・`state`・`nonce` を試行ごとに作り、リダイレクト先は `http://127.0.0.1:<一時ポート>/auth/callback`（`localhost` は使わない。`state` が合わない要求は無視）。`scope` は `openid profile email offline_access resource.invoke chatgpt.tokens.use.direct`、`resource` は `https://api.openai.com/v1`。client secret は持たない公開クライアント
- **複数アカウント**（[Accounts and sessions](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions)）: ChatGPT アカウント（クライアント登録）ごとに、発行された `client_id`・`sub`・メール・トークン（資格情報ストアのキーも `client_id` ごと）を別々に保存し、同じメールアドレスでも別の登録として扱う（同じアカウントでもう一度「追加」すると、別の `client_id` の別の登録になる）。ホスト ID（`ext_agent_host_id`）は全アカウントで共通。設定画面は保存済みのアカウントを一覧にして、使うアカウントを選べる（`SelectChatGPTAccount`。ログアウト済みの行はラジオが無く「再ログイン」を出すが、画面が古いなどでログアウト済みのアカウントを選んだときは、`details.reason = "account_signed_out"` を見て、画面が「このアカウントはログアウト済みです。「再ログイン」からもう一度ログインしてください。」を設定のエラー欄（`role="alert"`）に出し、一覧を読み込み直して「再ログイン」を押せる状態にする。文面は `messages`（`settings.ai.chatgpt.selectSignedOut`）。生成の前提の案内（未ログイン）とは別の文面）。別のアカウントの追加（`SignInChatGPT('')`）・再ログイン（`SignInChatGPT(id)`）は、ID トークンを検証して保存を終えるまで、使用中のアカウントにも保存済みのトークンにも影響しない。ほかのアカウントの `client_id`・ID トークン・メールを認可要求に混ぜない。生成・更新・失効は、使用中のアカウント（更新・失効は対象のアカウント）の `client_id` とトークンだけを使う。新規登録で発行された `client_id` が保存済みの登録と同じだったときは、`sub` が同じ場合だけ同じ登録を更新し、`sub` が違う場合も、保存済みの登録に `sub` が未記録で（旧形式から移行した直後など）同じアカウントと確認できない場合も、既存の登録・トークンを上書きせずに拒否する（`VALIDATION`。得たリフレッシュトークンは失効させる。発行される `client_id` は一意なので通常は起きない。登録済みのアカウントは「再ログイン」で入れ、成功すると `sub` が記録される）。Windows 資格情報マネージャーは資格情報キーの大文字小文字を区別しない（`Abc` と `abc` は同じキー）ので、`client_id` は大文字小文字を無視して一意として扱い、保存済みの登録と大文字小文字だけが違う `client_id` が発行された新規登録は、`sub` に関わらず同じく拒否する（`VALIDATION`・得たリフレッシュトークンは失効）。選んだ登録への再ログインで返った `client_id` が選んだ値と大文字小文字だけ違うときも、コードを交換する前に拒否する。資格情報キーの形式は変えない（変えるとリリース済みのキーの移行が要るため。発行される値は一意なので、拒否だけで足りる）
- **クライアント登録**: 初回は `client_id=dynamic_agent_client`・`agent_name_hint=Hitomado`・ホスト ID（`ext_agent_host_id`。`urn:uuid:<UUIDv4>`。初回ログインの前に作って `app_settings.ai.chatgpt` に保存し、以後同じ値）で動的登録し、コールバックが返す発行済みの `client_id`（`oaiapp_…`）を保存して以後の認可・更新・失効に使う（発行されなければ未完了として失敗）。再ログインは保存済みの `client_id`（`agent_name_hint` は付けない）と、`login_hint`（メール）・保持している `id_token_hint` を使う。この登録に事前の申請や承認は要らない（OSS 向けの動的登録）
- **検証**: コードをトークンに交換したあと、ID トークンの署名（OpenAI の JWKS）・`iss`・`aud`（発行済みの `client_id`）・`exp`・`nonce` を検証し、`sub` を本人の識別とする。付与された `scope` に `chatgpt.tokens.use.direct` が無ければ ChatGPT プランは使えないとして、得たトークンは保存せず（リフレッシュトークンは失効させる）`OAUTH_DENIED` を返す。ログイン中のアカウント（使用中のアカウントを含む）のセッションは変えない。そのアカウントにセッションが無いときだけ、登録（クライアント ID）をログアウト済みのアカウントとして残し、その登録での次のログインでは `prompt=consent` で同意画面を求め直す。再ログインで選んだ登録と別の `sub` のログインは `VALIDATION` で拒否し（得たリフレッシュトークンは失効させる）、何も上書きしない（別のアカウントを使うには、追加する）。ブラウザで拒否（`access_denied`）は `OAUTH_DENIED`、5 分の時間切れは `OAUTH_TIMEOUT`
- **保存**: アクセストークン・リフレッシュトークン・ID トークンは資格情報ストア（`hitomado/chatgpt_token/<client_id>/{access,refresh,id}`。1 件の上限を超える値は分割して保存）にだけ置く。ホスト ID・使用中のアカウント・アカウントごとの `client_id`・`sub`・メール・アクセストークンの期限（秘密ではない）は `app_settings.ai.chatgpt`（形式は [ER §3.9](../architecture/er.md)）。v0.1.7 までの 1 アカウントの形式（`hitomado/chatgpt_token/{access,refresh,id}`）は、最初の読み取りで、再ログインなしに新しい形式へ移す（新しい保存先へ書いてから登録を置き換え、最後に旧キーを消す。途中で失敗しても再実行できる。登録を新しい形式へ置き換えたあとに旧キーの削除だけが失敗した場合も、旧キーが残っている間は、プロセスの最初の呼び出し（削除に失敗し続けている間は呼び出しごと）に旧キーの削除をやり直す。旧キーの有無は読むだけで確かめ、無ければ何も消さない・新しい形式のキーには触れない。削除に失敗しても移行したアカウントは失わず、状態の取得・ログインは使える）。トークンはログ・エラーメッセージ・`AIConfig`・MCP のどこにも出さない（エラーはエラーコード名だけを持ち、応答本文は捨てる）
- **更新**: アクセストークンは 1 時間。期限の 60 秒前から、`grant_type=refresh_token`・発行済みの `client_id`・`resource` で更新し、アクセス・リフレッシュ（置き換え）・ID トークンをまとめて保存する（更新・ログアウトはアカウントごとに直列化し、同じアカウントの並行更新は 1 回にまとまる。別のアカウントの更新・ログアウトは互いを待たない）。`invalid_grant`・`invalid_refresh_token`・`token_expired`・`refresh_token_expired`・`refresh_token_invalidated`・`refresh_token_reused` は、更新に失敗したアカウントのトークンだけを消して再ログインを案内（`AI_NOT_CONFIGURED`。ほかのアカウントのトークンは残る）。通信失敗・5xx ではトークンを消さない。生成中に 401 を受けたときだけ、1 度更新してやり直し、それでも 401 なら `AI_FAILED`（ログインの確認を案内）。403（`subscription_sharing_user_not_eligible` 以外の方針・権限・地域の拒否など）・429・5xx では更新も再送もしない（公式ドキュメントが同じ要求の繰り返しを求めていないため）。トークンは同意の確認が済んでから取得・更新する。エラーコードは `error` が文字列・`error.code`・トップレベルの `code` のいずれでも判定する
- **ログアウト**（`SignOutChatGPT(accountId)`）: 指定したアカウントの失効エンドポイントへ `token_type_hint=refresh_token` で POST（通信失敗・5xx は最大 3 回）し、成否に関わらずそのアカウントのローカルのトークンだけを消す。ほかのアカウントのトークンと登録、ホスト ID、このアカウントの `client_id`（登録）は残す。登録を忘れるのは `RemoveChatGPTAccount`（ログアウト済みのアカウントのみ）
- **生成**: `POST https://api.openai.com/v1/responses`（`Authorization: Bearer <アクセストークン>`）。`store: false`・`stream: true`・`instructions`・`input`（配列）だけを送り、公式が未対応とする項目（`max_output_tokens`・`temperature`・`user`・`metadata` など）は送らない。`response.completed` を受けて初めて成功とし、`response.failed`／`response.incomplete`／途中で切れたストリームは失敗。`subscription_sharing_usage_limit_exceeded`（429）は `AI_FAILED`（`details.reason = "usage_limit_exceeded"`。画面は ChatGPT の使用状況の画面（`https://chatgpt.com/settings/usage`）を開く「Manage usage」を主なアクションにした案内を出す。更新も再送もしない）、`subscription_sharing_user_not_eligible`（403）は `AI_FAILED`（このアカウントでは使えない旨）。既存の Chat Completions の経路（`openai`）とは別の実装
- **初めてのログインの確認**（[UI/UX ガイドライン](https://developers.openai.com/siwc/ui-ux-guidelines)）: 初めてログインして ChatGPT プランの利用が有効になったとき（付与された `scope` に `chatgpt.tokens.use.direct` があり、保存できたとき）だけ、そのアカウントの `planNoticePending` が立つ。画面は「You're using your ChatGPT plan」の確認（プランの範囲で使うことの説明・「Got it」）を出し、「Got it」で `AckChatGPTPlanNotice` を呼ぶ。状態は `app_settings.ai.chatgpt` のアカウントごと（`planNotice`。[ER §3.9](../architecture/er.md)）に持つので、同じアカウントの再ログイン・ログアウト・再起動では立たず、別のアカウントを初めて追加したときだけ、そのアカウントで 1 度出る。登録は `client_id` 単位なので、アカウントを `RemoveChatGPTAccount` で忘れてから同じアカウントを追加し直したとき、および同じアカウントを 2 回追加したときは、新しい `client_id` の登録として扱い、そのアカウントでもう一度出る。許可されなかったログイン（`OAUTH_DENIED`）では立たない（あとで初めて有効になったときに立つ）。v0.1.7 までの登録は、本人情報（`sub`）があり `consentNeeded` でなければ（ログアウト済みの登録も含め）確認済みとして移す
- 使える対象: 公式ドキュメントが示すのは ChatGPT Plus／Pro。Business などほかのプランは、公式ドキュメントでは確認できていない（使えない場合は `subscription_sharing_user_not_eligible` で分かる）。使えるモデルは `ListAIModels('chatgpt')` が返す一覧による

#### 6.2.1 実機・実アカウントでの確認結果（#78）

偽の OpenAI サーバーでフロー全体はテストしたが、実際の ChatGPT アカウントでのログインは未実施。結果（成功・失敗・非対応）は下の表に環境・日付つきで記録する（記録がない項目は未確認）。トークン・メールアドレスは、ログ・Issue・PR・スクリーンショットに残さない。

| 項目 | 結果（環境・日付） |
|------|--------------------|
| ChatGPT Plus／Pro で「Continue with ChatGPT」→ OpenAI の画面 → 動的登録（`agent_name_hint=Hitomado`）→ `127.0.0.1` の一時ポートの `/auth/callback` → ログイン完了まで通る | 未確認 |
| 返るアクセストークン・ID トークンの実際の長さ（長さだけを記録する） | 未確認 |
| 資格情報マネージャーの 1 件の上限（約 2.5KB）を超えたときの分割保存（`hitomado/chatgpt_token/<client_id>/<kind>#N`）が正しく読み書きできる | 未確認 |
| 期限切れの自動更新（1 時間）・`id_token_hint` つきの再ログイン | 未確認 |
| ログアウトの失効（OpenAI 側で連携が外れる） | 未確認 |
| 「AIで下書き」で下書きが生成できる | 未確認 |
| `ListAIModels('chatgpt')` が実際に返す内容 | 未確認 |
| 利用枠を超えたときの応答（`subscription_sharing_usage_limit_exceeded`）と画面の表示 | 未確認 |
| ChatGPT Business など Plus／Pro 以外のプランで使えるか | 未確認 |
| 公式のサインイン画面・同意画面に表示される名前が `Hitomado` | 未確認 |

分割保存が実トークンで動かなければ修正する。

## 7. AppService

| メソッド | 引数 | 戻り値 | 内容 |
|----------|------|--------|------|
| `GetInboxFilter()` | — | `{ accountIds: string[]; unreadOnly: boolean }` | 保存済みフィルタ（`app_settings.inbox.filter`）。存在しないアカウント ID は除いて返す |
| `SetInboxFilter(filter)` | 同上 | — | 保存 |
| `GetAppInfo()` | — | `{ version: string; updateCheckEnabled: boolean; autoUpdateSupported: boolean; autoUpdateEnabled: boolean; sendConfirmEnabled: boolean; dataDir: string }` | `autoUpdateSupported` は新しい版の自動ダウンロードと入れ替えを行えるビルド（Windows のリリース版。Android・dev ビルド・開発用の Linux は `false`）。`autoUpdateEnabled` は自動アップデートの設定（既定 `true`。`autoUpdateSupported` が偽なら常に `false`）。`sendConfirmEnabled` は送信前の確認画面を出す設定（既定 `true`。未設定・壊れた値は `true`。#313） |
| `SetUpdateCheckEnabled(enabled)` | boolean | — | NF-D03。アプリ内の更新確認を持たない機能構成（`supportsAutoUpdate` が偽）では `UNSUPPORTED`。Android は更新確認を持つ（新しい版のお知らせとリンクだけ） |
| `CheckForUpdate()` | — | `{ available: boolean; version?: string; url?: string }` | GitHub Releases API を即時確認。無効時、および版が `0.0.0`（`VERSION` を注入しない dev・ローカルビルド）のときは通信せず `{ available: false }`。新しい版があり自動アップデートが有効（`autoUpdateEnabled`）なら、更新ファイルのダウンロードと照合をバックグラウンドで始める（応答は待たない。進みは `update:status`） |
| `SetAutoUpdateEnabled(enabled)` | boolean | — | NF-D03。自動アップデートのオン／オフ（`app_settings.update.auto_enabled`）。オフにするとダウンロード済みのファイルを捨てる。入れ替えを行えないビルド（`autoUpdateSupported` が偽）では `UNSUPPORTED` |
| `SetSendConfirmEnabled(enabled)` | boolean | — | 送信前に確認画面を出すか（`app_settings.compose.send_confirm`。既定オン。#313）。オフでも、`ComposeService.Send` は確認トークンなしでは拒否し（`CONFIRM_REQUIRED`）、送信元は受信アカウントに固定される。確認画面を省くのはフロントの送信フローだけで、`PrepareSend` を呼んでそのトークンで `Send` する（§5.2） |
| `GetUpdateStatus()` | — | `{ stage: 'none' \| 'downloading' \| 'ready' \| 'failed'; version?: string; message?: string }` | 自動アップデートの現在の進み（`update:status` と同じ形）。`ready` はダウンロードと `SHA256SUMS.txt` との照合が済んだ入れ替え待ち。`message` は失敗・注意の日本語の文面（URL・パスを含めない） |
| `RestartToUpdate()` | — | — | 「再起動して更新」。`stage` が `ready` のとき、終了を要求し（`AckBeforeQuit` を待って未保存の下書きを保存してから終了）、終了処理の最後でインストーラーを `/S /WAIT=60 /RELAUNCH` で分離起動して入れ替える。`ready` でないとき・送信中・下書きの保存中は `VALIDATION`（入れ替えない）。入れ替えを行えないビルドでは `UNSUPPORTED` |
| `AckBeforeQuit()` | — | — | `app:before-quit` を受けたフロントが、未保存の作成セッションを `SaveDraft` し終えたことを Go に伝える（§9）。Go はこれを受けるか猶予 3 秒の経過で終了処理に進む |

### 7.1 LogService

| メソッド | 引数 | 戻り値 | 内容 |
|----------|------|--------|------|
| `ReportClientError(error)` | `ClientError` | — | フロントの未処理例外・Promise の reject・Vue の描画エラー・バインディング呼び出しの失敗を Go のログへ記録する。`ClientError` は `{ kind: 'error' \| 'unhandledrejection' \| 'vue' \| 'binding'; message: string; stack?: string; source?: string; code?: string; accountId?: string }`。本文・件名・宛先の項目は持たない。画面側は例外のメッセージ文を送らず、`message` は種類のコード（`TypeError` などのエラークラス名・`unknown`・`render-error:コンポーネント名`・`binding` のエラーコード）、`stack` は関数名とファイル:行:列のフレームだけ。Go 側は 2 枚目の防御として、メッセージ・スタックはログの伏せ処理（NF-S10）を通り、長さを切る。1 秒に 1 件（瞬間 30 件）を超える報告は捨てる（エラーにしない）。`kind` が不正なら `VALIDATION`。`binding` は `code` が `INTERNAL` のときだけ Error、それ以外は Warn |
| `OpenLogFolder()` | — | — | ログフォルダ（`%APPDATA%\Hitomado\logs`）を OS の標準の方法で開く。Android ではフォルダを開く手段が無く `UNSUPPORTED`（メッセージで「ログを書き出す」を案内する） |
| `ExportLogs()` | — | `{ saved: boolean; path?: string }` | 保存ダイアログ（Android は保存先選択）で選んだ先に、ログ（現行とローテーション済み）を zip で書き出す。キャンセルは `saved=false`。`path` は `SaveAttachment` と同じ |

### 7.2 MCPService

内蔵 MCP サーバーの設定と状態。仕様・ツールは [MCP 連携とログ](../mcp.md)。

| メソッド | 引数 | 戻り値 | 内容 |
|----------|------|--------|------|
| `GetMCPStatus()` | — | `MCPStatus` | 設定と状態。`MCPStatus = { enabled; host; port; allowNonLoopback; hasToken; listening; endpoint; error? }`。既定は `enabled=false`・`host=127.0.0.1`・`port=47650`。トークンの値は含まない |
| `SetMCPConfig(config)` | `{ enabled; host; port; allowNonLoopback }` | `MCPStatus` | 検証して `app_settings.mcp.config` に保存し、待ち受けに反映する（オフなら止める）。ポートは 1024〜65535、loopback 以外のホストは `allowNonLoopback=true` が必要（満たさなければ `VALIDATION`・保存しない）。待ち受けに失敗（ポート使用中など）しても設定は保存し、`listening=false`・`error` で返す |
| `IssueMCPToken()` | — | `{ token: string; status: MCPStatus }` | Bearer トークンを（再）発行して資格情報ストア（`hitomado/mcp_token`）に保存する。古いトークンは無効になる。`token` を返すのはこの 1 回だけ |
| `RevokeMCPToken()` | — | `MCPStatus` | トークンを削除する（以後すべての MCP 接続を拒否） |

- Android（MCP を持たない OS）では `MCPService` をバインディングへ登録しない（`run.go`・`services_binding.go`）。`Start` も、保存済みの設定がオンでも待ち受けない（型・バインディングは OS によらず同じで、登録の有無だけが変わる）
- `Start`／`Shutdown`（起動・終了処理用）はフロントへ公開しない（`//wails:ignore`）。`SetMCPConfig` の `enabled=false` は検証せずに停止し、保存済みの host／port を維持する。起動時は保存された設定を検証してから待ち受ける
- MCP は待ち受けのみで、`Bearer` トークンの無い・合わない接続は `401`、`Host`／`Origin` が不正なら `403`。MCP のツール（読み取りと `create_reply_draft`）に送信は無い。ツールの呼び出しは `LogService` と同じログに残る（本文・引数は残さない）

### 7.3 PlatformService

実行中の OS で提供する機能（`internal/capability`）。フロントは OS 名ではなく、ここで返す機能の有無で画面を出し分ける（`frontend/src/stores/platform.ts` の `platformStore`）。バインディングは全 OS で同じ（値だけがビルドタグ `android`／`!android` で変わる）。

| メソッド | 引数 | 戻り値 | 内容 |
|----------|------|--------|------|
| `GetCapabilities()` | — | `{ os: string; supportsMcp: boolean; supportsTray: boolean; supportsAutoUpdate: boolean; supportsWindowState: boolean; singleInstance: boolean; touchUi: boolean }` | `os` は `runtime.GOOS` と同じ表記（表示・ログ用）。Windows は `touchUi` 以外が `true`、Android は `touchUi` と `supportsAutoUpdate`（アプリ内の更新確認。新しい版のお知らせとダウンロードページのリンクだけで、ダウンロード・入れ替えは `AppInfo.autoUpdateSupported` が別に決め、Android では偽）だけが `true`（MCP サーバー・トレイ・ウィンドウ状態の保存復元・多重起動の防止を持たない）。`touchUi` はタッチ操作中心の画面（幅による 1 画面ずつ・2 ペインの表示、44px のタップ領域）を使うか。`true` の OS は最小ウィンドウ幅の制限も外す |

### 7.4 NotifyService

通知の許可（Android 13 以降の `POST_NOTIFICATIONS` と OS のアプリの通知設定）と、通知のタップで開くスレッド。バインディングは全 OS で同じで、Windows では許可を管理しないため `unsupported` を返す（設定画面は項目を出さない）。

| メソッド | 引数 | 戻り値 | 内容 |
|----------|------|--------|------|
| `GetNotificationPermission()` | — | `'granted' \| 'denied' \| 'unsupported'` | 通知を出せるか。`denied` は未許可・OS の設定で通知がオフ |
| `RequestNotificationPermission()` | — | — | 許可のダイアログを出す（許可済み・永続的に拒否済みでは何も出ない）。結果は待たない。最初のアカウントの追加後は Go が自動で呼ぶ |
| `OpenNotificationSettings()` | — | — | OS の「このアプリの通知設定」を開く。開けなければ `UNSUPPORTED` |
| `TakePendingOpenThread()` | — | `{ accountId, threadId } \| null` | フロントが購読を始めた**あと**に 1 回呼ぶ。通知のタップで起動されて（コールドスタート）まだ開いていないスレッドがあれば返す。以後のタップは `notify:open-thread` で届く（フロントの準備前に発行したイベントの取りこぼしを避ける） |

## 8. イベント（Go → フロント）

Wails のイベントとして発行する。ペイロードに本文・件名・宛先などの機密を載せない。フロントは受信後に該当バインディングを再呼び出しして再描画する。

| イベント名 | ペイロード | 契機 |
|-----------|------------|------|
| `sync:updated` | `{ accountId: string; changedThreadIds: string[] }` | 差分同期・リセット同期でキャッシュが更新された |
| `account:status` | `{ accountId: string; status: 'active' \| 'syncing' \| 'needs_reauth' \| 'error'; message?: string }` | アカウント状態の変化（`syncing` は一時状態で DB に保存しない） |
| `notify:open-thread` | `{ accountId: string; threadId: string }` | 通知クリック（トレイ操作ではウィンドウを前面に出すだけで、このイベントは発行しない）。Android では、起動済みのアプリが通知のタップで前面に出たとき（フロントが準備済みなら）。起動前のタップは `TakePendingOpenThread` で受け取る |
| `update:available` | `{ version: string; url: string }` | 新バージョン検出 |
| `update:status` | `{ stage: 'none' \| 'downloading' \| 'ready' \| 'failed'; version?: string; message?: string }` | 自動アップデートの進みの変化（ダウンロードの開始・照合の完了〔`ready`〕・失敗〔`failed`。ハッシュの不一致・通信・インストーラーの失敗〕・オフにして破棄〔`none`〕）。フロントは `ready` で「再起動して更新」を出す |
| `app:before-quit` | なし | トレイの「終了」。フロントは未保存の下書きを保存して `AckBeforeQuit` を呼ぶ |
| `files-dropped` | `{ files: string[] }` | ウィンドウへのファイルドロップ（`data-file-drop-target` 属性の要素上のみ）。フロントは `AddAttachments` に渡す |

## 9. セッション・状態の寿命

| 対象 | 保持 | 破棄 |
|------|------|------|
| 作成セッション | メモリのみ | `Send` 成功／`CloseCompose`／`DiscardCompose`／アプリ終了。最終操作から 24 時間で自動破棄 |
| 確認トークン | メモリのみ | 5 分で失効／使用／`PrepareSend` の再実行／セッション破棄 |
| 本文キャッシュ（LRU） | メモリのみ（直近 50 スレッド） | `history` で変更検知／アプリ終了／アカウント削除 |
| 検索カーソル | カーソル文字列に埋め込み（サーバー状態なし） | — |
| スレッド AI の要約・質問と回答の会話 | フロントのメモリのみ（Go は保持しない。`AskAboutThread` は呼び出しごとに `history` を受け取る。書きかけの質問も同じ） | スレッドを切り替える・閉じる・一覧から除かれる／アプリ終了。「新しい会話」で会話だけ破棄 |

- アプリ終了時は、Go が `app:before-quit` を発行し、フロントが未保存の作成セッションに `SaveDraft` を呼んでから `AckBeforeQuit` を呼ぶ（F-12。Go は猶予 3 秒で打ち切る）。トレイ格納（× ボタン）ではセッションを維持する

## 10. 外部 API の利用

| 外部 API | 用途 | 主なエンドポイント／スコープ |
|----------|------|------------------------------|
| Google OAuth 2.0 | 認可・更新・revoke | `https://accounts.google.com/o/oauth2/v2/auth`、`https://oauth2.googleapis.com/token`、`https://oauth2.googleapis.com/revoke`、`https://openidconnect.googleapis.com/v1/userinfo`。スコープ: `https://www.googleapis.com/auth/gmail.modify`、`openid`、`email`、`profile` |
| Gmail API v1 | メール取得・操作・送信 | `users.getProfile`、`users.history.list`、`users.threads.list／get／modify／trash／untrash`、`users.messages.send`、`users.messages.list／get`（`Send` の応答を受け取れなかったときの送信結果の確認のみ。§5.2）、`users.messages.attachments.get`、`users.drafts.create／update／get／delete／list`、`users.labels.list／get`、`users.settings.sendAs.get`（署名の取得。`FetchGmailSignature`）（いずれも `gmail.modify` スコープ内） |
| AI プロバイダ | 下書き生成・スレッドの要約・スレッドへの質問 | Anthropic Messages API／OpenAI 互換 Chat Completions（Base URL 指定可）／ChatGPT プラン（Responses API） |
| GitHub Releases API | 更新確認 | `GET /repos/ousiassllc/hitomado/releases/latest`（未認証） |
| GitHub Release の添付ファイル | Windows 版の自動アップデート（NF-D03） | `GET https://github.com/ousiassllc/hitomado/releases/download/vX.Y.Z/hitomado-vX.Y.Z-windows-amd64-setup.exe` と `.../SHA256SUMS.txt`（未認証。`release-assets.githubusercontent.com` へ 302。HTTPS・`github.com`／`*.githubusercontent.com` のみ） |

### Gmail API の呼び出し規則

- `threads.get` は一覧同期で `format=metadata`（`metadataHeaders=From,To,Cc,Subject,Date`）、閲覧時は `format=full`
- すべての Gmail 呼び出しはアカウントごとの `rate.Limiter`（クォータユニット単位）を通す。429／5xx は指数バックオフ（初回 1 秒、上限 60 秒、ジッタ付き）。ただし二重実行になり得る `messages.send` と `drafts.create` は 5xx を再試行しない（429 は再試行する）
- `invalid_grant` は `NEEDS_REAUTH` に変換し、`accounts.status = 'needs_reauth'` を更新して `account:status` を発行する。`message`（`accounts.status_message`）は「再接続が必要です（Google がトークンを無効にしました）」にし、画面が原因の案内（OAuth 同意画面が「テスト」だと 7 日で失効する、など）を足せるようにする（同期の経路と利用者の操作の経路で同じ文言。それ以外の `NEEDS_REAUTH` は従来の固定文言）
- 要再接続（`needs_reauth`）に切り替えるたびに、ログへ Warn「account needs reauth」を 1 件残す。属性は `account`（アカウント ID。メールアドレス・トークンは載せない）、`reason`（`invalid_grant`／`client_rejected`／`api_401`／`scope_insufficient`／`secret_not_found`／`secret_key_lost`。分からなければ空）、`trigger`（`sync`＝同期周期、`action`＝利用者の操作）、`prev_status`、`since_last_sync_s`（直近に同期できてからの秒数。記録があるとき）、`err`、`origin`（`app`＝アプリ本体、`headless`＝Android のバックグラウンド同期。[Android ビルド §21](../environment/android.md#21-再接続待ち再ログインが頻繁に起きる原因369)）。読み方は同節
- `invalid_client`／`unauthorized_client`／トークン更新の 401 は `OAUTH_CLIENT_REJECTED` に変換する。同期では `needs_reauth` と同様に自動再試行を止め、`message` に「OAuth クライアントの設定（Client ID／Secret）を確認してください」を載せる（設定を直したあと `Reconnect` で再開する）
- ネットワーク不通は `OFFLINE` に変換する。取得済みのキャッシュ表示は維持する
- Gmail API がエラー応答を返したときは、Google の応答（`error.status`、`error.message`、`error.errors[].reason`、`error.details[].reason`、`error.details[].metadata` のうち `service`／`consumer`／`containerInfo`／`activationUrl`）を解析し、reason をエラー文言とログ（「Gmail API がエラーを返しました」。`op`／`status`／`api_status`／`reasons`／`api_message`／`api_metadata`）に含める。すべての Gmail 呼び出しが共通のエラー処理を通るので、`users.getProfile` 以外も同じに扱う。同期の周期失敗のログ（`sync cycle failed`）にも reason が入る。`message` は先頭 4096 バイトだけを使い、制御文字（改行を含む）を空白に置き換えてトークン・メールアドレスを伏せ（`logging.Scrub`）、300 文字に切り詰める。reason・`error.status` は英数字と `_.-` のみ・64 文字以内のものだけを載せ、reason は 8 件まで。本文が 64KiB を超える応答は JSON として解析せず、`googleapi.Error` が解析済みの `errors[].reason` だけを使う。JSON でない応答（プロキシの HTML など）は先頭の一部だけを使う。利用者に表示する `AppError.message` には Google の `message` 本文を載せない（HTTP ステータスと reason だけ）
- 403 で reason が `SERVICE_DISABLED`／`accessNotConfigured` のときは `GMAIL_ERROR`（同期では `error` 状態でメッセージ「Google Cloud のプロジェクトで Gmail API が有効になっていません。有効にしてください」。有効にすれば次の周期で回復する）、`ACCESS_TOKEN_SCOPE_INSUFFICIENT`／`insufficientPermissions` のときは `NEEDS_REAUTH`（同期では `needs_reauth` で「Gmail の権限（スコープ）が足りません。再接続して、すべての権限を許可してください」。再試行しない）に変換する。401 は従来どおり 1 回だけトークンを取り直して再試行し、それでも 401 なら `NEEDS_REAUTH`

## 改訂履歴

| 版 | 日付 | 変更内容 | 変更理由 |
|----|------|---------|---------|
| 1.0 | 2026-11-02 | 新規作成 | 初版 |
| 1.1 | 2026-11-02 | `AckBeforeQuit`、イベント `app:before-quit`／`files-dropped` を追加 | トレイ終了時の下書き保存（F-12）とファイルドロップのパス取得（Wails はパスを Go 側でのみ受け取る）のため |
| 1.2 | 2026-11-03 | §1.2 のエラー受け渡し方式を `MarshalError`（`apperr.Marshal`）／`error.cause`／`INTERNAL` フォールバックに確定。未決の記述を削除 | 実装との整合（#31） |
| 1.3 | 2026-11-03 | §8 `notify:open-thread` の契機を通知クリックのみに修正 | 実装（通知の応答のみで発行）・アーキテクチャ設計 §5 との整合（#30） |
| 1.4 | 2026-11-03 | §10 の Gmail API 一覧に `users.getProfile`・`users.drafts.get` を追加 | `internal/gmail` の呼び出しとの整合（#33） |
| 1.5 | 2026-11-03 | `SuggestContacts` の一致対象にメールアドレスと表示名の前方一致を明記 | 実装（`SuggestContacts` クエリ）との整合（#32） |
| 1.6 | 2026-11-03 | §5.2 に送信失敗時の動作を、§10 に `messages.send`／`drafts.create` が 5xx を再試行しない例外を追記 | `internal/gmail/call.go` との整合（#46） |
| 1.7 | 2026-11-03 | §1.2 に `SEND_UNCERTAIN`・`OAUTH_CLIENT_REJECTED` を、§2 に `UpdateOAuthClient`／`DeleteOAuthClient` の失敗時の挙動を、§4 に `ThreadDetail.draftId` を DRAFT ラベルがあるときだけ探す注記を、§5.2 に送信の転送へ総時間上限を掛けない記述を追記（履歴に未記録だった分） | 実装との整合（#37、#43、#45、#42。記録漏れの補正 #53） |
| 1.8 | 2026-11-03 | §1.2 の `AI_FAILED` と §6 に、AI プロバイダの 3xx を追従せず失敗とする挙動を追記 | 実装（`internal/ai/provider.go` の既定クライアント）との整合（#58） |
| 1.9 | 2026-11-03 | §1.2・§5.2 の `SEND_UNCERTAIN` を、送信後に応答を受け取れず送信済みを確認できない場合に限定（接続前の失敗は `OFFLINE`）。送信済みの確認を独自ヘッダ `X-Hitomado-Send-Id` と `in:sent` 検索に変更し、確認できなければ `SEND_UNCERTAIN` とした。MIME ヘッダに `X-Hitomado-Send-Id` を追加 | Gmail API が `Message-ID` を付け替えるため `rfc822msgid:` 検索では送信済みを確認できないこと、接続前の失敗まで `SEND_UNCERTAIN` になる不具合の修正（#52、#54） |
| 1.10 | 2026-11-03 | §10 の Gmail API 一覧に `users.messages.list／get`（`Send` の応答を受け取れなかったときの送信結果の確認のみ）を追加 | `internal/gmail` の呼び出しとの整合（#51） |
| 1.11 | 2026-11-03 | §1.2・§5.2 の `OFFLINE`／`CANCELLED`／`SEND_UNCERTAIN` の振り分けを実装に合わせて修正。送信後の呼び出し側キャンセルも送信済みの確認の対象とし、送信前の失敗は期限切れが `OFFLINE`、キャンセルが `CANCELLED` と明記 | `internal/gmail/errors.go`・`call.go`・`send_confirm.go`・`apperr` との整合（#66） |
| 1.12 | 2026-11-03 | `LogService`（§7.1）・`MCPService`（§7.2）を追加 | #14 |
| 1.13 | 2026-11-03 | `CheckForUpdate` に、版が `0.0.0`（dev・ローカルビルド）のときは通信せず `{ available: false }` を返す旨を追記 | #21 |
| 1.14 | 2026-11-03 | `RemoveAccount` の手順を、同期ワーカーの停止（実行中の周期の終了待ち）→DB 削除に改め、DB 削除に失敗したときは同期を再開してエラーを返す旨を追記 | 実装（`internal/app/account.go`）との整合（#4） |
| 1.15 | 2026-11-03 | `ReportClientError` の `message`・`stack` を、例外のメッセージ文ではなく種類のコード・位置だけのフレームにしたことを追記 | 実装（`frontend/src/lib/errorReporter/index.ts`）との整合（#24） |
| 1.16 | 2026-11-03 | `AIProvider` に `chatgpt` を追加し、`AIConfig` に `chatgptSignedIn`／`chatgptEmail` を追加。`SignInChatGPT`／`CancelSignInChatGPT`／`SignOutChatGPT`／`ListAIModels` と、§6.1 モデル一覧・§6.2 ChatGPT プランを追記。§1.2 の `AI_NOT_CONFIGURED`／`AI_FAILED` の説明を拡張 | Sign in with ChatGPT（#75）とモデル一覧からの選択（#76） |
| 1.17 | 2026-11-03 | `PlatformService`（§7.3）と `UNSUPPORTED`（§1.2）を追加。§7 の `SetUpdateCheckEnabled` と §7.2 に、Android での扱いを追記 | Android 版で使わない機能の無効化（#89） |
| 1.18 | 2026-11-03 | `SaveAttachment`・`ExportLogs` の保存先に Android（Storage Access Framework）の動作を、`OpenLogFolder` に Android で `UNSUPPORTED` となる旨を追記 | #85 |
| 1.19 | 2026-11-03 | `AccountService.TakeInterruptedAuth` を追加 | Android の Google OAuth（#86） |
| 1.20 | 2026-11-03 | `GetCapabilities` の戻り値に `touchUi` を追加 | Android のスマホ向け画面（#87） |
| 1.21 | 2026-11-03 | `NotifyService`（§7.4。通知の許可・OS の通知設定・通知のタップで開くスレッド）を追加。`notify:open-thread` の契機に Android の通知のタップを追記 | Android のバックグラウンド同期と通知（#88） |
| 1.22 | 2026-11-04 | `ChatGPTAccount`・`AIConfig.chatgptAccounts` を追加し、`SignInChatGPT`／`SignOutChatGPT` の引数を `differentAccount`（真偽値）／なしから `accountId` に変え、`SelectChatGPTAccount`／`RemoveChatGPTAccount` を追加。§6.2 に複数の ChatGPT アカウント（登録・トークンをアカウントごとに保存、選んで使う、追加は検証までほかに影響しない、ログアウトは選んだアカウントだけ）と旧形式からの移行を追記 | 複数の ChatGPT アカウント（#79） |
| 1.23 | 2026-11-04 | `ChatGPTAccount.planNoticePending` と `AckChatGPTPlanNotice` を追加し、§6.2 に初めてのログインの確認を追記。利用枠の超過の `AI_FAILED` に `details.reason = "usage_limit_exceeded"` を付け、画面が「Manage usage」を主なアクションにした案内を出すことを明記（§1.2・§6.2） | OpenAI の UI/UX ガイドラインへの対応（#80） |
| 1.24 | 2026-11-04 | §6.2 の ChatGPT の複数アカウントに、新規登録の `client_id` が保存済みの登録と衝突したとき `sub` が未記録で照合できない場合も拒否する旨を追記。更新・ログアウトの直列化をアカウントごとにしたことを追記 | sub を照合できない登録を別アカウントの新規ログインで上書きしないため（#100）。アカウントごとのロック（#93） |
| 1.25 | 2026-11-04 | `InboxQuery.tab`（すべて／メイン／ソーシャル／プロモーション／新着）を追加し、`ListInbox` に絞り込みとページングの扱い（ローカルのキャッシュだけに掛ける理由、カーソルはタブごとに独立）を追記 | 受信トレイのカテゴリのタブと新着タブ（#97） |
| 1.26 | 2026-11-04 | §1.2 の `GMAIL_ERROR`／`NEEDS_REAUTH` に Gmail API の reason による振り分け（API 無効・権限不足・401）と `details.reason`（`gmail_api_disabled`／`scope_insufficient`）を追記し、Google のエラー応答の解析・ログ出力・伏せ字処理の方針を追記 | #96 |
| 1.27 | 2026-11-04 | §6.2 の ChatGPT の複数アカウントに、`client_id` を大文字小文字を無視して一意として扱い、大文字小文字だけが違う登録・再ログインを拒否する旨を追記 | Windows 資格情報マネージャーがキーの大文字小文字を区別せず、別アカウントの資格情報がぶつかるため（#101） |
| 1.28 | 2026-11-04 | §1.2 の `OAUTH_CANCELLED`／`OAUTH_DENIED`／`OAUTH_TIMEOUT` の発生元に `SignInChatGPT` を追記（`OAUTH_DENIED` は ChatGPT プランの利用が許可されなかった場合も） | #75 から続いていた発生元の乖離（#94） |
| 1.29 | 2026-11-04 | `SelectChatGPTAccount` が、ログアウト済みのアカウントで `AI_NOT_CONFIGURED` に `details.reason = "account_signed_out"` と「再ログイン」を案内するメッセージを返す旨を、§6 の表と §6.2 の複数アカウントの記述に追記 | 何をすればよいか分からない文面だったため（#102） |
| 1.30 | 2026-11-04 | `AppService` に `SetAutoUpdateEnabled`・`GetUpdateStatus`・`RestartToUpdate`、`GetAppInfo` に `autoUpdateSupported`／`autoUpdateEnabled`、イベント `update:status`、外部 API に Release の添付ファイルを追加。`CheckForUpdate` に自動ダウンロードの開始を追記 | #103 |
| 1.31 | 2026-11-04 | `AIConfig` に `configured`（未保存のとき偽。既定のプロバイダは `chatgpt`）と `apiKeyProviders` を追加。`RecordAIConsent` と `ListAIModels` に `baseUrl` を足し、保存済みの設定ではなく渡された送信先に対して同意の保存・一覧の取得をするように変更（保存前でも取得できる。同意前は `consent_required`）。§6.1 の取得先と呼ぶ契機を更新 | #104・#105 |
| 1.32 | 2026-11-05 | `GenerateReplyDraft` に `instruction?`（AI への指示。2,000 文字まで・保存しない）を追加し、§6 に指示の扱いを追記 | #109 |
| 1.33 | 2026-11-05 | `AccountService.FetchGmailSignature`（Gmail の署名を取得して返す。保存しない）を追加し、§10 の Gmail API 一覧に `users.settings.sendAs.get` を追加 | #110 |
| 1.34 | 2026-11-05 | `AIConfig.effort`・`AIModel.efforts` を追加し、`SetAIConfig` に `effort?` を足した。§6 にプロバイダごとの渡し方（`output_config.effort`／`reasoning_effort`／`reasoning.effort`）、§6.1 に段階の取得（Anthropic は `capabilities.effort`。OpenAI 互換・ChatGPT は共通の 3 段階）を追記 | #111 |
| 1.35 | 2026-11-05 | `SendPreview.bodyPreview` を、改行を保ち署名・引用を除いた約 600 文字（従来は空白を畳んだ約 200 文字）に変更 | #115 |
| 1.36 | 2026-11-05 | `InboxService` に `Unarchive`・`Untrash` を追加し、§10 の Gmail API 一覧に `users.threads.untrash` を追記（アーカイブ・ゴミ箱のスナックバー「元に戻す」） | #156 |
| 1.37 | 2026-11-06 | §5.2.1（送信済み確認の実 Gmail での確認）・§6.2.1（ChatGPT ログインの実機確認）の結果欄を追加（いずれも未確認として記録） | #72・#78 |
| 1.38 | 2026-11-06 | `AIService` に `SummarizeThread`（スレッドの AI 要約）と型 `ThreadAIResult` を追加。「スレッド AI の入力」（全メッセージ・合計 40,000 文字・下書きと添付の中身を送らない）、プロンプトでの本文の扱い、保存しないこと、要約の寿命（§9）、`CancelGenerate` の対象を追記 | #206 |
| 1.39 | 2026-11-06 | `AIService` に `AskAboutThread`（スレッドの内容を文脈にした AI への質問）と型 `ThreadAITurn` を追加。`history`・`question` の検証（`VALIDATION`）、プロンプトの構成（スレッド本文は信頼しない先頭の user メッセージ・要約は文脈に入れない）、分からないことは失敗にしないこと、会話の寿命（§9）、`CancelGenerate` の対象を追記 | #207 |
| 1.40 | 2026-11-07 | Android でも更新確認（`CheckForUpdate`・`SetUpdateCheckEnabled`・`update:available`）を有効にした。`GetCapabilities` の Android は `supportsAutoUpdate` が `true`。新しい版のバナー・設定の項目はダウンロードページのリンクを出すだけで、ダウンロード・入れ替え（`SetAutoUpdateEnabled` など）は Android では `UNSUPPORTED` のまま | Android の新バージョンのお知らせ |
| 1.41 | 2026-11-10 | `AppService` に `SetSendConfirmEnabled`、`GetAppInfo` に `sendConfirmEnabled` を追加（送信前の確認画面のオン・オフ。既定オン）。§5.2 に、オフのときの送信の流れを追記 | #313 |
| 1.42 | 2026-11-10 | `SetAIApiKey`／`ClearAIApiKey` が `typesafe`（TypeSafe AI（Jev））を受け付け、`AIConfig.apiKeyProviders` に含むようにした（下書きのプロバイダではない。#314） | #314 |
| 1.43 | 2026-11-11 | `AIService` に Inbox Zero（`GetInboxZeroSettings`・`SetInboxZeroReasons`・`FindInboxZeroCandidates`）と型 `InboxZeroReasons`／`InboxZeroSettings`／`InboxZeroCandidate`／`InboxZeroResult` を追加し、§6.3 に判定の規則（理由 4 つ・Jev は広告／通知／ニュースレターのみ・確率 0.8 以上・送る内容・上限 100 件・並行 4・エラー）を追記。`RecordAIConsent` が `typesafe` を受け付け、`AIConfig.consentedProviders` に含むようにした（`SetAIConfig` で消えない）。`CancelGenerate` の対象に `FindInboxZeroCandidates` を追加 | #314 |
| 1.44 | 2026-11-13 | §1.2 の `AI_FAILED` の `details.reason` を、`usage_limit_exceeded` と `effort_rejected`（エフォートの拒否。`message` が設定で既定に戻す案内）の 2 つに直し、`ATTACHMENT_IO` に `details.filename` を追記。§6 の `effort` の渡し方に `details.reason = "effort_rejected"` を追記 | 実装（`internal/app/aicore/generate.go:136-137`・`apperr.go:59`・`composecore/compose_attach.go:25-28`）との整合（#325・#335） |
| 1.45 | 2026-11-14 | `CancelGenerate` が、生成の登録より前（前提の検査・ChatGPT のトークン更新の最中、または呼び出しより先に届いた場合）でも効くことを追記 | #352 |
| 1.46 | 2026-11-14 | `invalid_grant` による `NEEDS_REAUTH` の `status_message` を専用の文言にし（画面が原因の案内を足す）、再接続待ちに切り替えるときのログ「account needs reauth」（`reason`・`trigger`・`origin` など）を §10 に追記 | #369 |
