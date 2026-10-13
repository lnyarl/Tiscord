package main

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gorilla/websocket"
)

func TestDMChatSendUnreadAndNavigation(t *testing.T) {
	m := openDemo(t)
	m.draft.set("server draft")
	m.execute("dm")
	if m.screen != "channels" || m.guild.ID != "" || m.items()[0].Name != "alice" {
		t.Fatal("DM list unavailable")
	}
	runCommand(m, m.selectItem())
	if m.screen != "chat" || m.channel.Type != 1 || !strings.Contains(m.View().Content, "DM / alice") {
		t.Fatal("DM chat not opened")
	}
	m.Update(event("TYPING_START", map[string]any{"channel_id": "12", "user_id": "100"}))
	if !strings.Contains(m.typingLabel(time.Now()), "alice") {
		t.Fatal("DM typing did not resolve recipient")
	}
	m.draft.set("hello")
	runCommand(m, m.send())
	if len(m.messages) != 1 || m.messages[0].ChannelID != "12" {
		t.Fatal("DM sent to wrong channel")
	}
	m.Update(activityMessage("10", "12", "", "other"))
	if ansi.Strip(m.activitySymbols()) != "○" {
		t.Fatal("DM unread indicator absent")
	}
	runCommand(m, m.key(tea.KeyPressMsg{Code: 'G', Text: "G"}))
	if m.activitySymbols() != "" || m.readIDs["12"] != "10" {
		t.Fatal("DM latest read not cleared")
	}
	m.draft.set("DM draft")
	m.execute("servers")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.guild.Name != "DM" || m.screen != "channels" {
		t.Fatal("server Esc did not return to DM list")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	serverDraft := m.drafts["10"]
	if m.screen != "chat" || m.channel.ID != "12" || m.draft.value() != "DM draft" || serverDraft.value() != "server draft" {
		t.Fatal("DM Esc lost chat or drafts")
	}
}

func TestServerListOpensDMWithEnter(t *testing.T) {
	m := openDemo(t)
	m.execute("servers")
	m.query = "dm"
	items := m.items()
	if len(items) != 1 || items[0].Name != "DM" {
		t.Fatal("DM missing from server list search")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.screen != "channels" || m.guild.ID != "" || m.guild.Name != "DM" || m.busy != "" || len(m.items()) != 1 {
		t.Fatal("Enter did not open DM list")
	}
	runCommand(m, m.selectItem())
	if m.screen != "chat" || m.channel.ID != "12" {
		t.Fatal("DM conversation unavailable from server list")
	}
}

func TestDMEventsNamesAndUnreadSnapshot(t *testing.T) {
	m := openDemo(t)
	m.Update(gatewayEvent{fresh: true, selfUserID: "self", channels: []channel{{ID: "12", Type: 1, LastMessageID: "10", Recipients: []author{{ID: "self", Username: "me"}, {ID: "other", Username: "alice", GlobalName: "한글"}}}}, readStates: []readState{{ChannelID: "12", LastMessageID: "9"}}})
	if ansi.Strip(m.activitySymbols()) != "○" {
		t.Fatal("DM snapshot unread not represented")
	}
	m.execute("dm")
	if m.items()[0].Name != "alice" {
		t.Fatal("DM used nickname or included self")
	}
	m.Update(event("CHANNEL_CREATE", channel{ID: "13", Type: 3, Name: "team", LastMessageID: "20", Recipients: []author{{ID: "bob", Username: "bob"}}}))
	if len(m.items()) != 2 || m.items()[0].Name != "team" {
		t.Fatal("group DM creation or sorting missed")
	}
	m.Update(event("CHANNEL_UPDATE", map[string]any{"id": "13", "name": "renamed"}))
	if m.items()[0].Name != "renamed" || m.items()[0].Type != 3 {
		t.Fatal("partial DM update lost data")
	}
	m.Update(activityMessage("21", "13", "", "bob"))
	if len(m.pending) != 0 || len(m.messages) != 1 {
		t.Fatal("foreign DM body retained")
	}
	m.Update(event("MESSAGE_ACK", map[string]any{"channel_id": "12", "message_id": "10"}))
	if ansi.Strip(m.activitySymbols()) != "○" {
		t.Fatal("reading one DM cleared another")
	}
	m.Update(event("CHANNEL_DELETE", map[string]any{"id": "13"}))
	if len(m.items()) != 1 || m.activitySymbols() != "" {
		t.Fatal("removed DM remained in list or indicators")
	}
}

func TestGatewayDMReadyResolvesRecipientsAndForwardsChannels(t *testing.T) {
	g, closeServer := localGateway(t, func(c *websocket.Conn) {
		socketSend(t, c, 10, "", 0, map[string]any{"heartbeat_interval": 10000})
		socketRead(t, c)
		socketSend(t, c, 0, "READY", 1, map[string]any{"session_id": "s", "private_channels": []channel{{ID: "12", Type: 3, RecipientIDs: []string{"alice", "unknown"}}}, "users": []author{{ID: "alice", Username: "alice"}}})
		socketSend(t, c, 0, "CHANNEL_CREATE", 2, channel{ID: "13", Type: 1, Recipients: []author{{ID: "bob", Username: "bob"}}})
	})
	defer closeServer()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	g.connect(ctx)
	select {
	case ready := <-g.events:
		if len(ready.channels) != 1 || len(ready.channels[0].Recipients) != 2 || ready.channels[0].Recipients[0].Username != "alice" || ready.channels[0].Recipients[1].ID != "unknown" {
			t.Fatal("DM recipient resolution failed")
		}
	case <-ctx.Done():
		t.Fatal("READY missing")
	}
	select {
	case event := <-g.events:
		if event.kind != "CHANNEL_CREATE" {
			t.Fatal("DM creation missing")
		}
	case <-ctx.Done():
		t.Fatal("DM creation missing")
	}
}
