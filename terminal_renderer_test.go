package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestNativeChatKeepsWrappedURLContiguousAndFooterFixed(t *testing.T) {
	for _, style := range layoutStyles {
		m := openDemo(t)
		m.config.Layout.Style = style
		m.width, m.height = 25, 20
		link := "https://example.com/" + strings.Repeat("path/", 15) + "?x=1&y=2"
		m.messages[0].Content = "before\n" + link + "\nafter"
		view, chat := m.viewFrame()
		frame := nativeFrame(view, chat)
		if strings.Contains(frame, "\n") || strings.Contains(frame, "\r") {
			t.Fatal("renderer inserted a physical newline")
		}
		if !strings.Contains(ansi.Strip(frame), link) || !strings.Contains(frame, ansi.SetHyperlink(link)) {
			t.Fatal("long URL was split or truncated")
		}
		if !strings.Contains(frame, fmt.Sprintf("\x1b[%d;1H> ", m.height-1)) || !strings.Contains(frame, fmt.Sprintf("\x1b[%d;1H", m.height)) {
			t.Fatal("input or metadata moved from its fixed row")
		}
		if !strings.Contains(frame, "\x1b[?7h") {
			t.Fatal("terminal autowrap not enabled")
		}
	}
}

func TestNativeChatClipsPartialLinesWithoutPaddingOrNewlines(t *testing.T) {
	m := openDemo(t)
	m.config.Layout.Style = "editor"
	m.width, m.height = 15, 8
	m.messages[0].Content = "https://example.com/" + strings.Repeat("path/", 50)
	runeKey(m, "G")
	view, chat := m.viewFrame()
	frame := nativeFrame(view, chat)
	if strings.Contains(frame, "\n") || strings.Contains(frame, "\r") {
		t.Fatal("partial line contains a hard newline")
	}
	for _, logical := range chat.layout.logical {
		if logical.start < chat.start && logical.end > chat.start {
			end := min(logical.end, chat.start+chat.height)
			want := strings.Join(chat.layout.rows[chat.start:end], "")
			if !strings.Contains(frame, want) {
				t.Fatal("partial line was padded or reordered")
			}
		}
	}
}

func TestTerminalModelKeepsInputNavigationAndRestoresModes(t *testing.T) {
	m := openDemo(t)
	var output bytes.Buffer
	tm := &terminalModel{model: m, output: &output}
	tm.View()
	initial := output.Len()
	tm.View()
	if output.Len() != initial {
		t.Fatal("unchanged frame repainted")
	}
	next, _ := tm.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if next != tm || m.screen != "channels" {
		t.Fatal("native wrapper lost model or navigation")
	}
	tm.View()
	if !strings.Contains(output.String(), "\x1b[?1002l") {
		t.Fatal("mouse capture not disabled for list")
	}
	tm.close()
	if !strings.HasSuffix(output.String(), "\x1b[?1049l") || !strings.Contains(output.String(), "\x1b[?25h") {
		t.Fatal("terminal modes not restored")
	}
}

func TestNativeModelRunsInsideBubbleTeaEventLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	m := newModel(ctx, newDemo(), nil)
	tm := &terminalModel{model: m, output: io.Discard}
	program := tea.NewProgram(tm, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler(), tea.WithContext(ctx))
	done := make(chan error, 1)
	go func() { _, err := program.Run(); done <- err }()
	program.Send(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("native UI event loop did not quit")
	}
	tm.close()
}
