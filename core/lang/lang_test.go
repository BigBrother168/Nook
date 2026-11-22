package lang

import "testing"

func TestSet(t *testing.T) {
	defer func() { English = false }()
	for _, c := range []struct {
		setting   string
		preferred []string
		english   bool
	}{
		{"", []string{"zh-CN", "en-US"}, false},
		{"", []string{"en-US", "zh-CN"}, true},
		{"", nil, false},
		{"en", []string{"zh-CN"}, true},
		{"zh", []string{"en-US"}, false},
	} {
		Set(c.setting, c.preferred)
		if English != c.english {
			t.Errorf("Set(%q, %q): English = %v", c.setting, c.preferred, English)
		}
	}
	English = true
	if got := (Error{"错", "wrong"}).Error(); got != "wrong" {
		t.Errorf("Error = %q", got)
	}
}
