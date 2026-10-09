package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

func TestNormalUsesBaseKeyWhileInsertKeepsKoreanText(t *testing.T) {
	m := openDemo(t)
	m.messages = append(m.messages, message{ID: "2", ChannelID: "10"})
	m.messageIndex = 0
	m.Update(tea.KeyPressMsg{Code: 'ㅓ', Text: "ㅓ", BaseCode: 'j'})
	if m.messageIndex != 1 || m.draft.value() != "" {
		t.Fatal("Normal did not use physical j")
	}
	m.Update(tea.KeyPressMsg{Code: 'ㅏ', Text: "ㅏ", BaseCode: 'k'})
	if m.messageIndex != 0 {
		t.Fatal("Normal did not use physical k")
	}
	m.Update(tea.KeyPressMsg{Code: 'ㅑ', Text: "ㅑ", BaseCode: 'i'})
	if m.mode != "INSERT" || m.draft.value() != "" {
		t.Fatal("physical i did not enter Insert cleanly")
	}
	m.Update(tea.KeyPressMsg{Code: 'ㅇ', Text: "안녕하세요", BaseCode: 'd'})
	if m.draft.value() != "안녕하세요" {
		t.Fatal("Insert lost Korean composition")
	}
	m.Update(tea.KeyReleaseMsg{Code: 'd', BaseCode: 'd'})
	if m.draft.value() != "안녕하세요" {
		t.Fatal("release duplicated text")
	}
}

func TestPhysicalShiftAndModifiers(t *testing.T) {
	cases := []struct {
		key  tea.KeyPressMsg
		name string
	}{
		{tea.KeyPressMsg{Code: 'ㅎ', Text: "ㅎ", BaseCode: 'g', Mod: tea.ModShift}, "G"},
		{tea.KeyPressMsg{BaseCode: ';', Mod: tea.ModShift}, ":"},
		{tea.KeyPressMsg{BaseCode: '/', Mod: tea.ModShift}, "?"},
		{tea.KeyPressMsg{BaseCode: '\\', Mod: tea.ModShift}, "|"},
		{tea.KeyPressMsg{BaseCode: 's', Text: "ㄴ", Mod: tea.ModCtrl}, "ctrl+s"},
		{tea.KeyPressMsg{BaseCode: tea.KeyTab, Mod: tea.ModShift}, "shift+tab"},
		{tea.KeyPressMsg{Code: 'j', Text: "j"}, "j"},
		{tea.KeyPressMsg{Code: 'ㅓ', Text: "ㅓ"}, "ㅓ"},
	}
	for _, tc := range cases {
		if got := normalKeyName(tc.key); got != tc.name {
			t.Errorf("wanted %q got %q", tc.name, got)
		}
	}
}

func TestPhysicalPunctuationOpensCommandEntry(t *testing.T) {
	m := openDemo(t)
	m.Update(tea.KeyPressMsg{BaseCode: ';', Mod: tea.ModShift})
	if m.entry != "command" {
		t.Fatal("physical colon did not open command input")
	}
}

func TestPasteIsTextAndNeverExecutesNormalKeys(t *testing.T) {
	m := openDemo(t)
	m.Update(tea.PasteMsg{Content: "i:send"})
	if m.mode != "NORMAL" || m.entry != "" || m.draft.value() != "" {
		t.Fatal("paste executed shortcuts")
	}
	m.mode = "INSERT"
	m.Update(tea.PasteMsg{Content: "한글\n두 번째 줄"})
	if m.draft.value() != "한글\n두 번째 줄" {
		t.Fatal("multiline paste was not preserved")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	runeKey(m, ":")
	m.Update(tea.PasteMsg{Content: "help\n"})
	if m.showHelp {
		t.Fatal("paste executed a command")
	}
}

func TestEditorModifiedTextDoesNotInsertCharacters(t *testing.T) {
	e := editor{}
	e.set("abc")
	e.key(tea.KeyPressMsg{Code: 'h', Text: "h", Mod: tea.ModCtrl}, true)
	if e.value() != "ab" {
		t.Fatal("Ctrl-H text was inserted instead of deleting")
	}
}

func TestViewRequestsPhysicalKeysAndAssociatedText(t *testing.T) {
	m := openDemo(t)
	view := m.View()
	if !view.AltScreen || !view.KeyboardEnhancements.ReportAlternateKeys || !view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes || !view.KeyboardEnhancements.ReportAssociatedText {
		t.Fatal("keyboard enhancements were not requested")
	}
}

func TestTerminalProtocolPreservesPhysicalJAlongsideKorean(t *testing.T) {
	for _, sequence := range []string{"\x1b[12627::106;1u", "\x1b[74;36;12627;1;0;1_"} {
		decoder := uv.EventDecoder{}
		consumed, event := decoder.Decode([]byte(sequence))
		key, ok := event.(uv.KeyPressEvent)
		if !ok || consumed != len(sequence) || key.BaseCode != 'j' {
			t.Fatalf("physical key missing: consumed=%d event=%#v", consumed, event)
		}
		m := openDemo(t)
		m.messages = append(m.messages, message{ID: "2"})
		m.messageIndex = 0
		m.Update(tea.KeyPressMsg(key))
		if m.messageIndex != 1 {
			t.Fatal("decoded physical key did not scroll")
		}
	}
}

func TestKoreanNormalFallbackWithoutPhysicalKey(t *testing.T) {
	m := openDemo(t)
	m.messages = append(m.messages, message{ID: "2"}, message{ID: "3"})
	m.messageIndex = 0
	m.Update(tea.KeyPressMsg{Code: 'ㅓ', Text: "ㅓ"})
	if m.messageIndex != 1 {
		t.Fatal("Korean j fallback did not scroll")
	}
	m.Update(tea.KeyPressMsg{Code: 'ㅏ', Text: "ㅏ", BaseCode: 'ㅏ'})
	if m.messageIndex != 0 {
		t.Fatal("non-ASCII base key prevented fallback")
	}
	m.messageIndex = 2
	m.Update(tea.KeyPressMsg{Text: "ㅎㅎ"})
	if m.messageIndex != 0 {
		t.Fatal("batched gg was not handled")
	}
	m.Update(tea.KeyPressMsg{Code: 'ㅑ', Text: "ㅑ"})
	if m.mode != "INSERT" || m.draft.value() != "" {
		t.Fatal("Korean i fallback failed")
	}
	m.Update(tea.KeyPressMsg{Text: "한글 입력"})
	if m.draft.value() != "한글 입력" {
		t.Fatal("Insert text was converted")
	}
}

func TestKoreanFallbackCanBeDisabledAndKeepsSearchText(t *testing.T) {
	m := openDemo(t)
	m.config.Behavior.KoreanShortcuts = false
	m.Update(tea.KeyPressMsg{Code: 'ㅑ', Text: "ㅑ"})
	if m.mode != "NORMAL" {
		t.Fatal("disabled fallback still entered Insert")
	}
	m.config.Behavior.KoreanShortcuts = true
	m.execute("channels")
	runeKey(m, "/")
	m.Update(tea.KeyPressMsg{Text: "한글"})
	if m.command.value() != "한글" {
		t.Fatal("search text was converted")
	}
}

func TestKoreanFallbackUsesPhysicalKeyFirstAndPreservesTextAfterInsert(t *testing.T) {
	m := openDemo(t)
	m.Update(tea.KeyPressMsg{Code: 'ㅑ', Text: "ㅑ", BaseCode: 'j'})
	if m.mode != "NORMAL" {
		t.Fatal("fallback overrode physical key")
	}
	m.Update(tea.KeyPressMsg{Text: "ㅑ안녕하세요"})
	if m.mode != "INSERT" || m.draft.value() != "안녕하세요" {
		t.Fatal("text after Insert shortcut was converted")
	}
}

func TestKoreanSyllableAndJamoKeyboardDecomposition(t *testing.T) {
	for text, expected := range map[string]string{"가": "rk", "값": "rkqt", "한": "gks", "글": "rmf", "ㅘ": "hk", "ᄒ": "g", "ᅡ": "k", "ᆫ": "s", "ㄲ": "R", "ㅓ": "j", "ㅑ": "i"} {
		got := ""
		for _, r := range text {
			keys, ok := koreanKey(r)
			if !ok {
				t.Fatal("Hangul not recognized")
			}
			got += keys
		}
		if got != expected {
			t.Errorf("%s: got %s want %s", text, got, expected)
		}
	}
}
