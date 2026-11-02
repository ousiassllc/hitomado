# Hitomado

複数の Gmail／Google Workspace アドレスを 1 つの統合受信箱で扱い、**受信したアカウントから正しい送信元で返信できる**デスクトップアプリ（Windows）。アカウント切替と送信元の取り違えをなくすことが目的です。

- 公開者のサーバーは持ちません。通信は利用者の PC と Google／AI プロバイダ（任意）／GitHub Releases（更新確認）の間だけで完結します。
- Gmail の制限付きスコープの審査（CASA）を避けるため、**利用者が自分の Google Cloud で OAuth クライアントを作る**方式（BYO OAuth）です。手順は [OAuth クライアントの作り方](#oauth-クライアントの作り方) を参照してください。
- ライセンス: [AGPL-3.0](LICENSE)

## 主な機能

- 複数アカウントの統合受信箱（アカウント色・名前チップ・未読・添付アイコン、アカウントフィルタ、未読のみ）
- スレッド閲覧（HTML 本文はサニタイズして `sandbox` iframe で表示。リモート画像は既定でブロック）
- 既読／未読・アーカイブ・ゴミ箱（完全削除はしません）
- 返信・全員に返信・転送・送信してアーカイブ。**返信・転送の送信元は受信アカウントに固定**され、送信前に必ず送信元を確認するダイアログが出ます
- 新規作成（送信元を選択）、アカウント別署名、添付（合計 25MB まで）、宛先オートコンプリート、Gmail 下書き同期
- Gmail 検索クエリを全アカウント横断で実行
- 新着のデスクトップ通知とトレイ常駐
- AI で返信を下書き（任意。ボタンを押したときだけ生成し、送信は必ず人が行います）
- ライト／ダークテーマ、Gmail 互換のキーボードショートカット

## インストール

1. [Releases](https://github.com/ousiassllc/hitomado/releases) から最新の `hitomado-vX.Y.Z-windows-amd64.zip` をダウンロードして展開します（同じ Release の `SHA256SUMS.txt` でハッシュを確認できます）。
2. `hitomado.exe` を実行します。インストーラーはありません。
3. 初回は [OAuth クライアントを作成](#oauth-クライアントの作り方)し、アプリの設定 → OAuth クライアントに登録してから、アカウントを追加します。

動作環境: Windows 10（1809 以降）／Windows 11（x64）。WebView2 ランタイムが必要です（Windows 11 は標準搭載。未導入なら [Microsoft の配布ページ](https://developer.microsoft.com/microsoft-edge/webview2/) から入れてください）。

### SmartScreen の警告について

このアプリは**コード署名をしていません**。初回の実行時に「Windows によって PC が保護されました」という SmartScreen の警告が出ることがあります。

1. 警告画面の **「詳細情報」** をクリックします。
2. 表示された **「実行」** をクリックします。

ダウンロード元が [公式の Releases](https://github.com/ousiassllc/hitomado/releases) であることを確認してから実行してください。

## OAuth クライアントの作り方

Hitomado は Gmail API を使うため、Google Cloud で OAuth クライアント（種類: **デスクトップ アプリ**）を作成し、その Client ID と Client Secret を設定画面に登録します。アカウント（会社）ごとに別のクライアントを使うこともできます。

1. [Google Cloud Console](https://console.cloud.google.com/) でプロジェクトを作成（または既存のものを選択）します。
2. **API とサービス → ライブラリ** で **Gmail API** を有効にします。
3. **API とサービス → OAuth 同意画面** を設定します。
   - ユーザーの種類: 個人の Gmail や他社のアカウントも使うなら **外部**、同じ Workspace 組織のアカウントだけなら **内部**。
   - アプリ名・サポートメール・連絡先メールを入力します。
   - スコープに `https://www.googleapis.com/auth/gmail.modify` を追加します（`openid`・`email`・`profile` は自動で含まれます）。
4. **認証情報 → 認証情報を作成 → OAuth クライアント ID** を選び、アプリケーションの種類に **デスクトップ アプリ** を指定して作成します。
5. 表示された **クライアント ID** と **クライアント シークレット** を、Hitomado の設定 → OAuth クライアントに名前を付けて登録します。
6. 設定 → アカウントの追加で、登録した OAuth クライアントを選び、ブラウザで Google の認可を完了します。

### 重要: 「テスト」のままだと 7 日でトークンが失効します

OAuth 同意画面の公開ステータスが **外部かつ「テスト」** のままだと、**リフレッシュトークンが 7 日で失効**し、そのたびにアカウントが「要再接続」になります。次のいずれかで回避してください。

- **「本番環境」に切り替える**（OAuth 同意画面 → アプリを公開）。審査を受けない「未確認のアプリ」のまま使えます。100 ユーザーの上限は個人利用では問題になりません。
- **Workspace の「内部」アプリ**にする（その組織のアカウントだけで使う場合）。ただし他社のアカウントには使えないため、会社ごとに別の OAuth クライアントを作って登録します。

### 「このアプリは Google で確認されていません」と出たら

本番環境で未確認のアプリを使うと、認可時に警告が出ます。自分で作ったクライアントなので、**「詳細」→「（アプリ名）に移動（安全でない）」** を選んで続行してください。

### Workspace の管理者がアクセスを制限している場合

組織の管理者が外部アプリのアクセスを制限していると認可できません。管理者に、Google Workspace 管理コンソールの **セキュリティ → API の制御** で、この OAuth クライアントを許可してもらってください。

## データの保存とセキュリティ

- リフレッシュトークン・Client Secret・AI の API キーは **Windows 資格情報マネージャー**に保存します。ファイル・ログに平文では残りません。
- ローカル DB（`%APPDATA%\Hitomado\hitomado.db`）には、**メール本文・添付は保存しません**。保存するのは差出人・件名・日時・スニペット・ラベル・同期状態・宛先候補のみです。件名とスニペットは暗号化されないため、**BitLocker などのディスク暗号化の利用を推奨**します。
- 保存した添付ファイルには Windows の Mark-of-the-Web（`Zone.Identifier` の `ZoneId=3`）を付けます。開くときに SmartScreen／保護ビューが働きます（FAT/exFAT など代替データストリーム非対応のボリュームでは付かず、保存自体は成功します）。
- ログ（`%APPDATA%\Hitomado\logs\`）にトークン・本文・件名・宛先は出力しません。
- アプリが行う外部通信先は次の 4 種類だけです。公開者のサーバーへは一切通信しません。テレメトリもありません。
  - Google（`accounts.google.com`／`oauth2.googleapis.com`／`openidconnect.googleapis.com`／`gmail.googleapis.com`）
  - AI プロバイダ（設定したもの。「AI で下書き」を押したときだけ、スレッド本文の一部・表示名・署名を送信）
  - GitHub Releases（`api.github.com`。新バージョンの確認のみ。設定でオフにできます）
- 例外: メールのリモート画像は既定でブロックします。メッセージごとに「画像を表示」を押したときだけ、そのメールが参照する画像を WebView が直接取得します（取得先はメールの送信者が指定したホストです）。

## キーボードショートカット

入力欄にフォーカスがあるとき、およびダイアログ表示中は無効です。

| キー | 動作 |
|------|------|
| `j` / `k` | 次／前のスレッド |
| `Enter` / `o` | スレッドを開く |
| `e` | アーカイブ |
| `#` | ゴミ箱へ移動 |
| `Shift+i` / `Shift+u` | 既読／未読にする |
| `r` / `a` / `f` | 返信／全員に返信／転送 |
| `c` | 新規作成 |
| `/` | 検索 |
| `g` → `i` | 受信箱へ |
| `Esc` | 閉じる／検索をクリア |
| `Ctrl+Enter` | エディタ内で送信（確認ダイアログを開く） |

## 開発

必要なもの: Go 1.26 系、[bun](https://bun.sh)（`npm`／`yarn`／`pnpm` は使いません）、`wails3`（v3.1.0）、`sqlc`、`golangci-lint`、`task`、`lefthook`。設定ファイルと各ツールの方針は [docs/environment/setup.md](docs/environment/setup.md) にあります。

```bash
bun install --cwd frontend      # フロントの依存
go mod download                 # Go の依存
lefthook install                # Git Hooks
task generate                   # sqlc とバインディングを生成
task dev                        # 開発サーバー（wails3 dev）
task check                      # CI と同等の検査（生成物の差分なし＋lint＋test）
```

- 動作保証の対象は Windows です。Linux／macOS では Go の単体テスト・フロントのテスト・lint を実行できますが、アプリ本体の実行確認は Windows で行ってください。Linux で Go をビルド・テストするときは GTK を使わない `CGO_ENABLED=0 go test -tags purego ./...` を使います。
- 仕様書は [docs/overview.md](docs/overview.md) のドキュメントマップから辿れます。
- マージは squash／rebase せず、マージコミット方式です。コミットには `git commit -s`（Signed-off-by）を付けます。
- リリースは `v*` タグの push で GitHub Actions が Windows 向けの成果物をビルドして Releases に添付します（コード署名なし）。
