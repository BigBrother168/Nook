; Nook 安装包的补充（mygo.json 的 windows.installerInclude），接在 MyGo
; 生成的脚本里 MUI2 和英文之后：
; - 中文界面：系统是中文就用中文，其他用英文。
; - 卸载：Nook 自己生成的门卫副本和预览数据删掉；设置、历史对话列表和
;   改动备份问一声再删（默认不删，静默卸载不删）。AI 自己存的对话记录、
;   项目里的文件一概不动。
; 这个文件要存成带 BOM 的 UTF-8，NSIS 才不会按系统代码页读错中文。
; - 装进了放着别的东西的文件夹（比如选了 D:\Tools，或者源码文件夹）：卸载只删
;   Nook 自己的文件，文件夹和别的东西都留着。MyGo 的卸载最后会删整个安装
;   文件夹，所以这时把 $INSTDIR 指到一个不存在的子文件夹，让那一步什么也不删。

!include "LogicLib.nsh"

!insertmacro MUI_LANGUAGE "SimpChinese"
LangString MYGO_DESKTOP_SHORTCUT ${LANG_SIMPCHINESE} "创建桌面快捷方式"

LangString NOOK_ASK_DATA ${LANG_ENGLISH} "Also delete Nook's settings, its list of past conversations and the backups of changes?$\r$\n$\r$\nThe AI tools' own conversation files and your projects are not touched."
LangString NOOK_ASK_DATA ${LANG_SIMPCHINESE} "同时删除 Nook 的设置、历史对话列表和改动备份吗？$\r$\n$\r$\nAI 自己存的对话记录和你的项目都不会动。"

!macro customUnInstall
  RMDir /r "$LOCALAPPDATA\Nook\shims"
  RMDir /r "$LOCALAPPDATA\Nook\WebView2"
  IfSilent nook_keep
  MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "$(NOOK_ASK_DATA)" IDNO nook_keep
  RMDir /r "$APPDATA\Nook"
  RMDir /r "$LOCALAPPDATA\Nook"
  nook_keep:
  RMDir "$LOCALAPPDATA\Nook"
  ; What else is in the folder: anything not Nook's keeps the folder.
  StrCpy $R1 0
  FindFirst $R0 $R2 "$INSTDIR\*"
  ${DoWhile} $R2 != ""
    ${If} $R2 != "."
    ${AndIf} $R2 != ".."
    ${AndIf} $R2 != "Nook.exe"
    ${AndIf} $R2 != "nook-guard.exe"
    ${AndIf} $R2 != "OpenConsole.exe"
    ${AndIf} $R2 != "conpty.dll"
    ${AndIf} $R2 != "ghostty-vt.dll"
    ${AndIf} $R2 != "icon.png"
    ${AndIf} $R2 != "LICENSE.txt"
    ${AndIf} $R2 != "THIRD-PARTY-NOTICES.txt"
    ${AndIf} $R2 != "conpty-NOTICE.txt"
    ${AndIf} $R2 != "Uninstall.exe"
    ${AndIf} $R2 != ".mygo-files"
      IntOp $R1 $R1 + 1
    ${EndIf}
    FindNext $R0 $R2
  ${Loop}
  FindClose $R0
  ${If} $R1 > 0
    Delete "$INSTDIR\Nook.exe"
    Delete "$INSTDIR\nook-guard.exe"
    Delete "$INSTDIR\OpenConsole.exe"
    Delete "$INSTDIR\conpty.dll"
    Delete "$INSTDIR\ghostty-vt.dll"
    Delete "$INSTDIR\icon.png"
    Delete "$INSTDIR\LICENSE.txt"
    Delete "$INSTDIR\THIRD-PARTY-NOTICES.txt"
    Delete "$INSTDIR\conpty-NOTICE.txt"
    Delete "$INSTDIR\Uninstall.exe"
    Delete "$INSTDIR\.mygo-files"
    ; The folder is the user's: the last step, which deletes the whole
    ; folder, finds nothing to delete.
    StrCpy $INSTDIR "$INSTDIR\.nook-not-here"
  ${EndIf}
!macroend
