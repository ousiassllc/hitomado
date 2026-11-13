# Hitomado

複数の Gmail／Google Workspace アドレスを 1 つの統合受信箱で扱い、**受信したアカウントから正しい送信元で返信できる**デスクトップアプリ（Windows。Android・macOS は試験段階の検証用配布）。アカウント切替と送信元の取り違えをなくすことが目的です。

- 公開者のサーバーは持ちません。通信は利用者の PC と Google／AI プロバイダ（任意）／GitHub Releases（更新確認）の間だけで完結します。
- Gmail の制限付きスコープの審査（CASA）を避けるため、**利用者が自分の Google Cloud で OAuth クライアントを作る**方式（BYO OAuth）です。手順は [OAuth クライアントの作り方](#oauth-クライアントの作り方) を参照してください。
- ライセンス: [AGPL-3.0](LICENSE)。依存ライブラリのライセンス表記（MPL-2.0 の依存のソース入手先を含む）は [NOTICE](NOTICE)（Release にも添付。インストール先には `NOTICE.txt` として入ります）

## 主な機能

- 複数アカウントの統合受信箱（アカウント色・名前チップ・未読・添付アイコン、アカウントフィルタ、未読のみ）
- スレッド閲覧（HTML 本文はサニタイズして `sandbox` iframe で表示。リモート画像は既定でブロック）
- 既読／未読・アーカイブ・ゴミ箱（完全削除はしません）
- 返信・全員に返信・転送・送信してアーカイブ。**返信・転送の送信元は受信アカウントに固定**され、送信前に送信元を確認するダイアログが出ます（設定で省けますが、警告があるときは常に出ます）
- 新規作成（送信元を選択）、アカウント別署名、添付（合計 25MB まで）、宛先オートコンプリート、Gmail 下書き同期
- Gmail 検索クエリを全アカウント横断で実行
- 新着のデスクトップ通知とトレイ常駐
- Inbox Zero（任意。ボタンを押したときだけ、TypeSafe AI で広告・通知・ニュースレターのメールを選び、確認の一覧でチェックしたものだけをまとめてアーカイブ。返信済みのメールの検出は端末内で行います）
- AI で返信を下書き（任意。ボタンを押したときだけ生成し、送信は必ず人が行います）。Anthropic・OpenAI 互換の API キーのほか、**ChatGPT のプラン（Sign in with ChatGPT）**でもログインして使えます。モデルは API キーの登録後に取得した一覧から選べます
- MCP でエージェントとつなげる（Windows 版のみ。既定はオフ・トークン必須。状態・ログ・メールの読み取りと返信の下書き作成まで。**送信はできません**）。詳しくは [docs/mcp.md](docs/mcp.md)
- エラーとログの記録（Go とフロントの両方。トークン・本文・件名・宛先は残しません）
- ライト／ダークテーマ、Gmail 互換のキーボードショートカット

## インストール

1. [Releases](https://github.com/ousiassllc/hitomado/releases) から最新の `hitomado-vX.Y.Z-windows-amd64-setup.exe` をダウンロードします。同じ Release の `SHA256SUMS.txt` でハッシュを確認できます（PowerShell: `Get-FileHash .\hitomado-vX.Y.Z-windows-amd64-setup.exe -Algorithm SHA256`）。
2. ダウンロードしたインストーラーを実行します。**管理者権限は要りません**。利用者ごとに `%LOCALAPPDATA%\Programs\Hitomado` へ入ります（インストール先は画面で変えられます）。初回は [SmartScreen の警告](#smartscreen-の警告について)が出ることがあります。
   - スタートメニューに「Hitomado」が登録されます。デスクトップのショートカットは、インストール時に選んだときだけ作られます。
   - WebView2 ランタイムが無い PC では、インストーラーが一緒に入れます。
3. スタートメニューの「Hitomado」から起動します。初回は [OAuth クライアントを作成](#oauth-クライアントの作り方)し、アプリの設定 → OAuth クライアントに登録してから、アカウントを追加します。

動作環境: Windows 10（1809 以降）／Windows 11（x64。ARM64 の Windows 11 では x64 エミュレーションで動きます。ARM64 の Windows 10 は対象外）。WebView2 ランタイムが必要です（Windows 11 は標準搭載。未導入ならインストーラーが入れます）。

Android と macOS の試験版（検証用。警告が出る前提）は、[Android 版](#android-版試験段階検証用)・[macOS 版](#macos-版試験段階検証用)を参照してください。

### 更新

- アプリは起動時と 24 時間ごとに新しい版を確認し、あれば設定画面などに表示します（設定でオフにできます）。Windows 版は、新しい版を自動でダウンロードして `SHA256SUMS.txt` と照合し、「再起動して更新」を押したとき、または次にアプリを終了するときに入れ替えます（送信中・下書きの保存中は入れ替えません。設定の「自動アップデート」でオフにでき、オフならリンクを出すだけです）。ハッシュが合わないファイルは使わずに捨てます。
- 更新するには、新しい版のインストーラーをそのまま実行します（**上書きインストール**）。前回の場所に入り、データ・アカウント・設定はそのまま残ります。Hitomado が起動中なら、タスクトレイのアイコンから「終了」してから実行してください（起動中はインストーラーが先へ進めません）。
- 版番号は、インストーラー・アプリの設定画面・「アプリと機能」・`hitomado.exe` のプロパティで同じ値です。

### アンインストール

「設定 → アプリ → インストールされているアプリ」（Windows 10 は「アプリと機能」）の「Hitomado」からアンインストールできます。Hitomado を終了してから実行してください。

アンインストールで消えるのは、プログラム本体・ショートカット・登録情報だけです。**利用者のデータは消しません**。再インストールすればアカウントを登録し直さずに使えます。

| 残るもの | 場所 | 内容 |
|----------|------|------|
| DB・設定・ログ | `%APPDATA%\Hitomado\` | メタデータのキャッシュ、アカウント・設定、ログ |
| 資格情報 | Windows 資格情報マネージャー（「Windows 資格情報」の `Hitomado:` で始まる項目） | リフレッシュトークン、OAuth クライアントシークレット、AI の API キー、ChatGPT ログインのトークン |

完全に消したいときは、先にアプリの設定でアカウントを削除し（Google 側のトークンも失効します）、アンインストール後に `%APPDATA%\Hitomado\` を削除し、資格情報マネージャーに `Hitomado:` で始まる項目が残っていれば削除してください。

### SmartScreen の警告について

このアプリ（インストーラーも）は**コード署名をしていません**。インストーラーの初回実行時（とダウンロード直後）に「Windows によって PC が保護されました」という SmartScreen の警告が出ることがあります。

1. 警告画面の **「詳細情報」** をクリックします。
2. 表示された **「実行」** をクリックします。

ダウンロード元が [公式の Releases](https://github.com/ousiassllc/hitomado/releases) であることと、`SHA256SUMS.txt` のハッシュが一致することを確認してから実行してください。

#### スマート アプリ コントロールが有効な Windows 11 の場合

Windows 11 の**スマート アプリ コントロール（Smart App Control）が「オン」**の PC では、署名の無いインストーラーが、上の「詳細情報」→「実行」の手順が出ないまま**ブロックされて実行できない**ことがあります（v0.1.32 のインストーラーで報告があります）。現在の Microsoft の説明によると、スマート アプリ コントロールは、クラウドの判定でアプリの安全性を確信できないとき、署名が無い（または無効な）アプリを「信頼できない」としてブロックします。**アプリ 1 つだけを許可する方法は無く**、Microsoft が示す対処は、スマート アプリ コントロールをオフにするか、開発元に署名を求めることです。このアプリは署名をしない方針なので、ブロックされた場合の選択肢は次のとおりです。

- **スマート アプリ コントロールをオフにして、インストールする**: Windows セキュリティ → **アプリとブラウザー コントロール** → **スマート アプリ コントロールの設定**で「オフ」にします。署名の無い他のアプリへの保護も外れます。オフにしたあとで再びオンにできるかは、Microsoft の説明がページや Windows の更新で食い違っています（「アプリとブラウザー コントロール」のページは新規インストールの Windows 11 だけで使え、リセットか再インストールが要るとし、FAQ は最近の更新で再インストールなしにオン／オフを切り替えられるとしています）。**お使いの Windows で再びオンにできるかを、オフにする前に Microsoft の最新の説明で確認してください**。
- **スマート アプリ コントロールをオフにしない場合は、このアプリを使えません**。

スマート アプリ コントロールが「評価モード」のあいだは、何もブロックされません。Windows 10 にはこの機能はありません。詳しくは Microsoft の公式ドキュメントを参照してください。

- [スマート アプリ コントロールの FAQ（Smart App Control Frequently Asked Questions）](https://support.microsoft.com/en-us/windows/security/threat-malware-protection/smart-app-control-frequently-asked-questions)
- [Windows セキュリティ アプリの「アプリとブラウザー コントロール」（App & browser control in the Windows Security App）](https://support.microsoft.com/en-us/windows/security/windows-security/app-browser-control-in-the-windows-security-app)

## Android 版（試験段階・検証用）

Releases の `hitomado-vX.Y.Z-android-arm64.apk`（arm64 の実機向け。`SHA256SUMS.txt` でハッシュを確認できます）を Android 12（API 31）以上の端末へ入れて試せます。Google Play では配布していません。OAuth クライアントは [Android で使う場合の OAuth クライアント](#android-で使う場合の-oauth-クライアント)を参照してください。

1. 端末の設定で、APK を開くアプリ（ブラウザ・ファイル）に「不明なアプリのインストール」を許可し、APK をインストールします。Play プロテクトの警告が出ることがあります。
2. 起動して、Windows 版と同じように OAuth クライアントの登録・アカウントの追加をします。

**警告が出る前提です（意図した状態です）。** Google Play では配布していないので、Play プロテクトの警告が出ることがあります。この APK は、リポジトリの管理者が保管する**固定の署名鍵**で署名していて、どの版も署名証明書が同じです。そのため:

- **新しい版は、入っている版の上に上書きインストールできます**。アカウント・設定・下書きは残ります。新しい APK を開いて「更新」を選ぶか、PC に接続した端末なら `adb install -r hitomado-vX.Y.Z-android-arm64.apk` を実行します。
- **固定の鍵に切り替えた最初の版だけは、上書きできません。** それより前の APK は、版ごとに作る使い捨ての鍵で署名していたため、入れようとすると「アプリがインストールされていません」「競合するパッケージ」などで失敗します。この切り替えのときだけ、いったんアンインストールして入れ直します。**アプリのデータ（アカウント・設定・下書き）は消えます**。PC に接続した端末なら `adb uninstall jp.ousias.hitomado` の後に `adb install hitomado-vX.Y.Z-android-arm64.apk`、端末だけなら「設定 → アプリ → Hitomado → アンインストール」の後に新しい APK を入れます。次の版からは上書きで更新できます。
- アプリ内で新しい版を確認してリンクを出しますが、自動では入れ替えません（新しい APK は Releases から入れてください）。
- エミュレーター（x86_64）向けの APK は添付していません。

## macOS 版（試験段階・検証用）

Releases の `hitomado-vX.Y.Z-macos-arm64.zip`（Apple Silicon）または `hitomado-vX.Y.Z-macos-amd64.zip`（Intel）をダウンロードし、`SHA256SUMS.txt` でハッシュを確認します（`shasum -a 256 hitomado-vX.Y.Z-macos-arm64.zip`）。zip を開くと `Hitomado.app` ができるので、`/Applications` などへ置いて起動します。動作環境は macOS 12 以上です。

**警告が出る前提です（意図した状態です）。** この `.app` は ad-hoc 署名（`codesign -s -`）だけで、Apple の Developer ID の署名も公証もしていません。ダウンロードした直後は Gatekeeper が「開発元を検証できません」「壊れているため開けません」などの警告を出して起動を止めます。次のどちらかで開けます。

- 初回だけ、Finder で `Hitomado.app` を **右クリック（Control＋クリック）→「開く」** を選び、確認で「開く」を押す（macOS 15 以降は、一度ブロックされたあとに「システム設定 → プライバシーとセキュリティ」の「このまま開く」を押す）。
- ターミナルで検疫属性を外す: `xattr -dr com.apple.quarantine /Applications/Hitomado.app`

ダウンロード元が [公式の Releases](https://github.com/ousiassllc/hitomado/releases) であることと、ハッシュが一致することを確認してから実行してください。データは `~/Library/Application Support/Hitomado/`、資格情報はキーチェーン（サービス名 `Hitomado`）に保存します。アプリ内の更新確認は新しい版のリンクを出すだけで、自動では入れ替えません（新しい `.app` で置き換えてください。データは残ります）。実機での動作確認は限定的です。


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

### Android で使う場合の OAuth クライアント

Android 版（試験段階。[docs/environment/android.md](docs/environment/android.md)）も、上の手順で作る**「デスクトップ アプリ」種別のクライアントをそのまま使います**。Windows 版と同じ Client ID／Client Secret を、Android の設定 → OAuth クライアントにも登録してください。

- **「Android」種別のクライアントは作らないでください**。Android 版は認可コードを 127.0.0.1 の一時ポートで受け取ります。Google は、この方式を Android 種別のクライアントでは認めておらず（[ループバック移行ガイド](https://developers.google.com/identity/protocols/oauth2/resources/loopback-migration)）、カスタム URI スキームも Android では使えません（[OAuth 2.0 for Desktop apps](https://developers.google.com/identity/protocols/oauth2/native-app)。いずれも 2026-11-03 に確認）。デスクトップ アプリ種別のループバックは引き続きサポートされています。
- **パッケージ名（`jp.ousias.hitomado`）と署名鍵の SHA-1 は、Google Cloud に登録しません**（Android 種別だけが要るものです）。したがって、デバッグ鍵で入れた APK でも、自分で署名した release 鍵の APK でも、同じクライアントで認可できます。署名が違う APK へ入れ替えるときに要るのは、`adb uninstall jp.ousias.hitomado`（アプリのデータも消えます）だけです。
- 同意画面が **外部かつ「テスト」** のときは、**Google Auth Platform → 対象（Audience）→ テストユーザー**に、Android で追加する Gmail アドレスを登録してください（登録していないアドレスは認可できません）。「テスト」のままだと 7 日でトークンが失効する点は次のとおりです。
- 認可はブラウザで行います。同意したあと、ブラウザの「このタブを閉じて Hitomado に戻ってください」の表示を見たら、アプリに戻ってください（自動では戻りません）。ブラウザに出ている間にアプリが終了してしまったときは、アプリに「ログインの途中でアプリが終了したため、完了できませんでした」と出るので、もう一度アカウントの追加からやり直してください。
- 実機・本物の Google アカウントでの動作確認はまだです（確認の手順は [docs/environment/android.md §14.6](docs/environment/android.md#146-実機で確認する手順未確認)）。

### 重要: 「テスト」のままだと 7 日でトークンが失効します

OAuth 同意画面の公開ステータスが **外部かつ「テスト」** のままだと、**リフレッシュトークンが 7 日で失効**し、そのたびにアカウントが「要再接続」になります。次のいずれかで回避してください。

- **「本番環境」に切り替える**（OAuth 同意画面 → アプリを公開）。審査を受けない「未確認のアプリ」のまま使えます。100 ユーザーの上限は個人利用では問題になりません。
- **Workspace の「内部」アプリ**にする（その組織のアカウントだけで使う場合）。ただし他社のアカウントには使えないため、会社ごとに別の OAuth クライアントを作って登録します。

### 「このアプリは Google で確認されていません」と出たら

本番環境で未確認のアプリを使うと、認可時に警告が出ます。自分で作ったクライアントなので、**「詳細」→「（アプリ名）に移動（安全でない）」** を選んで続行してください。

### Workspace の管理者がアクセスを制限している場合

組織の管理者が外部アプリのアクセスを制限していると認可できません。管理者に、Google Workspace 管理コンソールの **セキュリティ → API の制御** で、この OAuth クライアントを許可してもらってください。

## データの保存とセキュリティ

- リフレッシュトークン・Client Secret・AI の API キー・ChatGPT ログインのトークンは **Windows 資格情報マネージャー**に保存します。ファイル・ログに平文では残りません。
- ローカル DB（`%APPDATA%\Hitomado\hitomado.db`）には、**メール本文・添付は保存しません**。保存するのは差出人・件名・日時・スニペット・ラベル・同期状態・宛先候補のみです。件名とスニペットは暗号化されないため、**BitLocker などのディスク暗号化の利用を推奨**します。
- 保存した添付ファイルには Windows の Mark-of-the-Web（`Zone.Identifier` の `ZoneId=3`）を付けます。開くときに SmartScreen／保護ビューが働きます（FAT/exFAT など代替データストリーム非対応のボリュームでは付かず、保存自体は成功します）。
- ログ（`%APPDATA%\Hitomado\logs\`）にトークン・パスワード・OAuth 情報・本文・件名・宛先は出力しません（書き込む前に伏せます）。設定 → ログ から、フォルダを開く・書き出すことができます。形式・保持期間は [docs/mcp.md](docs/mcp.md#ログ) にあります。
- ChatGPT のプランでのログイン（Sign in with ChatGPT）は OpenAI の公式画面（システムブラウザ）で行い、Hitomado はパスワードもセッション Cookie も受け取りません。Codex CLI など他アプリの認証情報は読みません。Hitomado は OpenAI の公式アプリではありません。使った分は ChatGPT の設定の使用状況（https://chatgpt.com/settings/usage）で確認でき、連携はそこから解除できます。
- MCP サーバーは既定でオフです。オンにしても、アプリが作るトークン（Bearer）が無い接続は拒否します。既定は `127.0.0.1` のみで待ち受け、別のアドレス（tailnet など）は設定で明示的に許可したときだけです。トークンは Windows 資格情報マネージャーに保存し、再発行・無効化できます。MCP には送信の機能が無く、下書きは必ずあなたがアプリで確認して送ります。
- アプリが行う外部通信先は次の 6 種類だけです。公開者のサーバーへは一切通信しません。テレメトリもありません。
  - Google OAuth（`accounts.google.com`／`oauth2.googleapis.com`／`openidconnect.googleapis.com`。認可・トークン更新・アドレス取得）
  - Gmail（`gmail.googleapis.com`。メールの取得・操作・送信）
  - AI プロバイダ（設定したもの。「AI で下書き」「AI で要約」や質問の送信を押したときだけ、スレッドの内容〔下書きは本文の一部・表示名・署名、要約・質問は全メッセージの本文など〕を送信）
  - ChatGPT のログイン（`auth.openai.com`。設定で「ChatGPT プラン」を選んでログインしたときだけ。認可・トークン更新・失効）。ChatGPT プランで下書きを作るときの送信先は `api.openai.com` です
  - TypeSafe AI（`api.typesafe.ai`。「Inbox Zero」ボタンを押したときだけ。スレッドごとに、最新のメッセージの差出人・件名・本文の先頭約 500 文字を送信します。初回に同意が必要で、同意前は送りません。「返信済み」だけをオンにしたときは通信しません）
  - GitHub Releases（`api.github.com` で新バージョンの確認、`github.com` と配信元の `release-assets.githubusercontent.com` で Windows 版の自動アップデートのダウンロード。設定でオフにできます。macOS 版と Android 版は新しい版の確認と案内のリンクだけで、自動では入れ替えません）
- 例外: メールのリモート画像は既定でブロックします。メッセージごとに「画像を表示」を押したときだけ、そのメールが参照する画像を WebView が直接取得します（取得先はメールの送信者が指定したホストです）。

## AI 返信下書きの設定

設定 → **AI** で、プロバイダとモデルを選びます。

- **Anthropic／OpenAI 互換**: API キーを登録します（Windows 資格情報マネージャーに保存し、画面には再表示しません）。OpenAI 互換のサービスでは Base URL も指定できます。
- **ChatGPT プラン**: プロバイダで「ChatGPT プラン」を選び、「Continue with ChatGPT」でログインします（ChatGPT Plus／Pro が対象です。Business などほかのプランは公式ドキュメントで確認できていません）。API キーは要りません。複数の ChatGPT アカウントを登録でき（同じメールアドレスでも別の登録）、設定画面の一覧から使うアカウントを選んで切り替えられます。ログアウトは選んだアカウントだけに行い、そのトークンを削除して OpenAI 側の連携も失効させます（ほかのアカウントのトークンと登録は残ります）。
- **TypeSafe AI（Inbox Zero 用）**: 設定 → AI の「TypeSafe AI」のページで API キーを登録します（返信下書きのプロバイダにはなりません）。同じページで、Inbox Zero が「不要」とみなす理由（広告・宣伝／通知だけのメール／未読のニュースレター／返信済みのメール）をオン・オフできます（既定はすべてオン）。ボタンを押すと、いまのタブに読み込まれているメールの先頭 100 件までを調べ、候補を一覧で出します。チェックを確認して「アーカイブする」を押したときだけアーカイブし、「元に戻す」もできます。初めて使うときに、件名・差出人・本文の先頭約 500 文字が TypeSafe AI へ送信されることへの同意を取ります。
- **モデル**: 送信先への同意のあと、API キーの登録・変更、Base URL の変更、ログインのあと、および「一覧を更新」で、使えるモデルを取得して選べます（自動の定期取得はしません）。一覧が取れない（ローカルの LLM など Models API が無い、通信できない、権限がない）ときは、理由を表示して手入力に切り替わります。
- 送信先（プロバイダと URL）は設定画面に常に表示します。初めて使うときに、スレッドの内容が送信されることへの同意を取ります（プロバイダと送信先ごと）。

## MCP でつなぐ

1. 設定 → **MCP** で有効にし（既定はオフ）、「トークンを発行」を押します（値は 1 回だけ表示されます）。
2. MCP クライアントに接続先 URL（既定 `http://127.0.0.1:47650/mcp`、Streamable HTTP）と `Authorization: Bearer <トークン>` を設定します。

使えるのは、状態・ログ・アカウント・スレッド（一覧・検索・取得）の読み取りと、返信の下書き作成です。送信はできません。設定・ツール・ログの中身の詳細は [docs/mcp.md](docs/mcp.md) を見てください。

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
| `Ctrl+Enter` | エディタ内で送信（既定は確認ダイアログを開く。設定「送信前に確認画面を出す」がオフで警告が無いときは、そのまま送信する） |

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

- 動作保証の対象は Windows です（Android・macOS は試験段階の検証用の配布です）。Linux では Go の単体テスト・フロントのテスト・lint を実行できますが、アプリ本体の実行確認は Windows で行ってください。Linux で Go をビルド・テストするときは GTK を使わない `CGO_ENABLED=0 go test -tags purego ./...` を使います。
- `task dev`・`task build`（`VERSION` なし）の版は `0.0.0` です（`task package VERSION=X.Y.Z` で渡した値だけが配布物の版になります）。`0.0.0` の dev・ローカルビルドは更新確認も自動アップデートもしません（設定画面にも新しい版は出ません）。
- **Android 版のビルド**（APK／AAB。`task android:package ARCH=arm64 VERSION=X.Y.Z` で `bin/hitomado.apk`、`task android:install` で実機へ）の手順・署名・版番号は [docs/environment/android.md](docs/environment/android.md) にあります。まだ試験段階で、Google Play では配布していません（Releases の APK は検証用です）。
- 仕様書は [docs/overview.md](docs/overview.md) のドキュメントマップから辿れます。
- マージは squash／rebase せず、マージコミット方式です。コミットには `git commit -s`（Signed-off-by）を付けます。
- リリースは自動です。`main` にマージされるたびに、GitHub Actions が最新の `v*` タグから patch を 1 つ上げたタグ（`v0.1.0` → `v0.1.1`。タグがまだ無ければ最初は `v0.1.0`）を作り、Windows 向けのインストーラー（`hitomado-vX.Y.Z-windows-amd64-setup.exe`）・Android の APK（`hitomado-vX.Y.Z-android-arm64.apk`）・macOS の zip（`hitomado-vX.Y.Z-macos-arm64.zip`・`hitomado-vX.Y.Z-macos-amd64.zip`）・`NOTICE`・`SHA256SUMS.txt` を Releases に添付します（Windows は署名なし、Android は固定鍵の署名（Google Play の署名ではありません）、macOS は ad-hoc 署名のみで、Windows・macOS は正式な署名・公証をしません）。どれか 1 つのビルドが失敗したら Release は作りません。Android の固定鍵は管理者が GitHub Secrets（`ANDROID_KEYSTORE_BASE64`・`ANDROID_KEYSTORE_PASSWORD`・`ANDROID_KEY_ALIAS`・`ANDROID_KEY_PASSWORD`）に登録します（鍵の作り方・登録の手順・控えは [docs/environment/android.md §4.1](docs/environment/android.md#41-リリースの固定鍵github-secrets310)）。Secrets が無いと、使い捨ての鍵では公開せず Release の Android のビルドが失敗します。PR の CI（フォークを含む）は Secrets を使わず、使い捨ての鍵でビルドします。手元でインストーラーを作るときは、Windows で [NSIS](https://nsis.sourceforge.io/)（`makensis`）を入れて `task package VERSION=X.Y.Z` を実行します（`bin/` に出力）。macOS の `.app`・zip は、macOS 上で `task package VERSION=X.Y.Z`（Apple Silicon なら arm64。Intel 向けは `ARCH=amd64` を付ける）で `bin/` に作れます。手でタグを打つ必要はありません。
