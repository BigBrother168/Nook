; Nook 安装包的补充（mygo.json 的 windows.installerInclude），接在 MyGo
; 生成的脚本里 MUI2 和英文之后：
; - 中文界面：系统是中文就用中文，其他用英文。
; - 卸载：Nook 自己生成的门卫副本和预览数据删掉；设置、历史对话列表和
;   改动备份问一声再删（默认不删，静默卸载不删）。AI 自己存的对话记录、
;   项目里的文件一概不动。
; 这个文件要存成带 BOM 的 UTF-8，NSIS 才不会按系统代码页读错中文。

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
!macroend
