package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func runCommand(m *model, cmd tea.Cmd) {
	if cmd != nil {
		m.Update(cmd())
	}
}
func openDemo(t *testing.T) *model {
	t.Helper()
	m := newModel(context.Background(), newDemo(), nil)
	runCommand(m, m.Init())
	runCommand(m, m.selectItem())
	runCommand(m, m.selectItem())
	if m.screen != "chat" {
		t.Fatal("failed to open chat")
	}
	return m
}
func runeKey(m *model, text string) {
	for _, r := range text {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}
func TestVimUnicodeMultilineAndFooter(t *testing.T) {
	m := openDemo(t)
	runeKey(m, "i안녕하세요")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	runeKey(m, "둘째 줄")
	if m.draft.value() != "안녕하세요\n둘째 줄" {
		t.Fatal(m.draft.value())
	}
	if len(m.messages) != 1 {
		t.Fatal("Enter sent the draft")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	runCommand(m, cmd)
	if m.messages[len(m.messages)-1].Content != "안녕하세요\n둘째 줄" || m.draft.value() != "" {
		t.Fatal("send failed")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.mode != "NORMAL" {
		t.Fatal(m.mode)
	}
	rows := strings.Split(ansi.Strip(m.View().Content), "\n")
	if len(rows) != m.height {
		t.Fatalf("wrong height: %d", len(rows))
	}
	if !strings.Contains(rows[len(rows)-1], "NORMAL · Tiscord demo / #general") || !strings.HasPrefix(rows[len(rows)-2], "> ") {
		t.Fatal(rows)
	}
	for _, width := range []int{1, 10, 40, 80} {
		m.width = width
		for _, line := range strings.Split(m.View().Content, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("line exceeds %d: %q", width, line)
			}
		}
	}
}
func TestChannelSearchAndDrafts(t *testing.T) {
	m := openDemo(t)
	m.draft.set("general 초안")
	m.execute("channels")
	runeKey(m, "/dev")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.query != "dev" {
		t.Fatal("search failed")
	}
	runCommand(m, m.selectItem())
	if m.channel.Name != "dev" || m.draft.value() != "" {
		t.Fatal("wrong channel")
	}
	m.draft.set("dev 초안")
	m.execute("channels")
	runCommand(m, m.selectItem())
	if m.draft.value() != "general 초안" {
		t.Fatal("draft lost")
	}
}

type failingClient struct{ *demoClient }

func (c failingClient) Send(context.Context, string, string) (message, error) {
	return message{}, errors.New("Network error")
}
func TestSendFailureAndDoubleSend(t *testing.T) {
	m := openDemo(t)
	m.client = failingClient{newDemo()}
	m.draft.set("keep me")
	cmd := m.send()
	if m.send() != nil {
		t.Fatal("duplicate send allowed")
	}
	runCommand(m, cmd)
	if m.draft.value() != "keep me" || m.busy != "" {
		t.Fatal("failure lost draft")
	}
}
func event(kind string, data any) gatewayEvent {
	encoded, _ := json.Marshal(data)
	return gatewayEvent{kind: kind, data: encoded}
}
func TestPushUpdatesDeletesAndDuplicateSendEcho(t *testing.T) {
	m := openDemo(t)
	m.applyEvent(event("MESSAGE_CREATE", message{ID: "3", ChannelID: "11", Content: "other channel"}))
	if len(m.messages) != 1 {
		t.Fatal("cross-channel event displayed")
	}
	created := message{ID: "3", ChannelID: "10", Content: "new", Author: author{Username: "alice"}}
	m.applyEvent(event("MESSAGE_CREATE", created))
	m.applyEvent(event("MESSAGE_CREATE", created))
	if len(m.messages) != 2 {
		t.Fatal("duplicate message")
	}
	m.applyEvent(event("MESSAGE_UPDATE", map[string]any{"id": "3", "channel_id": "10", "content": "edited"}))
	if m.messages[1].Content != "edited" || m.messages[1].Author.Username != "alice" {
		t.Fatal("partial edit destroyed omitted data")
	}
	m.applyEvent(event("MESSAGE_DELETE", map[string]any{"id": "3", "channel_id": "10"}))
	if len(m.messages) != 1 {
		t.Fatal("delete not applied")
	}
	m.applyEvent(event("MESSAGE_DELETE_BULK", map[string]any{"ids": []string{"1"}, "channel_id": "10"}))
	if len(m.messages) != 0 {
		t.Fatal("bulk delete not applied")
	}
}
func TestEventDuringHistoryLoadIsNotLost(t *testing.T) {
	m := openDemo(t)
	m.busy = "chat"
	m.Update(event("MESSAGE_CREATE", message{ID: "3", ChannelID: "10", Content: "arrived during load"}))
	m.Update(event("MESSAGE_DELETE", map[string]any{"id": "1", "channel_id": "10"}))
	m.Update(loaded{kind: "chat", messages: []message{{ID: "1", ChannelID: "10", Content: "stale history"}}})
	if len(m.messages) != 1 || m.messages[0].ID != "3" {
		t.Fatal(m.messages)
	}
}
func TestPushPreservesScrollPosition(t *testing.T) {
	m := openDemo(t)
	m.applyEvent(event("MESSAGE_CREATE", message{ID: "3", ChannelID: "10"}))
	m.messageIndex = 0
	m.applyEvent(event("MESSAGE_CREATE", message{ID: "4", ChannelID: "10"}))
	if m.messageIndex != 0 {
		t.Fatal("new event stole scroll position")
	}
}
func TestEditorVimAndControlSafety(t *testing.T) {
	e := editor{}
	e.set("한글 word\nsecond")
	e.cursor = 0
	e.word(1)
	if e.cursor != 3 {
		t.Fatal(e.cursor)
	}
	e.word(-1)
	if e.cursor != 0 {
		t.Fatal(e.cursor)
	}
	e.deleteLine()
	if e.value() != "second" {
		t.Fatal(e.value())
	}
	if strings.ContainsAny(clean("\x1b]52;c;secret\x07한글"), "\x1b\x07") {
		t.Fatal("terminal control characters were not stripped")
	}
}

func TestHistoryWaitsForGatewaySubscription(t *testing.T) {
	m := openDemo(t)
	m.gateway = newGateway("token")
	ready := make(chan struct{})
	m.watchReady = ready
	command := m.loadHistory("chat")
	done := make(chan tea.Msg, 1)
	go func() { done <- command() }()
	select {
	case <-done:
		t.Fatal("history loaded before subscription was sent")
	case <-time.After(20 * time.Millisecond):
	}
	close(ready)
	select {
	case result := <-done:
		if result.(loaded).err != nil {
			t.Fatal(result)
		}
	case <-time.After(time.Second):
		t.Fatal("history did not load after subscription")
	}
}

type uiSnapshot struct {
	screen, status, content string
	count                   int
}
type observedModel struct {
	*model
	snapshots chan uiSnapshot
}

func (m *observedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.model.Update(msg)
	snapshot := uiSnapshot{screen: m.screen, status: m.status, count: len(m.messages)}
	if len(m.messages) > 0 {
		snapshot.content = m.messages[len(m.messages)-1].Content
	}
	m.snapshots <- snapshot
	return m, cmd
}
func TestTerminalProgramKeyboardFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	m := &observedModel{newModel(ctx, newDemo(), nil), make(chan uiSnapshot, 128)}
	program := tea.NewProgram(m, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler(), tea.WithContext(ctx))
	done := make(chan error, 1)
	go func() { _, err := program.Run(); done <- err }()
	await := func(predicate func(uiSnapshot) bool) {
		t.Helper()
		for {
			select {
			case snapshot := <-m.snapshots:
				if predicate(snapshot) {
					return
				}
			case <-ctx.Done():
				t.Fatal("terminal event loop timed out")
			}
		}
	}
	await(func(s uiSnapshot) bool { return s.status == "Select a server" })
	program.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	await(func(s uiSnapshot) bool { return s.screen == "channels" })
	program.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	await(func(s uiSnapshot) bool { return s.screen == "chat" })
	program.Send(tea.KeyPressMsg{Text: "i"})
	program.Send(tea.KeyPressMsg{Text: "안녕하세요"})
	program.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	program.Send(tea.KeyPressMsg{Text: "둘째 줄"})
	program.Send(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	await(func(s uiSnapshot) bool {
		return s.status == "Sent" && s.count == 2 && s.content == "안녕하세요\n둘째 줄"
	})
	program.Send(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
