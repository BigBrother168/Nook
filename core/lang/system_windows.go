package lang

import "golang.org/x/sys/windows"

// System returns the languages of the user's Windows, the first first, as
// "zh-CN".
func System() []string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil {
		return nil
	}
	return langs
}
