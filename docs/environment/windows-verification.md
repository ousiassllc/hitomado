# Windows 実機・実 Gmail での確認

> 関連: [環境構築 §4.3 Windows インストーラー](setup.md#43-windows-インストーラー) / [MCP 連携とログ](../mcp.md) / [コンポーネント設計](../components/overview.md)

CI（Linux・windows-latest）とローカルのテストでは確かめられず、Windows の実機や本物の Google アカウントが要る項目を集める。#1・#10・#14 を PR（#2・#11・#22）のマージで close したとき、各 PR の「未確認」に残った項目を [#174](https://github.com/ousiassllc/hitomado/issues/174) で追跡している。本書はその手順と、確かめた結果の記録場所である。

自動化できない理由は、Windows のデスクトップ（資格情報マネージャー・トースト・トレイ・インストーラーの GUI）と、実 Google のアカウント・同意画面を CI から操作できないこと。ここで確かめた内容は、結果（日付・版・環境・合否）を §結果 に追記し、#174 のチェックボックスと対応させる。

## 準備

- Windows 10 以降の x64 実機（または仮想マシン）。WebView2 ランタイムが入っていない状態と入っている状態の両方があるとよい。
- 確認する版のインストーラーと `SHA256SUMS.txt`（[Releases](https://github.com/ousiassllc/hitomado/releases) から取る）。`Get-FileHash -Algorithm SHA256 <ファイル>` の値が `SHA256SUMS.txt` の行と一致することを先に確かめる。
- 実 Gmail の確認用に、テスト用の Google アカウントと、**デスクトップ アプリ種別**の OAuth クライアント（作り方は [README](../../README.md) の OAuth クライアントの節）。
- 既に Hitomado を使っている PC では、`%APPDATA%\Hitomado\` と資格情報マネージャーの `Hitomado:` の項目が実データと共有になる。確認前にバックアップするか、専用の Windows ユーザー・仮想マシンを使う。

## 確認項目と手順

### A. 資格情報マネージャー・トースト・トレイ・単一インスタンス・`wails3 build`（PR #2）

1. `wails3 build`（または `task package VERSION=X.Y.Z`）で exe を作り、エラーなく起動する。
2. アカウントを追加し、コントロールパネル → 資格情報マネージャー → Windows 資格情報に、`Hitomado:` で始まる項目（ターゲット名 `Hitomado:hitomado/<種別>/<ID>`）ができることを確かめる。アプリを閉じて開き直しても再認証なしで使える。アカウントを削除すると該当の項目が消える。
3. 新着（テスト用アカウントへ別のアカウントからメールを送る）でトースト通知が出る。トーストをクリックすると、そのスレッドが開く。通知をオフにしたアカウントでは出ない。
4. ウィンドウの × で終了せずトレイへ格納される。トレイメニューの「開く」で戻り、未読があるときはトレイアイコンにバッジが付く。「終了」でプロセスが終わる（タスクマネージャーに `hitomado.exe` が残らない）。
5. 起動中にもう一度 `hitomado.exe` を起動すると、2 つ目のプロセスは終了し、既存のウィンドウが前面に出る（ウィンドウがトレイにあるときも同じ）。

### B. 実 Google／Gmail への OAuth・API 通信（PR #2）

CI は `httptest` の偽サーバーだけを使うため、実通信はここでしか確かめられない。

1. 設定 → OAuth クライアントに Client ID／Client Secret を登録し、アカウントの追加でブラウザの認可を完了する。完了ページが出てアプリに戻れる。
2. 「テスト」公開状態の同意画面では、テストユーザーに登録した Gmail でだけ認可でき、「未確認アプリ」の警告は「詳細」から続行できる。
3. 受信トレイ・スレッド表示・既読／未読・アーカイブ・返信（送信元が受信アカウントに固定され、送信前に確認が出る）・下書き保存が、実 Gmail で動く。差分同期（`history.list`）で新着が反映される。
4. 同意画面を取り消す（Google アカウントのサードパーティアクセスから削除する）と、アカウントが「要再接続」になり、再接続で復旧する。

### C. インストーラーの GUI と「アプリと機能」（PR #11）

1. インストーラー（`hitomado-vX.Y.Z-windows-amd64-setup.exe`）を普通に実行する。UAC は出ない。ウィザードにコンポーネント選択（スタートメニュー必須・デスクトップは任意。既定はオフ）と完了ページが出て、文言・レイアウトが崩れていない。
2. 設定 → アプリ → インストールされているアプリ（「アプリと機能」）に Hitomado が出る。表示名・版・発行元・インストール先が正しい。そこからアンインストールでき、アンインストールでプログラム・ショートカット・登録が消え、`%APPDATA%\Hitomado\` と資格情報マネージャーの項目は残る。
3. WebView2 ランタイムが無い PC では、インストーラーが同梱のブートストラッパーで入れる。

### D. 実アプリが起動中の上書きインストール拒否（PR #11）

CI（`.github/scripts/test-installer.ps1`）は `ping.exe` を `hitomado.exe` の名前で常駐させて代用している。実アプリでは次を確かめる。

1. 旧版を入れて起動したまま、新しい版のインストーラーを GUI で実行する。終了を促す表示が出て、再試行できる（何も書き換わらない）。
2. 起動したまま `setup.exe /S` を実行すると、何も変えずに終了コード 5 で終わる（`$LASTEXITCODE`）。「アプリと機能」の版は変わらない。
3. アプリを終了してから実行すると上書きでき、版が上がる。トレイに格納しただけ（プロセスが残る状態）でも 1・2 と同じ扱いになる。

### E. 初回リリースの成果物（PR #11）

Releases にインストーラーと `SHA256SUMS.txt` が付くこと。Linux から `gh` で確かめられる（結果は §結果）。

```
gh release view <tag> --json assets -q '.assets[].name'
gh release download <tag> -p SHA256SUMS.txt -p '*setup.exe'
sha256sum -c SHA256SUMS.txt --ignore-missing
```

### F. MCP・ログの設定画面（PR #22）

1. 設定 → MCP: 既定はオフ。オンにすると待ち受けが始まり、「トークンを発行」で値が発行直後に 1 回だけ表示・コピーできる。再発行で古いトークンが 401 になる。無効化は確認モーダルを経由する。トークンは資格情報マネージャーに保存され、ファイル・ログに平文で残らない（`%APPDATA%\Hitomado\logs\` を検索して確かめる）。
2. `Authorization: Bearer <トークン>` つきで [MCP 連携](../mcp.md#つなぎ方) の接続先 URL へつなげる。トークン無し・不一致は 401。
3. 設定 → ログ: 「ログフォルダを開く」でエクスプローラーが `%APPDATA%\Hitomado\logs\` を開く。「ログの書き出し」で保存先を選べ、書き出したファイルにトークン・本文・件名・宛先が含まれない。

### G. 本文 iframe の高さ計測・リンク捕捉・スクリプト遮断（#49）

対象は `MessageBody` の `<iframe sandbox="allow-same-origin" srcdoc>`（[コンポーネント設計 §3.8](../components/overview.md#38-本文の表示方針messagebodylibsanitize)。NF-S04）。自動テストは happy-dom 上のもの（`thread.test.ts`・`bodyFrame.test.ts`・`lib/sanitize/regression.test.ts`）で、WebView2 の実挙動は確かめていない。Windows 実機で、DevTools が開ける開発ビルド（`wails3 dev`。F12）を起動して確かめる。1・2 は、テスト用アカウントの受信箱にある長い HTML メール（ニュースレター等。画像・リンクを含むもの）を使う。

1. **高さ**: 長い HTML メールを開く。iframe に内部のスクロールバーが出ず、内容の高さに合う（`bodyFrame.ts` の `measured` が真）。「画像を表示」を押して画像が読み込まれたあとも、高さが内容に追従して切れない・余白が残らない。ペインの幅を変えても追従する。計測できなかった場合は固定高 480px＋内部スクロールになる（§3.8 の代替案①）ので、その状態になっていないことを見る。
2. **リンク**: 本文のリンクを、左クリック・中クリックで操作する。(a) `https://` と `mailto:` は既定のブラウザ／メーラーで開き（`ThreadService.OpenExternal`）、iframe の中はナビゲーションしない（本文が置き換わらない）。(b) `javascript:`・`file:///C:/Windows/win.ini`・`ftp:` のリンクは何も開かず、iframe も動かない。右クリックは何もしない。
3. **スクリプトの遮断**: サニタイズを通さずに危険な HTML を iframe へ入れ、`sandbox` と CSP のそれぞれ単独で止まることを確かめる。DevTools のコンソールで、メール本文を開いた状態から次を実行する（`f` は本文の iframe）。

   ```js
   const f = document.querySelector('[data-testid="body-frame"]')
   const csp = (v) => `<meta http-equiv="Content-Security-Policy" content="${v}">`
   const CSP = "default-src 'none'; img-src data:; style-src 'unsafe-inline'"
   const body =
     '<p id="p">probe</p>' +
     '<script>window.__script = "ran"; document.getElementById("p").textContent = "script ran"<\/script>' +
     '<img src="x" onerror="window.__onerror = \'ran\'">' +
     '<a href="javascript:window.__href = \'ran\'">js link</a>'
   // 判定: f.contentWindow.__script / __onerror が undefined のまま・"probe" のまま・iframe 内のリンクを押しても __href が undefined
   f.srcdoc = '<!doctype html>' + csp(CSP) + body        // ① sandbox + CSP（本番と同じ）
   // ② sandbox のみ（CSP を外す）。属性を直接書き換えると再読み込みされる
   f.srcdoc = '<!doctype html>' + body
   // ③ CSP のみ（sandbox を外す。確認後は本文を開き直して元に戻す）
   f.removeAttribute('sandbox'); f.srcdoc = '<!doctype html>' + csp(CSP) + body
   ```

   ①②③のいずれも `__script`・`__onerror`・`__href` が `undefined`、本文の文字が `probe` のまま（スクリプトも `onerror` も実行されない）なら合格。③は `onerror` などのインラインスクリプトを CSP（`default-src 'none'`）が止めること、②は `sandbox`（`allow-scripts` なし）が止めることを示す。①が実行されたら NF-S04 を満たさない。②③のどちらかが実行されたら、その層が効いておらず多層防御が 1 枚になっている。いずれも別の Issue を作り、`allow-same-origin` を外す設計（§3.8 の代替案）を検討する。
   本番の経路（`sanitizeEmailHtml` → `buildSrcdoc`）の確認として、`body` と同じ HTML を本文にしたテストメールを送り、`<script>`・`onerror`・`javascript:` が除去されて `probe` だけが表示されることも見る。

別の Issue で追跡している項目（本書の対象外）: ログのローテーション中の読み取りとフォルダ表示は #26。ARM64 の Windows 11 での起動・インストーラーは #13 で CI（`installer-arm64`）に載せた（[setup.md §4.3](setup.md)「ARM64 の扱い」）。

## 結果

| 項目 | 確認日 | 版 | 環境 | 結果 |
|------|--------|----|------|------|
| A. 資格情報マネージャー・トースト・トレイ・単一インスタンス・`wails3 build` | — | — | — | **未確認**（Windows 実機が要る） |
| B. 実 Google／Gmail への OAuth・API 通信 | — | — | — | **未確認**（実アカウントが要る） |
| C. インストーラーの GUI・「アプリと機能」 | — | — | — | **未確認**（Windows のデスクトップが要る） |
| D. 実アプリ起動中の上書きインストール拒否 | — | — | — | **未確認**（Windows 実機が要る。CI は `ping.exe` で代用） |
| E. 初回リリースの成果物 | 2026-11-06 | v0.1.16 | Linux から `gh` で確認 | **確認済み**。Release に `hitomado-v0.1.16-windows-amd64-setup.exe`・`NOTICE`・`SHA256SUMS.txt`（Android の APK・macOS の zip も）が付いている。`SHA256SUMS.txt` に Windows インストーラーの行があり、`sha256sum -c` で一致した。初回のリリース（v0.1.0 など）はすでに別の版に置き換わっているため、現行の版で確かめた。`release.yml` の `publish` が、成果物の欠けた Release を出さない作りになっている（[setup.md §4.2](setup.md#42-releaseyml)） |
| F. MCP・ログの設定画面の GUI 操作 | — | — | — | **未確認**（Windows 実機が要る） |
| G. 本文 iframe の高さ計測・リンク捕捉・スクリプト遮断（#49） | — | — | — | **未確認**（WebView2 の実機が要る。happy-dom の自動テストのみ） |

未確認の行は、確かめたら日付・版・環境・合否を書いて置き換える。確かめて問題が見つかったら、別の Issue を作って番号をここに書く。
