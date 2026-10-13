package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

func TestEnterSendsAndModifiedEnterInsertsNewline(t *testing.T) {
	for _, mod := range []tea.KeyMod{tea.ModCtrl, tea.ModShift} {
		m := openDemo(t)
		m.mode = "INSERT"
		m.draft.set("onetwo")
		m.draft.cursor = 3
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "\r", Mod: mod})
		if cmd != nil || m.draft.value() != "one\ntwo" || len(m.messages) != 1 {
			t.Fatal("modified Enter sent instead of inserting a newline")
		}
		_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("Enter did not send by default")
		}
		runCommand(m, cmd)
		if len(m.messages) != 2 || m.messages[1].Content != "one\ntwo" || m.draft.value() != "" {
			t.Fatal("Enter send lost multiline content")
		}
	}
}

func TestTerminalDecodesModifiedEnterAsNewline(t *testing.T) {
	for _, sequence := range []string{"\x1b[13;2u", "\x1b[13;5u", "\x1b[27;2;13~", "\x1b[27;5;13~"} {
		decoder := uv.EventDecoder{}
		consumed, event := decoder.Decode([]byte(sequence))
		key, ok := event.(uv.KeyPressEvent)
		if !ok || consumed != len(sequence) {
			t.Fatalf("modified Enter not decoded: %q", sequence)
		}
		m := openDemo(t)
		m.mode = "INSERT"
		m.draft.set("draft")
		_, cmd := m.Update(tea.KeyPressMsg(key))
		if cmd != nil || m.draft.value() != "draft\n" {
			t.Fatalf("terminal newline key sent a message: %q", sequence)
		}
	}
}

func TestExplicitLegacySendKeyStillWorks(t *testing.T) {
	m := openDemo(t)
	m.config.Behavior.SendKey = "ctrl+s"
	m.mode = "INSERT"
	m.draft.set("first")
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.draft.value() != "first\n" {
		t.Fatal("explicit legacy setting was ignored")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("legacy send shortcut not available")
	}
	runCommand(m, cmd)
	if m.messages[1].Content != "first\n" {
		t.Fatal("legacy send changed content")
	}
}

func TestPasteAndBusySendDoNotTriggerAdditionalMessages(t *testing.T) {
	m := openDemo(t)
	m.mode = "INSERT"
	m.Update(tea.PasteMsg{Content: "first\nsecond\n"})
	if len(m.messages) != 1 || m.draft.value() != "first\nsecond\n" {
		t.Fatal("multiline paste sent a message")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter did not send pasted draft")
	}
	_, duplicate := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if duplicate != nil {
		t.Fatal("Enter repeated pending send")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if m.draft.value() != "first\nsecond\n" {
		t.Fatal("newline altered a pending send")
	}
}
