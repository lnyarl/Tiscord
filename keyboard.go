package main

import (
	"unicode"

	tea "charm.land/bubbletea/v2"
)

func keyName(key tea.KeyPressMsg) string {
	if key.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) != 0 {
		return key.Keystroke()
	}
	return key.String()
}

func normalKeyName(key tea.KeyPressMsg) string {
	if key.BaseCode == 0 {
		return keyName(key)
	}
	physical := key.Key()
	physical.Code = key.BaseCode
	physical.Text = ""
	physical.ShiftedCode = 0
	if key.Mod.Contains(tea.ModShift) {
		if physical.Code >= 'a' && physical.Code <= 'z' {
			physical.Code = unicode.ToUpper(physical.Code)
			physical.Mod &^= tea.ModShift
		} else {
			const unshifted = "`1234567890-=[]\\;',./"
			const shifted = "~!@#$%^&*()_+{}|:\"<>?"
			for i, r := range unshifted {
				if physical.Code == r {
					physical.Code = rune(shifted[i])
					physical.Mod &^= tea.ModShift
					break
				}
			}
		}
	}
	// Key names normally use Text for printable characters. Keep the physical
	// character here, never the IME's composed text.
	if physical.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0 && physical.Code >= ' ' && physical.Code <= '~' {
		physical.Text = string(physical.Code)
	}
	return keyName(tea.KeyPressMsg(physical))
}

// Two-set Korean input can reach a terminal as jamo or a composed syllable,
// without a usable physical key. Decode only Normal-mode shortcut input.
func koreanKey(r rune) (string, bool) {
	initial := [...]string{"r", "R", "s", "e", "E", "f", "a", "q", "Q", "t", "T", "d", "w", "W", "c", "z", "x", "v", "g"}
	vowel := [...]string{"k", "o", "i", "O", "j", "p", "u", "P", "h", "hk", "ho", "hl", "y", "n", "nj", "np", "nl", "b", "m", "ml", "l"}
	final := [...]string{"", "r", "R", "rt", "s", "sw", "sg", "e", "f", "fr", "fa", "fq", "ft", "fx", "fv", "fg", "a", "q", "qt", "t", "T", "d", "w", "c", "z", "x", "v", "g"}
	consonant := [...]string{"r", "R", "rt", "s", "sw", "sg", "e", "E", "f", "fr", "fa", "fq", "ft", "fx", "fv", "fg", "a", "q", "Q", "qt", "t", "T", "d", "w", "W", "c", "z", "x", "v", "g"}
	switch {
	case r >= 0xAC00 && r <= 0xD7A3:
		n := int(r - 0xAC00)
		return initial[n/(21*28)] + vowel[n/28%21] + final[n%28], true
	case r >= 0x3131 && r <= 0x314E:
		return consonant[r-0x3131], true
	case r >= 0x314F && r <= 0x3163:
		return vowel[r-0x314F], true
	case r >= 0x1100 && r <= 0x1112:
		return initial[r-0x1100], true
	case r >= 0x1161 && r <= 0x1175:
		return vowel[r-0x1161], true
	case r >= 0x11A8 && r <= 0x11C2:
		return final[r-0x11A7], true
	}
	return string(r), false
}

func (m *model) koreanNormalKey(key tea.KeyPressMsg) (tea.Cmd, bool) {
	if !m.config.Behavior.KoreanShortcuts || m.mode != "NORMAL" || m.entry != "" || key.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) != 0 || key.BaseCode >= ' ' && key.BaseCode <= '~' {
		return nil, false
	}
	text := key.Text
	if text == "" {
		text = string(key.Code)
	}
	runes := []rune(text)
	hangul := false
	for _, r := range runes {
		if _, ok := koreanKey(r); ok {
			hangul = true
			break
		}
	}
	if !hangul {
		return nil, false
	}
	appLog.Debug("keyboard.korean_fallback", "characters", len(runes), "has_base_code", key.BaseCode != 0)
	for i, r := range runes {
		keys, _ := koreanKey(r)
		if len(keys) == 1 && key.Mod.Contains(tea.ModShift) {
			keys = string(unicode.ToUpper(rune(keys[0])))
		}
		for _, code := range keys {
			if command := m.key(tea.KeyPressMsg{Code: code, Text: string(code)}); command != nil {
				return command, true
			}
			if m.mode != "NORMAL" || m.entry != "" {
				// Text after the mode-changing jamo is still IME text, not shortcuts.
				if m.mode == "INSERT" && !m.showHelp {
					m.draft.insert([]rune(clean(string(runes[i+1:]))))
				}
				return nil, true
			}
		}
	}
	return nil, true
}
