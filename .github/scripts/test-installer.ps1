# Windows インストーラーの動作確認（CI の build ジョブで実行する）。
#
#   旧版を入れる → 起動中は上書きを拒否する → 新版で上書きする → アンインストールする
#
# 確認すること:
#   - スタートメニューのショートカットと「アプリと機能」の登録（DisplayVersion）ができる
#   - 起動中（実行中の exe がある）なら、サイレントインストールは何も変えずに終了コード 5 で終わる
#   - /WAIT=秒 つきなら、起動中でも終了を待ち、終了したあとで上書きして終了コード 0 で終わる（アプリの自動アップデートが使う）
#   - 上書きで版が上がり、exe が新しいものに置き換わり、exe のファイルバージョンと埋め込まれた
#     main.version が版と一致する
#   - アンインストールでプログラム・ショートカット・登録は消え、利用者のデータ（%APPDATA%\Nefos Inbox）は残る
#   - v0.1.46 までの Hitomado（%LOCALAPPDATA%\Programs\Hitomado・hitomado.exe）の上に上書きすると、起動中なら /WAIT で待ち、
#     %LOCALAPPDATA%\Programs\Nefos Inbox の nefos-inbox.exe へ替わり、旧いインストール先・旧名のショートカットが残らず、
#     スタートメニューとデスクトップのショートカット・「アプリと機能」の登録が新しい場所を指す
#
# 利用者の環境を壊さないため、既に Nefos Inbox（旧名 Hitomado）が入っている・データがある PC では -Force なしでは動かない
# （登録キーと %APPDATA%\Nefos Inbox・既定のインストール先を本物と共有するため）。-Force でも、消すのはこのスクリプトが作った
# マーカーだけで、本物のデータは消さない（ただし入っている Nefos Inbox は旧版で上書きされ、最後に消える）。
#
# 使い方（版は、exe に埋め込んだ版を他と取り違えないよう、他に出てこない値にしておく）:
#   pwsh .github/scripts/test-installer.ps1 -OldInstaller old.exe -OldVersion 0.99.1 `
#        -NewInstaller new.exe -NewVersion 0.99.2 -Exe bin\nefos-inbox.exe -OldExe old-nefos-inbox.exe
param(
  [Parameter(Mandatory)][string]$OldInstaller,
  [Parameter(Mandatory)][string]$OldVersion,
  [Parameter(Mandatory)][string]$NewInstaller,
  [Parameter(Mandatory)][string]$NewVersion,
  # NewInstaller に同梱した nefos-inbox.exe（置き換わったことと、埋め込まれた版の照合用）
  [Parameter(Mandatory)][string]$Exe,
  # OldInstaller に同梱した nefos-inbox.exe（新版の exe に旧版が混ざっていないことの照合用）
  [Parameter(Mandatory)][string]$OldExe,
  [switch]$Force
)
$ErrorActionPreference = 'Stop'

$uninstKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\jp.ousias.hitomado'
$installDir = Join-Path ([IO.Path]::GetTempPath()) 'nefos-inbox-installer-test'
$startMenuLink = Join-Path ([Environment]::GetFolderPath('Programs')) 'Nefos Inbox.lnk'
$desktopLink = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Nefos Inbox.lnk'
$dataDir = Join-Path $env:APPDATA 'Nefos Inbox'
$marker = Join-Path $dataDir 'installer-test-marker.txt'

function Assert([bool]$Condition, [string]$Message) {
  if (-not $Condition) { throw "FAIL: $Message" }
  Write-Host "ok   $Message"
}

# 条件が満たされるまで上限つきで待つ（固定時間の sleep に頼らない）。
function Wait-Until([scriptblock]$Condition, [int]$TimeoutSec = 60) {
  $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSec)
  while (-not (& $Condition)) {
    if ([DateTime]::UtcNow -gt $deadline) { return $false }
    Start-Sleep -Milliseconds 200
  }
  return $true
}

# インストーラーを実行して終了コードを返す。失敗の原因が分かるよう、終了コードは判定の前に必ずログへ出す。
function Invoke-Installer([string]$Installer, [string[]]$Arguments) {
  $code = (Start-Process -FilePath $Installer -ArgumentList $Arguments -Wait -PassThru).ExitCode
  Write-Host "installer $(Split-Path $Installer -Leaf) $($Arguments -join ' ') exit code: $code"
  $code
}

# NSIS の /D= は最後の引数で、引用符なし。
function Install([string]$Installer) {
  Invoke-Installer $Installer @('/S', "/D=$installDir")
}

# NSIS の CheckNotRunning と同じ判定（nefos-inbox.exe を書き込みで開けるか。共有は読み取りだけ許す）。
# ウイルス対策ソフトが書いたばかりの exe を走査している間なども、ここで開けない。
function Test-ExeWritable([string]$Path) {
  try {
    [IO.File]::Open($Path, 'Open', 'Write', 'Read').Dispose()
    $true
  } catch [IO.IOException], [UnauthorizedAccessException] {
    # 共有違反は IOException、走査中・実行中の exe への書き込み open がアクセス拒否で返ることは UnauthorizedAccessException。
    $false
  }
}

# $installDir の nefos-inbox.exe を動かしているプロセス。
function Get-ExeProcesses {
  @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq "$installDir\nefos-inbox.exe" })
}

# nefos-inbox.exe のプロセスが無くなり、exe を書き込みで開けるようになるまで待つ。次のインストーラーが
# 起動中の exe に当たって終了コード 5 で止まらないよう、続けて入れる前にこれを通す。
function Wait-ExeReleased {
  $sw = [Diagnostics.Stopwatch]::StartNew()
  $released = Wait-Until { (Get-ExeProcesses).Count -eq 0 -and (Test-ExeWritable "$installDir\nefos-inbox.exe") }
  # ほぼ 0 ms なら、この待ちは効いていない（原因は別）。数百 ms 以上なら、2b の直後に exe が実際に掴まれていた証拠になる。
  Write-Host "waited $([int]$sw.Elapsed.TotalMilliseconds) ms for nefos-inbox.exe to be released (released: $released)"
  if (-not $released) {
    Write-Host "nefos-inbox.exe is still held. processes: $((Get-ExeProcesses | ForEach-Object { "$($_.Id):$($_.ProcessName)" }) -join ', ')"
  }
  $released
}

function Displayed-Version { (Get-ItemProperty -Path $uninstKey -ErrorAction Stop).DisplayVersion }

# exe に -X main.version で埋め込まれた版（Go の文字列定数なので ASCII のまま入っている）。
function Embeds-Version([string]$Path, [string]$Version) {
  [IO.File]::ReadAllText($Path, [Text.Encoding]::Latin1).Contains($Version)
}

# v0.1.46 までの Hitomado の入れ方（#402）。上書き更新で、新しい名前の場所・exe・ショートカットに替わり、古いものが残らないことを確かめる。
$legacyDir = Join-Path $env:LOCALAPPDATA 'Programs\Hitomado'
$defaultDir = Join-Path $env:LOCALAPPDATA 'Programs\Nefos Inbox'
$legacyStartMenuLink = Join-Path ([Environment]::GetFolderPath('Programs')) 'Hitomado.lnk'
$legacyWebViewDir = Join-Path $env:APPDATA 'hitomado.exe'
$webViewDir = Join-Path $env:APPDATA 'nefos-inbox.exe'

function New-Shortcut([string]$Path, [string]$Target) {
  $link = (New-Object -ComObject WScript.Shell).CreateShortcut($Path)
  $link.TargetPath = $Target
  $link.Save()
}

function Get-ShortcutTarget([string]$Path) { (New-Object -ComObject WScript.Shell).CreateShortcut($Path).TargetPath }

# 旧名の Hitomado が入っている状態を作る。起動中のアプリの代わりに、ping.exe を hitomado.exe の名前で置く。
function New-LegacyInstall {
  New-Item -ItemType Directory -Force -Path $legacyDir, $legacyWebViewDir | Out-Null
  Copy-Item "$env:SystemRoot\System32\PING.EXE" "$legacyDir\hitomado.exe" -Force
  Set-Content -Path "$legacyDir\NOTICE.txt" -Value 'legacy notice'
  Set-Content -Path "$legacyDir\uninstall.exe" -Value 'legacy uninstaller'
  Set-Content -Path "$legacyWebViewDir\legacy-webview.txt" -Value 'webview data'
  New-Item -Path $uninstKey -Force | Out-Null
  Set-ItemProperty -Path $uninstKey -Name DisplayName -Value 'Nefos Inbox'
  Set-ItemProperty -Path $uninstKey -Name DisplayVersion -Value '0.1.46'
  Set-ItemProperty -Path $uninstKey -Name InstallLocation -Value $legacyDir
  Set-ItemProperty -Path $uninstKey -Name UninstallString -Value "`"$legacyDir\uninstall.exe`""
  New-Shortcut $startMenuLink "$legacyDir\hitomado.exe"
  New-Shortcut $legacyStartMenuLink "$legacyDir\hitomado.exe"
  New-Shortcut $desktopLink "$legacyDir\hitomado.exe"
}

function Remove-LegacyLeftovers {
  foreach ($p in $legacyDir, $defaultDir, $legacyWebViewDir, $webViewDir, $legacyStartMenuLink, $startMenuLink, $desktopLink) {
    Remove-Item $p -Recurse -Force -ErrorAction SilentlyContinue
  }
}

if (-not $Force) {
  if ((Test-Path $uninstKey) -or (Test-Path $dataDir) -or (Test-Path $legacyDir) -or (Test-Path $defaultDir)) {
    throw 'Nefos Inbox is already installed or has data here. This test shares the registry key and %APPDATA%\Nefos Inbox with the real app; use -Force on a throwaway machine (CI).'
  }
}
$dataDirExisted = Test-Path $dataDir

# 0. v0.1.46 までの Hitomado の上に、新版を上書きする。起動中の旧い exe があれば /WAIT で終了を待ち、そのあとで
#    新しい場所（Nefos Inbox）へ入れ替わり、旧いインストール先・ショートカット・exe が残らない。
function Test-LegacyUpgrade {
  Remove-LegacyLeftovers
  Remove-Item $uninstKey -Recurse -Force -ErrorAction SilentlyContinue
  New-LegacyInstall
  $running = Start-Process -FilePath "$legacyDir\hitomado.exe" -ArgumentList '-t', '127.0.0.1' -WindowStyle Hidden -PassThru
  try {
    Assert (-not $running.HasExited) 'stand-in hitomado.exe (legacy) is running'
    $code = Invoke-Installer $NewInstaller @('/S')
    Assert ($code -eq 5) 'installer refuses to overwrite a running legacy hitomado.exe (exit code 5)'
    Assert (Test-Path "$legacyDir\hitomado.exe") 'a refused install leaves the legacy install in place'
    $waiting = Start-Process -FilePath $NewInstaller -ArgumentList '/S', '/WAIT=60' -PassThru
    $null = $waiting.Handle
    Assert (-not $waiting.WaitForExit(3000)) 'installer with /WAIT keeps waiting while the legacy hitomado.exe is running'
  } finally {
    Stop-Process -Id $running.Id -Force -ErrorAction SilentlyContinue
    $running.WaitForExit()
  }
  Assert ($waiting.WaitForExit(60000)) 'installer with /WAIT finishes after the legacy hitomado.exe exits'
  Assert ($waiting.ExitCode -eq 0) 'installer with /WAIT exits 0 after the legacy exe exits'

  Assert (Test-Path "$defaultDir\nefos-inbox.exe") 'nefos-inbox.exe is installed in the Nefos Inbox folder'
  Assert ((Get-FileHash "$defaultDir\nefos-inbox.exe").Hash -eq (Get-FileHash $Exe).Hash) 'the new exe is the one from this build'
  Assert (-not (Test-Path $legacyDir)) 'the legacy Hitomado install folder is removed'
  Assert (-not (Test-Path $legacyStartMenuLink)) 'the legacy Hitomado Start Menu shortcut is removed'
  Assert ((Get-ShortcutTarget $startMenuLink) -eq "$defaultDir\nefos-inbox.exe") 'the Start Menu shortcut points to the new exe'
  Assert ((Get-ShortcutTarget $desktopLink) -eq "$defaultDir\nefos-inbox.exe") 'the existing desktop shortcut points to the new exe'
  Assert ((Displayed-Version) -eq $NewVersion) "Apps & features shows $NewVersion after the legacy upgrade"
  Assert ((Get-ItemProperty $uninstKey).InstallLocation -eq $defaultDir) 'InstallLocation points to the new folder'
  Assert ((Get-ItemProperty $uninstKey).UninstallString -eq "`"$defaultDir\uninstall.exe`"") 'the uninstall command points to the new folder'
  Assert (-not (Test-Path $legacyWebViewDir)) 'the legacy WebView2 data folder (hitomado.exe) is gone'
  Assert (Test-Path "$webViewDir\legacy-webview.txt") 'the WebView2 data moved to the new exe name'
  Assert (Test-Path $marker) 'user data is untouched by the installer (the app moves it at startup)'

  Start-Process -FilePath "$defaultDir\uninstall.exe" -ArgumentList '/S' -Wait
  Assert (Wait-Until { -not (Test-Path $uninstKey) }) 'uninstall after the legacy upgrade removes the Apps & features entry'
  Assert (Wait-Until { -not (Test-Path $defaultDir) }) 'uninstall after the legacy upgrade removes the Nefos Inbox folder'
  Assert (-not (Test-Path $desktopLink)) 'uninstall removes the desktop shortcut'
  Remove-LegacyLeftovers
}

try {
  if (Test-Path $installDir) { Remove-Item $installDir -Recurse -Force }
  New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
  Set-Content -Path $marker -Value 'user data must survive uninstall'
  Test-LegacyUpgrade

  # 1. 新規インストール（旧版）
  Assert ((Install $OldInstaller) -eq 0) 'old installer exits 0'
  Assert (Test-Path "$installDir\nefos-inbox.exe") 'nefos-inbox.exe is installed'
  Assert (Test-Path "$installDir\uninstall.exe") 'uninstall.exe is installed'
  Assert (Test-Path "$installDir\NOTICE.txt") 'NOTICE.txt is installed'
  Assert (Test-Path $startMenuLink) 'Start Menu shortcut exists'
  Assert ((Displayed-Version) -eq $OldVersion) "Apps & features shows $OldVersion"
  Assert ((Get-ItemProperty $uninstKey).DisplayName -eq 'Nefos Inbox') 'Apps & features shows the name'
  Assert ((Get-ItemProperty $uninstKey).InstallLocation -eq $installDir) 'InstallLocation is recorded'
  Assert (-not (Test-Path $desktopLink)) 'desktop shortcut is optional (not created silently)'
  Assert (Embeds-Version "$installDir\nefos-inbox.exe" $OldVersion) "installed nefos-inbox.exe embeds main.version $OldVersion"
  Assert ((Get-Item "$installDir\nefos-inbox.exe").VersionInfo.ProductVersion -eq $OldVersion) "installed nefos-inbox.exe file version is $OldVersion"

  # 2. 起動中は上書きしない（実行中の exe は書き込みで開けない。ping.exe を nefos-inbox.exe の名前で動かして代わりにする）
  Copy-Item "$env:SystemRoot\System32\PING.EXE" "$installDir\nefos-inbox.exe" -Force
  $running = Start-Process -FilePath "$installDir\nefos-inbox.exe" -ArgumentList '-t', '127.0.0.1' -WindowStyle Hidden -PassThru
  try {
    Assert (-not $running.HasExited) 'stand-in nefos-inbox.exe is running'
    $code = Install $NewInstaller
    Write-Host "installer exit code while running: $code"
    Assert ($code -eq 5) 'installer refuses to overwrite a running nefos-inbox.exe (exit code 5)'
    Assert ((Displayed-Version) -eq $OldVersion) 'a refused install leaves the registration unchanged'
  } finally {
    Stop-Process -Id $running.Id -Force -ErrorAction SilentlyContinue
    $running.WaitForExit()
  }

  # 2b. /WAIT=秒 つきなら、起動中は待ち、終了したあとで上書きする（自動アップデートは、終了前にインストーラーを起動する）
  $running = Start-Process -FilePath "$installDir\nefos-inbox.exe" -ArgumentList '-t', '127.0.0.1' -WindowStyle Hidden -PassThru
  try {
    Assert (-not $running.HasExited) 'stand-in nefos-inbox.exe is running again'
    $waiting = Start-Process -FilePath $NewInstaller -ArgumentList '/S', '/WAIT=60' -PassThru
    $null = $waiting.Handle # 終了コードを取れるよう、ハンドルを保持する
    Assert (-not $waiting.WaitForExit(3000)) 'installer with /WAIT keeps waiting while nefos-inbox.exe is running'
    Assert ((Displayed-Version) -eq $OldVersion) 'the waiting installer has not changed the registration yet'
  } finally {
    Stop-Process -Id $running.Id -Force -ErrorAction SilentlyContinue
    $running.WaitForExit()
  }
  Assert ($waiting.WaitForExit(60000)) 'installer with /WAIT finishes after nefos-inbox.exe exits'
  Write-Host "installer with /WAIT exit code: $($waiting.ExitCode)"
  Assert ($waiting.ExitCode -eq 0) 'installer with /WAIT exits 0 after the wait'
  Assert ((Displayed-Version) -eq $NewVersion) "Apps & features shows $NewVersion after the waiting install"

  # 3. 上書き（新版。2b で入った同じ版の上に、もう一度入れる）。前回の場所に入る（/D= なしでも InstallLocation を使う）。
  # 2b のインストーラーが終わった直後は、書いたばかりの nefos-inbox.exe をまだ書き込みで開けないことがある（#385）。次のインストーラーが
  # 起動中と取り違えて終了コード 5 で止まらないよう、開けるようになるのを待ってから入れる（固定の sleep ではなく条件で待つ）。
  Assert (Wait-ExeReleased) 'nefos-inbox.exe is released after the waiting install'
  Assert ((Invoke-Installer $NewInstaller @('/S')) -eq 0) 'new installer exits 0'
  Assert ((Displayed-Version) -eq $NewVersion) "Apps & features shows $NewVersion after overwrite"
  Assert ((Get-FileHash "$installDir\nefos-inbox.exe").Hash -eq (Get-FileHash $Exe).Hash) 'nefos-inbox.exe was replaced by the new build'
  Assert ((Get-Item "$installDir\nefos-inbox.exe").VersionInfo.ProductVersion -eq $NewVersion) "nefos-inbox.exe file version is $NewVersion"
  Assert (Embeds-Version "$installDir\nefos-inbox.exe" $NewVersion) "nefos-inbox.exe embeds main.version $NewVersion"
  Assert (-not (Embeds-Version "$installDir\nefos-inbox.exe" $OldVersion)) "nefos-inbox.exe no longer embeds $OldVersion"
  Assert (-not (Embeds-Version $Exe $OldVersion)) 'the new build does not embed the old version'
  Assert (Embeds-Version $OldExe $OldVersion) 'the old build embeds the old version'

  # 4. アンインストール（NSIS のアンインストーラーは自分を一時フォルダへ複製して実行するため、完了を条件で待つ）
  Start-Process -FilePath "$installDir\uninstall.exe" -ArgumentList '/S' -Wait
  Assert (Wait-Until { -not (Test-Path $uninstKey) }) 'Apps & features entry is removed'
  Assert (Wait-Until { -not (Test-Path $installDir) }) 'install folder is removed'
  Assert (-not (Test-Path $startMenuLink)) 'Start Menu shortcut is removed'
  Assert (Test-Path $marker) 'user data (%APPDATA%\Nefos Inbox) is kept'
  Write-Host 'installer test passed'
} finally {
  # 消すのはこのスクリプトが作ったものだけ。
  Remove-Item $marker -Force -ErrorAction SilentlyContinue
  if (-not $dataDirExisted) { Remove-Item $dataDir -Recurse -Force -ErrorAction SilentlyContinue }
  Remove-LegacyLeftovers
  if (Test-Path $installDir) { Remove-Item $installDir -Recurse -Force -ErrorAction SilentlyContinue }
}
