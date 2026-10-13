package main

import (
	tea "charm.land/bubbletea/v2"
	"time"
)

func (m *model) scrollChat(direction int) tea.Cmd {
	if len(m.messages) == 0 || m.busy == "chat" || m.busy == "older" || m.busy == "sync" {
		return nil
	}
	lines, target, positions := m.chatBody()
	height := m.chatBodyHeight()
	start := m.chatBodyStart(height, len(lines), target)
	target = max(0, min(max(0, len(lines)-height), start+direction))
	index := 0
	for i, start := range positions {
		if start > target+height-1 {
			break
		}
		index = i
	}
	m.messageIndex = index
	m.chatOffset = target - positions[index]
	m.chatScrolling = true
	if direction > 0 && target+height >= len(lines) {
		return m.markRead()
	}
	return nil
}

func (m *model) chatBodyHeight() int {
	input, _ := editorLines(m.draft, "> ", m.width)
	meta, typing, connection := 0, 0, 0
	if m.config.Layout.ShowMetadata {
		meta = 1
	}
	if m.typingLabel(time.Now()) != "" {
		typing = 1
	}
	if m.connectionHint() != "" {
		connection = 1
	}
	if m.height-meta-typing-connection < 2 {
		typing = 0
	}
	gap := min(m.config.Layout.InputGapLines, max(0, m.height-meta-typing-connection-2))
	inputHeight := min(m.config.Layout.InputMaxLines, min(len(input), max(1, m.height-meta-typing-connection-gap-1)))
	return max(1, m.height-inputHeight-gap-meta-typing-connection)
}

func (m *model) chatBodyStart(height, count, target int) int {
	if m.chatScrolling || m.unreadPinned {
		return max(0, min(target, count-height))
	}
	if m.messageIndex == len(m.messages)-1 {
		return max(0, count-height)
	}
	return max(0, min(target-height+1, count-height))
}

func (m *model) chatAtLatest() bool {
	if m.messageIndex < len(m.messages)-1 || m.unreadPinned {
		return false
	}
	if !m.chatScrolling {
		return true
	}
	lines, target, _ := m.chatBody()
	return target+m.chatBodyHeight() >= len(lines)
}

func (m *model) chatEnd() {
	lines, _, positions := m.chatBody()
	if len(positions) == 0 {
		return
	}
	m.messageIndex = len(positions) - 1
	m.chatOffset = max(0, len(lines)-m.chatBodyHeight()) - positions[m.messageIndex]
	m.chatScrolling = true
}
