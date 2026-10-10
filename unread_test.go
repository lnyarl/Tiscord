package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gorilla/websocket"
)

type unreadTestClient struct {
	*demoClient
	history []message
	acks    []string
	err     error
}

func (c *unreadTestClient) MarkRead(_ context.Context, channelID, id string) error {
	c.acks = append(c.acks, channelID+":"+id)
	return c.err
}
func (c *unreadTestClient) Messages(_ context.Context, _ string, before string) ([]message, error) {
	var result []message
	for _, item := range c.history {
		if before == "" || snowflakeLess(item.ID, before) {
			result = append(result, item)
		}
	}
	if len(result) > 50 {
		result = result[len(result)-50:]
	}
	return result, nil
}
func unreadHistory(count int) []message {
	var history []message
	for i := 1; i <= count; i++ {
		history = append(history, message{ID: fmt.Sprint(i), ChannelID: "10", Content: fmt.Sprintf("message-%d", i), Author: author{ID: "100", Username: "alice"}, Timestamp: time.Unix(1000+int64(i), 0)})
	}
	return history
}

func TestEnterRestoresOldestUnreadAcrossHistoryPages(t *testing.T) {
	m := openDemo(t)
	c := &unreadTestClient{demoClient: newDemo(), history: unreadHistory(160)}
	m.client = c
	m.readIDs["10"] = "3"
	m.execute("channels")
	runCommand(m, m.selectItem())
	if m.messages[m.messageIndex].ID != "4" || !m.unreadPinned || len(m.messages) != 160 {
		t.Fatal("old unread boundary not loaded or restored")
	}
	if len(c.acks) != 0 {
		t.Fatal("channel entry marked messages read")
	}
	lines, _ := m.body()
	if strings.Count(strings.Join(lines, "\n"), "여기부터 새 메시지") != 1 || !strings.Contains(m.View().Content, "여기부터 새 메시지") {
		t.Fatal("unread separator absent")
	}
	if strings.Count(strings.Join(lines, "\n"), "alice") != 2 {
		t.Fatal("grouping crossed unread boundary")
	}
}

func TestIncomingMessagesKeepFirstUnreadVisibleAndDoNotAck(t *testing.T) {
	m := openDemo(t)
	c := &unreadTestClient{demoClient: newDemo()}
	m.client = c
	m.messages = unreadHistory(5)
	m.messageIndex = 4
	m.height = 8
	m.readIDs["10"] = "5"
	m.unreadBoundary = "5"
	for i := 6; i <= 60; i++ {
		item := unreadHistory(i)[i-1]
		if i == 6 {
			item.Content = "FIRST-UNREAD"
		}
		m.applyEvent(event("MESSAGE_CREATE", item))
	}
	if m.messages[m.messageIndex].ID != "6" || !strings.Contains(m.View().Content, "FIRST-UNREAD") || len(c.acks) != 0 {
		t.Fatal("new arrivals scrolled first unread away or marked it read")
	}
	cmd := m.key(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if cmd == nil {
		t.Fatal("G did not acknowledge latest")
	}
	m.applyEvent(event("MESSAGE_CREATE", unreadHistory(61)[60]))
	runCommand(m, cmd)
	if m.readIDs["10"] != "60" || m.firstUnread() < 0 || m.messages[m.firstUnread()].ID != "61" {
		t.Fatal("new message during read save was marked read")
	}
	if len(c.acks) != 1 || c.acks[0] != "10:60" {
		t.Fatal("read target changed during request")
	}
}

func TestExplicitLatestNavigationMarksReadAndFailureKeepsPosition(t *testing.T) {
	for _, wheel := range []bool{false, true} {
		m := openDemo(t)
		c := &unreadTestClient{demoClient: newDemo()}
		m.client = c
		m.messages = unreadHistory(3)
		m.messageIndex = 1
		m.unreadBoundary = "1"
		m.unreadPinned = true
		var cmd tea.Cmd
		if wheel {
			_, cmd = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
		} else {
			cmd = m.key(tea.KeyPressMsg{Code: 'j', Text: "j"})
		}
		runCommand(m, cmd)
		if m.readIDs["10"] != "3" || m.unreadPinned || m.firstUnread() >= 0 {
			t.Fatal("latest navigation not marked read")
		}
	}
	m := openDemo(t)
	c := &unreadTestClient{demoClient: newDemo(), err: errors.New("failed")}
	m.client = c
	m.messages = unreadHistory(3)
	m.messageIndex = 0
	m.readIDs["10"] = "1"
	m.unreadBoundary = "1"
	m.unreadPinned = true
	runCommand(m, m.key(tea.KeyPressMsg{Code: 'G', Text: "G"}))
	if m.readIDs["10"] != "1" || !m.unreadPinned || !strings.Contains(m.status, "실패") {
		t.Fatal("failed save advanced read state")
	}
}

func TestReadRequestsQueueLatestExplicitTarget(t *testing.T) {
	m := openDemo(t)
	c := &unreadTestClient{demoClient: newDemo()}
	m.client = c
	m.messages = unreadHistory(2)
	m.messageIndex = 1
	first := m.markRead()
	if m.markRead() != nil {
		t.Fatal("duplicate read request")
	}
	m.messages = unreadHistory(3)
	m.messageIndex = 2
	if m.markRead() != nil || m.readWanted["10"] != "3" {
		t.Fatal("new target not queued behind pending read")
	}
	_, next := m.Update(first())
	if next == nil {
		t.Fatal("queued explicit target not saved")
	}
	runCommand(m, next)
	if strings.Join(c.acks, ",") != "10:2,10:3" || m.readIDs["10"] != "3" {
		t.Fatal("read requests reordered")
	}
}

func TestReadStateProtocolAndAckToken(t *testing.T) {
	states := decodeReadStates(json.RawMessage(`{"entries":[{"id":"10","last_message_id":"2"},{"id":"11","last_message_id":0},{"id":"other","read_state_type":1,"last_message_id":"999"}]}`))
	if len(states) != 3 || states[1].LastMessageID != "0" {
		t.Fatal("READY read state decode failed")
	}
	m := openDemo(t)
	m.receiveReadState(gatewayEvent{fresh: true, readStates: states})
	if m.readIDs["10"] != "2" || m.readIDs["other"] != "" {
		t.Fatal("non-channel state included")
	}
	m.receiveReadState(event("MESSAGE_ACK", map[string]any{"channel_id": "10", "message_id": "3"}))
	if m.readIDs["10"] != "3" {
		t.Fatal("external read state not updated")
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/ack") {
			t.Error("invalid ack request")
		}
		if calls == 0 && body["token"] != nil || calls == 1 && body["token"] != "ack-test-token" {
			t.Error("ack token chain incorrect")
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"token":"ack-test-token"}`))
	}))
	defer server.Close()
	c := newREST("test-account-token")
	c.base = server.URL
	if err := c.MarkRead(context.Background(), "10", "2"); err != nil {
		t.Fatal(err)
	}
	if err := c.MarkRead(context.Background(), "10", "3"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("ack request missing")
	}
}

func TestGatewaySuppliesReadStateAndAck(t *testing.T) {
	g, closeServer := localGateway(t, func(c *websocket.Conn) {
		socketSend(t, c, 10, "", 0, map[string]any{"heartbeat_interval": 10000})
		socketRead(t, c)
		socketSend(t, c, 0, "READY", 1, map[string]any{"session_id": "session", "read_state": map[string]any{"entries": []map[string]any{{"id": "10", "last_message_id": "2"}}}})
		socketSend(t, c, 0, "MESSAGE_ACK", 2, map[string]any{"channel_id": "10", "message_id": "3"})
	})
	defer closeServer()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	g.connect(ctx)
	select {
	case ready := <-g.events:
		if len(ready.readStates) != 1 || ready.readStates[0].LastMessageID != "2" {
			t.Fatal("read state not forwarded")
		}
	case <-ctx.Done():
		t.Fatal("READY missing")
	}
	select {
	case ack := <-g.events:
		if ack.kind != "MESSAGE_ACK" {
			t.Fatal("ack not forwarded")
		}
	case <-ctx.Done():
		t.Fatal("ACK missing")
	}
}

func TestLateReadyRestoresUnreadAfterInitialHistoryLoad(t *testing.T) {
	m := openDemo(t)
	c := &unreadTestClient{demoClient: newDemo(), history: unreadHistory(160)}
	m.client = c
	m.gateway = newGateway("test-token")
	m.messages = append([]message{}, c.history[110:]...)
	m.messageIndex = 49
	m.unreadAwaiting = true
	m.receiveReadState(gatewayEvent{fresh: true, readStates: []readState{{ChannelID: "10", LastMessageID: "3"}}})
	runCommand(m, m.startSync())
	if m.messages[m.messageIndex].ID != "4" || !m.unreadPinned || m.unreadAwaiting || len(c.acks) != 0 {
		t.Fatal("late READY did not restore unread without marking read")
	}
}

func TestUnreadRestoreSettingAndChannelReadIsolation(t *testing.T) {
	m := openDemo(t)
	c := &unreadTestClient{demoClient: newDemo()}
	m.client = c
	m.config.Behavior.RestoreUnread = false
	m.readIDs["10"] = "1"
	m.Update(loaded{kind: "chat", channelID: "10", messages: unreadHistory(4)})
	if m.messageIndex != 3 || m.unreadPinned {
		t.Fatal("restore setting ignored")
	}
	cmd := m.markRead()
	m.channel = channel{ID: "11"}
	m.unreadBoundary = "9"
	runCommand(m, cmd)
	if m.readIDs["10"] != "4" || m.unreadBoundary != "9" {
		t.Fatal("old channel read completion changed current channel")
	}
}
