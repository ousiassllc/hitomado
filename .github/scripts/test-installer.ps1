# Windows インストーラーの動作確認（CI の build ジョブで実行する。ローカルでは Windows のみ）。
#
#   旧版を入れる → 起動中は上書きを拒否する → 新版で上書きする → アンインストールする
#
# 確認すること:
#   - スタートメニューのショートカットと「アプリと機能」の登録（DisplayVersion）ができる
#   - 起動中（実行中の exe がある）なら、サイレントインストールは何も変えずに非 0 で終わる
#   - 上書きで版が上がり、exe が新しいものに置き換わり、exe のファイルバージョンが版と一致する
#   - アンインストールでプログラム・ショートカット・登録は消え、利用者のデータ（%APPDATA%\Hitomado）は残る
#
# 使い方:
#   pwsh .github/scripts/test-installer.ps1 -OldInstaller old.exe -OldVersion 0.0.0 `
#        -NewInstaller new.exe -NewVersion 0.0.1 -Exe bin\hitomado.exe
param(
  [Parameter(Mandatory)][string]$OldInstaller,
  [Parameter(Mandatory)][string]$OldVersion,
  [Parameter(Mandatory)][string]$NewInstaller,
  [Parameter(Mandatory)][string]$NewVersion,
  # NewInstaller に同梱した hitomado.exe（置き換わったことの照合用）
  [Parameter(Mandatory)][string]$Exe
)
$ErrorActionPreference = 'Stop'

$uninstKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\jp.ousias.hitomado'
$installDir = Join-Path ([IO.Path]::GetTempPath()) 'hitomado-installer-test'
$startMenuLink = Join-Path ([Environment]::GetFolderPath('Programs')) 'Hitomado.lnk'
$desktopLink = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Hitomado.lnk'
$dataDir = Join-Path $env:APPDATA 'Hitomado'
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

# NSIS の /D= は最後の引数で、引用符なし。
function Install([string]$Installer) {
  (Start-Process -FilePath $Installer -ArgumentList '/S', "/D=$installDir" -Wait -PassThru).ExitCode
}

function Displayed-Version { (Get-ItemProperty -Path $uninstKey -ErrorAction Stop).DisplayVersion }

if (Test-Path $installDir) { Remove-Item $installDir -Recurse -Force }
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
Set-Content -Path $marker -Value 'user data must survive uninstall'

# 1. 新規インストール（旧版）
Assert ((Install $OldInstaller) -eq 0) 'old installer exits 0'
Assert (Test-Path "$installDir\hitomado.exe") 'hitomado.exe is installed'
Assert (Test-Path "$installDir\uninstall.exe") 'uninstall.exe is installed'
Assert (Test-Path $startMenuLink) 'Start Menu shortcut exists'
Assert ((Displayed-Version) -eq $OldVersion) "Apps & features shows $OldVersion"
Assert ((Get-ItemProperty $uninstKey).DisplayName -eq 'Hitomado') 'Apps & features shows the name'
Assert ((Get-ItemProperty $uninstKey).InstallLocation -eq $installDir) 'InstallLocation is recorded'
Assert (-not (Test-Path $desktopLink)) 'desktop shortcut is optional (not created silently)'

# 2. 起動中は上書きしない（実行中の exe は書き込みで開けない。ping.exe を hitomado.exe の名前で動かして代わりにする）
Copy-Item "$env:SystemRoot\System32\PING.EXE" "$installDir\hitomado.exe" -Force
$running = Start-Process -FilePath "$installDir\hitomado.exe" -ArgumentList '-t', '127.0.0.1' -WindowStyle Hidden -PassThru
try {
  $code = Install $NewInstaller
  Write-Host "installer exit code while running: $code"
  Assert ($code -ne 0) 'installer refuses to overwrite a running hitomado.exe'
  Assert ((Displayed-Version) -eq $OldVersion) 'a refused install leaves the registration unchanged'
} finally {
  Stop-Process -Id $running.Id -Force -ErrorAction SilentlyContinue
  $running.WaitForExit()
}

# 3. 上書き（新版）。前回の場所に入る（/D= なしでも InstallLocation を使う）。
Assert ((Start-Process -FilePath $NewInstaller -ArgumentList '/S' -Wait -PassThru).ExitCode -eq 0) 'new installer exits 0'
Assert ((Displayed-Version) -eq $NewVersion) "Apps & features shows $NewVersion after overwrite"
Assert ((Get-FileHash "$installDir\hitomado.exe").Hash -eq (Get-FileHash $Exe).Hash) 'hitomado.exe was replaced by the new build'
Assert ((Get-Item "$installDir\hitomado.exe").VersionInfo.ProductVersion -eq $NewVersion) "hitomado.exe file version is $NewVersion"

# 4. アンインストール（NSIS のアンインストーラーは自分を一時フォルダへ複製して実行するため、完了を条件で待つ）
Start-Process -FilePath "$installDir\uninstall.exe" -ArgumentList '/S' -Wait
Assert (Wait-Until { -not (Test-Path $uninstKey) }) 'Apps & features entry is removed'
Assert (Wait-Until { -not (Test-Path $installDir) }) 'install folder is removed'
Assert (-not (Test-Path $startMenuLink)) 'Start Menu shortcut is removed'
Assert (Test-Path $marker) 'user data (%APPDATA%\Hitomado) is kept'

Remove-Item $dataDir -Recurse -Force
Write-Host 'installer test passed'
