# Windows でのエージェントのマスコット（#372）の動作確認（CI の mascot-windows ジョブで実行する）。
#
#   E2E ビルドの exe を、エージェントとマスコットがオンの状態で起動する
#   → メインウィンドウとマスコットの別ウィンドウが出る
#   → メインウィンドウを閉じる（WM_CLOSE。× と同じ）→ メインは隠れるが、アプリは終了せず、マスコットは残る
#
# 確認すること:
#   - マスコットは、枠なしの別の最上位ウィンドウ（72×72）で、常に最前面（WS_EX_TOPMOST）・タスクバーに出ない（WS_EX_TOOLWINDOW）
#   - メインウィンドウを閉じたあとも、プロセスは生きていて、マスコットのウィンドウは見えている
#   - 閉じる前後の画面のスクリーンショットと、ウィンドウの一覧（JSON）を OutDir に残す
#
# 使い方: pwsh .github/scripts/test-mascot.ps1 -Exe bin\hitomado-e2e.exe -OutDir mascot-evidence
param(
  [Parameter(Mandatory)][string]$Exe,
  [Parameter(Mandatory)][string]$OutDir
)
$ErrorActionPreference = 'Stop'
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;

public class WinInfo {
  public long Handle; public string Title; public bool Visible;
  public int Left, Top, Width, Height; public long ExStyle;
}

public static class Win {
  delegate bool EnumProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc p, IntPtr l);
  [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowText(IntPtr h, StringBuilder s, int n);
  [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", EntryPoint = "GetWindowLongPtrW")] static extern IntPtr GetWindowLongPtr(IntPtr h, int i);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [StructLayout(LayoutKind.Sequential)] struct RECT { public int L, T, R, B; }

  public static List<WinInfo> OfProcess(uint pid) {
    var list = new List<WinInfo>();
    EnumWindows((h, l) => {
      uint p; GetWindowThreadProcessId(h, out p);
      if (p != pid) return true;
      var sb = new StringBuilder(256); GetWindowText(h, sb, sb.Capacity);
      RECT r; GetWindowRect(h, out r);
      list.Add(new WinInfo {
        Handle = h.ToInt64(), Title = sb.ToString(), Visible = IsWindowVisible(h),
        Left = r.L, Top = r.T, Width = r.R - r.L, Height = r.B - r.T,
        ExStyle = GetWindowLongPtr(h, -20).ToInt64()
      });
      return true;
    }, IntPtr.Zero);
    return list;
  }
}
'@

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

function Shot([string]$Name) {
  $b = [System.Windows.Forms.SystemInformation]::VirtualScreen
  $bmp = New-Object System.Drawing.Bitmap $b.Width, $b.Height
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($b.X, $b.Y, 0, 0, $bmp.Size)
  $bmp.Save((Join-Path $OutDir $Name), [System.Drawing.Imaging.ImageFormat]::Png)
  $g.Dispose(); $bmp.Dispose()
}

# マスコットのタイトルは「Hitomado エージェント」、メインは「Hitomado」ちょうど。
function Get-Main([uint32]$ProcId) { [Win]::OfProcess($ProcId) | Where-Object { $_.Title -eq 'Hitomado' } | Select-Object -First 1 }
function Get-Mascot([uint32]$ProcId) { [Win]::OfProcess($ProcId) | Where-Object { $_.Title -like 'Hitomado *' } | Select-Object -First 1 }
function Dump([string]$Name, [uint32]$ProcId) {
  [Win]::OfProcess($ProcId) | Where-Object { $_.Title } | ConvertTo-Json | Set-Content -Encoding utf8 (Join-Path $OutDir $Name)
}

$WS_EX_TOPMOST = 0x8
$WS_EX_TOOLWINDOW = 0x80
$WM_CLOSE = 0x0010

$env:HITOMADO_E2E_AGENT_MASCOT = '1'
$p = Start-Process -FilePath $Exe -PassThru
try {
  $procId = [uint32]$p.Id
  $up = Wait-Until { (Get-Main $procId).Visible -and (Get-Mascot $procId).Visible } 90
  Dump 'windows-before.json' $procId
  Shot '1-before-close.png'
  Assert $up 'the main window and the mascot window are both visible after startup'

  $mascot = Get-Mascot $procId
  Write-Host "mascot: $($mascot.Width)x$($mascot.Height) at ($($mascot.Left),$($mascot.Top)) exstyle=0x$('{0:x}' -f $mascot.ExStyle)"
  Assert (($mascot.ExStyle -band $WS_EX_TOPMOST) -ne 0) 'the mascot is always on top (WS_EX_TOPMOST)'
  Assert (($mascot.ExStyle -band $WS_EX_TOOLWINDOW) -ne 0) 'the mascot is hidden from the taskbar and Alt+Tab (WS_EX_TOOLWINDOW)'

  # メインウィンドウの × と同じ WM_CLOSE。アプリは終了せず、トレイへ隠れるだけのはず。
  $main = Get-Main $procId
  Assert ([Win]::PostMessage([IntPtr]$main.Handle, $WM_CLOSE, [IntPtr]::Zero, [IntPtr]::Zero)) 'posted WM_CLOSE to the main window'
  $hidden = Wait-Until { -not (Get-Main $procId).Visible } 15
  Assert $hidden 'the main window is hidden after it is closed'
  Assert (-not $p.HasExited) 'the app is still running after the main window is closed'
  Assert ((Get-Mascot $procId).Visible) 'the mascot stays visible after the main window is closed'
  # 隠れたあと少し置いても、マスコットは消えない（終了処理が遅れて走るのを拾う）。
  $gone = Wait-Until { $p.HasExited -or -not (Get-Mascot $procId).Visible } 5
  Assert (-not $gone) 'the mascot is still there 5 seconds after the main window was closed'
  Dump 'windows-after.json' $procId
  Shot '2-after-close.png'
} finally {
  if (-not $p.HasExited) { $p.Kill() }
}
