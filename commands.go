package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

var commands = []struct{ name, description string }{
	{"?", "Show available commands and descriptions"},
	{"help", "Show available commands (same as :?)"},
	{"channels", "Open the selected server's channels or the current DM list"},
	{"servers", "Open the server list"},
	{"dm", "Open DM and group DM list · Enter to open a conversation"},
	{"send", "Send the draft (chat screen)"},
	{"older", "Load older messages (chat screen)"},
	{"sync", "Resync current channel history (connected chat screen)"},
	{"layout", "Cycle plain/log/editor or use :layout style (saved automatically)"},
	{"config", "Open settings · j/k move · h/l change · Ctrl-S save · Esc cancel"},
	{"slash", "Open Discord slash command helper (same as / in chat)"},
	{"q", "Quit"},
	{"quit", "Quit (same as :q)"},
}

func (m *model) completeCommand(direction int) {
	if len(m.completions) == 0 {
		prefix := strings.TrimSpace(m.command.value())
		if strings.HasPrefix(m.command.value(), "layout ") {
			for _, style := range layoutStyles {
				candidate := "layout " + style
				if strings.HasPrefix(candidate, prefix) {
					m.completions = append(m.completions, candidate)
				}
			}
		}
		if !strings.HasPrefix(m.command.value(), "layout ") {
			for _, command := range commands {
				if strings.HasPrefix(command.name, prefix) {
					m.completions = append(m.completions, command.name)
				}
			}
		}
		if len(m.completions) == 0 {
			m.status = "No matching commands. :?"
			return
		}
		m.completionIndex = 0
		if direction < 0 {
			m.completionIndex = len(m.completions) - 1
		}
	} else {
		m.completionIndex = (m.completionIndex + direction + len(m.completions)) % len(m.completions)
	}
	m.command.set(m.completions[m.completionIndex])
	appLog.Debug("ui.command_completed", "command", m.command.value(), "matches", len(m.completions))
}

func (m *model) helpLines() []string {
	rows := []string{m.tr("Available commands"), m.tr("Esc / Enter: return · j/k: scroll"), ""}
	for _, command := range commands {
		rows = append(rows, fmt.Sprintf("  :%-8s %s", command.name, m.tr(command.description)))
	}
	rows = append(rows, "", m.tr("Chat Insert: Enter sends · Ctrl/Shift-Enter adds a newline · Ctrl-J fallback"), m.tr("Normal mode Ctrl-O: chat → channel/DM list → server list"), m.tr("Command entry Tab: next match · Shift-Tab: previous match"), m.tr("Cycle commands matching the prefix. Example: :s → :servers → :send"))
	var lines []string
	for _, row := range rows {
		lines = append(lines, strings.Split(ansi.Hardwrap(row, m.width, true), "\n")...)
	}
	return lines
}
