package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func commandKey(m *model, key tea.KeyPressMsg) { m.Update(key) }

func TestCommandTabCyclesMatchesAndWraps(t *testing.T) {
	m := openDemo(t)
	runeKey(m, ":s")
	for _, expected := range []string{"servers", "send", "sync", "slash", "servers"} {
		commandKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
		if m.command.value() != expected {
			t.Fatalf("wanted %s, got %s", expected, m.command.value())
		}
	}
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.command.value() != "slash" {
		t.Fatal("reverse completion failed")
	}
}

func TestEmptyCommandTabCyclesAllCommands(t *testing.T) {
	m := openDemo(t)
	runeKey(m, ":")
	for _, command := range commands {
		commandKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
		if m.command.value() != command.name {
			t.Fatal("full command cycle failed")
		}
	}
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.command.value() != commands[0].name {
		t.Fatal("full cycle did not wrap")
	}
}

func TestEditingOrEscapingResetsCompletion(t *testing.T) {
	m := openDemo(t)
	runeKey(m, ":s")
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	for i := 0; i < len("servers"); i++ {
		commandKey(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	runeKey(m, "o")
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.command.value() != "older" {
		t.Fatal("editing did not reset candidates")
	}
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	runeKey(m, ":s")
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.command.value() != "servers" {
		t.Fatal("Esc did not reset candidates")
	}
}

func TestNoMatchAndSearchDoNotCompleteCommands(t *testing.T) {
	m := openDemo(t)
	runeKey(m, ":xyz")
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.command.value() != "xyz" {
		t.Fatal("unmatched input was replaced")
	}
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m.execute("channels")
	runeKey(m, "/s")
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.command.value() != "s" {
		t.Fatal("search used command completion")
	}
}

func TestQuestionHelpShowsCommandsAndPreservesChat(t *testing.T) {
	m := openDemo(t)
	m.draft.set("keep this draft")
	m.messageIndex = 0
	runeKey(m, ":?")
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.showHelp {
		t.Fatal("question command did not open help")
	}
	lines, _ := m.body()
	text := strings.Join(lines, "\n")
	for _, command := range commands {
		if !strings.Contains(text, ":"+command.name) {
			t.Fatalf("missing command %s", command.name)
		}
	}
	if !strings.Contains(m.View().Content, "사용 가능한 명령어") {
		t.Fatal("help is scrolled to bottom on opening")
	}
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.showHelp || m.screen != "chat" || m.draft.value() != "keep this draft" || m.messageIndex != 0 {
		t.Fatal("closing help changed chat state")
	}
	m.busy = "send"
	m.execute("help")
	if !m.showHelp {
		t.Fatal("help was blocked by a pending request")
	}
}

func TestCompletedCommandExecutes(t *testing.T) {
	m := openDemo(t)
	runeKey(m, ":ser")
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	commandKey(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.screen != "servers" || m.entry != "" {
		t.Fatal("completed command did not execute")
	}
}
