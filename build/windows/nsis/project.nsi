Unicode true

####
## Hitomado の Windows インストーラー（利用者ごとのインストール。管理者権限は要らない）。
##
## 必須の定義（makensis の -D で渡す。build/windows/Taskfile.yml の installer:nsis が渡す）:
##   VERSION   版番号 X.Y.Z（exe の -X main.version と同じ値）
##   ARG_EXE   同梱する hitomado.exe のパス
##   OUTFILE   出力するインストーラーのパス
##
## 直接ビルドする例（exe を先に作っておく）:
##   makensis -DVERSION=0.1.1 -DARG_EXE=..\..\..\bin\hitomado.exe -DOUTFILE=..\..\..\bin\setup.exe project.nsi
##
## 実行時の引数（アプリの自動アップデートが使う。docs/requirements/non-functional.md NF-D03）:
##   /S          サイレント（画面なし）
##   /WAIT=秒    サイレント実行のとき、起動中の hitomado.exe が終了するのを最大この秒数（上限 300）待ってから上書きする。
##               省略（または 0）なら待たず、起動中は終了コード 5 で中止する
##   /RELAUNCH   インストールが成功したら、インストール先の hitomado.exe を起動する
##   /D=パス     インストール先（最後の引数・引用符なし。省略すると前回の場所）
##
## 日本語を含むため、このファイルは BOM 付き UTF-8 で保存する（.editorconfig）。
####
!ifndef VERSION
    !error "VERSION is required (-DVERSION=X.Y.Z)"
!endif
!ifndef ARG_EXE
    !error "ARG_EXE is required (-DARG_EXE=path\to\hitomado.exe)"
!endif
!ifndef OUTFILE
    !error "OUTFILE is required (-DOUTFILE=path\to\setup.exe)"
!endif

!define APP_NAME     "Hitomado"
!define APP_EXE      "hitomado.exe"
!define COMPANY      "Ousias"
!define APP_URL      "https://github.com/ousiassllc/hitomado"
# 「アプリと機能」に出るキー。変えると別アプリ扱いになり、上書き更新できなくなる。
!define UNINST_KEY   "Software\Microsoft\Windows\CurrentVersion\Uninstall\jp.ousias.hitomado"
# WebView2 のデータ置き場（%APPDATA%\<exe 名>）。アプリのデータ（%APPDATA%\Hitomado）とは別物。
!define WEBVIEW_DATA "$APPDATA\${APP_EXE}"

!include "LogicLib.nsh"
!include "x64.nsh"
!include "WinVer.nsh"
!include "FileFunc.nsh"
!include "MUI2.nsh"

!insertmacro GetOptions

# 起動中の hitomado.exe の終了を待つ残り回数（0.5 秒単位。/WAIT=秒 から。サイレント実行のときだけ使う）と、
# インストール後に起動し直すか（/RELAUNCH）。
Var WaitLeft
Var Relaunch

Name "${APP_NAME} ${VERSION}"
OutFile "${OUTFILE}"
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\Programs\${APP_NAME}"
# 上書きインストールでは前回の場所を使う。
InstallDirRegKey HKCU "${UNINST_KEY}" "InstallLocation"
ShowInstDetails show
ShowUninstDetails show
ManifestDPIAware true

# 4 桁必須。
VIProductVersion "${VERSION}.0"
VIFileVersion    "${VERSION}.0"
VIAddVersionKey "CompanyName"     "${COMPANY}"
VIAddVersionKey "FileDescription" "${APP_NAME} Installer"
VIAddVersionKey "ProductVersion"  "${VERSION}"
VIAddVersionKey "FileVersion"     "${VERSION}"
VIAddVersionKey "LegalCopyright"  "Hitomado contributors, AGPL-3.0"
VIAddVersionKey "ProductName"     "${APP_NAME}"

!define MUI_ICON   "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "${APP_NAME} を起動する"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "Japanese"

# 起動中の hitomado.exe は上書き・削除できない。実行中の exe は書き込みで開けないことを使って検出し、
# 画面ありなら終了を促して再試行、サイレント（/S）なら /WAIT=秒 の間 0.5 秒ごとに再確認し、それでも開けなければエラーコード 5 で中止する。
!macro DEFINE_CHECK_NOT_RUNNING PREFIX
Function ${PREFIX}CheckNotRunning
    check:
    IfFileExists "$INSTDIR\${APP_EXE}" 0 done
    ClearErrors
    FileOpen $0 "$INSTDIR\${APP_EXE}" a
    IfErrors locked
    FileClose $0
    Goto done
    locked:
    IfSilent silent
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "${APP_NAME} が起動中です。$\r$\nタスクトレイの ${APP_NAME} のアイコンを右クリックして「終了」を選んでから、「再試行」を押してください。" IDRETRY check
    Abort
    silent:
    ${If} $WaitLeft > 0
        IntOp $WaitLeft $WaitLeft - 1
        Sleep 500
        Goto check
    ${EndIf}
    SetErrorLevel 5
    Abort
    done:
FunctionEnd
!macroend
!insertmacro DEFINE_CHECK_NOT_RUNNING ""
!insertmacro DEFINE_CHECK_NOT_RUNNING "un."

Function .onInit
    StrCpy $WaitLeft 0
    StrCpy $Relaunch 0
    ClearErrors
    ${GetOptions} $CMDLINE "/WAIT=" $0
    ${IfNot} ${Errors}
        # 数字以外は 0 秒として扱う（IntOp は数字でない文字列を 0 とみなす）。
        IntOp $WaitLeft $0 * 2
        ${If} $WaitLeft > 600
            StrCpy $WaitLeft 600
        ${ElseIf} $WaitLeft < 0
            StrCpy $WaitLeft 0
        ${EndIf}
    ${EndIf}
    ClearErrors
    ${GetOptions} $CMDLINE "/RELAUNCH" $0
    ${IfNot} ${Errors}
        StrCpy $Relaunch 1
    ${EndIf}
    ClearErrors

    # 対応は x64 の Windows 10 以降と、ARM64 の Windows 11（x64 エミュレーションで x64 版の exe を動かす。ARM64 版のビルドは無い）。
    # ARM64 の Windows 10 は x64 エミュレーションが無い（x86 のみ）ので対象外。Windows 11 は build 22000 以降。
    StrCpy $0 1
    ${If} ${IsNativeAMD64}
        ${IfNot} ${AtLeastWin10}
            StrCpy $0 0
        ${EndIf}
    ${ElseIf} ${IsNativeARM64}
        ${IfNot} ${AtLeastBuild} 22000
            StrCpy $0 0
        ${EndIf}
    ${Else}
        StrCpy $0 0
    ${EndIf}
    ${If} $0 == 0
        IfSilent silentUnsupported
        MessageBox MB_OK|MB_ICONSTOP "${APP_NAME} は 64 bit 版の Windows 10 以降（ARM64 は Windows 11 のみ）にのみインストールできます。"
        Quit
        silentUnsupported:
        SetErrorLevel 64
        Abort
    ${EndIf}
FunctionEnd

Function un.onInit
    StrCpy $WaitLeft 0
FunctionEnd

# 自動アップデートの「再起動して更新」（/RELAUNCH）。インストールが成功したときだけ、新しい hitomado.exe を起動する。
Function .onInstSuccess
    ${If} $Relaunch == 1
        Exec '"$INSTDIR\${APP_EXE}"'
    ${EndIf}
FunctionEnd

Section "${APP_NAME}（必須）"
    SectionIn RO
    SetShellVarContext current
    SetRegView 64

    Call CheckNotRunning

    # WebView2 ランタイムが無ければ入れる（Windows 11 と最近の Windows 10 には入っている）。
    ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
    ${If} $0 == ""
        ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
    ${EndIf}
    ${If} $0 == ""
        DetailPrint "WebView2 ランタイムをインストールしています"
        InitPluginsDir
        SetOutPath "$PLUGINSDIR"
        File "MicrosoftEdgeWebview2Setup.exe"
        ExecWait '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install'
    ${EndIf}

    SetOutPath "$INSTDIR"
    File "/oname=${APP_EXE}" "${ARG_EXE}"
    # 依存ライセンスの表記（リポジトリ直下の NOTICE。docs/environment/setup.md §4.3）。
    File "/oname=NOTICE.txt" "..\..\..\NOTICE"
    WriteUninstaller "$INSTDIR\uninstall.exe"

    CreateShortcut "$SMPROGRAMS\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"

    # 「アプリと機能」への登録。
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${APP_NAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
    WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "${COMPANY}"
    WriteRegStr HKCU "${UNINST_KEY}" "URLInfoAbout" "${APP_URL}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${APP_EXE}"
    WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
    WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
    WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
    WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" $0
SectionEnd

Section /o "デスクトップにショートカットを作る"
    SetShellVarContext current
    CreateShortcut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"
SectionEnd

# アンインストールで消すのはプログラム本体・ショートカット・登録（通知用の登録を含む）だけ。利用者のデータ
# （%APPDATA%\Hitomado の DB・設定・ログ、資格情報マネージャーの秘密）は消さない（docs/environment/setup.md §4.3）。
Section "Uninstall"
    SetShellVarContext current
    SetRegView 64

    Call un.CheckNotRunning

    Delete "$INSTDIR\${APP_EXE}"
    Delete "$INSTDIR\NOTICE.txt"
    Delete "$INSTDIR\uninstall.exe"
    # 再帰削除はしない（利用者が選んだフォルダに他のファイルがあっても消さない）。空のときだけ消える。
    RMDir "$INSTDIR"

    Delete "$SMPROGRAMS\${APP_NAME}.lnk"
    Delete "$DESKTOP\${APP_NAME}.lnk"

    RMDir /r "${WEBVIEW_DATA}"

    # アプリ（Wails の通知サービス）が起動時に HKCU へ書く通知用の登録。次の起動で作り直されるので、残さない。
    # CLSID の GUID は AppUserModelId\Hitomado の CustomActivator に入っている。
    ReadRegStr $0 HKCU "Software\Classes\AppUserModelId\Hitomado" "CustomActivator"
    ${If} $0 != ""
        DeleteRegKey HKCU "Software\Classes\CLSID\$0"
    ${EndIf}
    DeleteRegKey HKCU "Software\Classes\AppUserModelId\Hitomado"
    DeleteRegKey HKCU "Software\Hitomado\NotificationCategories"
    DeleteRegKey /ifempty HKCU "Software\Hitomado"

    DeleteRegKey HKCU "${UNINST_KEY}"
SectionEnd
