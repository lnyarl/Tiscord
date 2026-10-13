package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gorilla/websocket"
)

func activityMessage(id, channelID, guildID, authorID string) gatewayEvent {
	return event("MESSAGE_CREATE", map[string]any{"id": id, "channel_id": channelID, "guild_id": guildID, "author": map[string]any{"id": authorID}, "content": "private text"})
}

func TestActivityDistinguishesChannelsAndGuildsWithoutMovingChat(t *testing.T) {
	m := openDemo(t)
	m.selfUserID = "self"
	m.draft.set("draft")
	m.messageIndex = 0
	m.Update(activityMessage("20", "11", m.guild.ID, "other"))
	if got := ansi.Strip(m.activitySymbols()); got != "●" {
		t.Fatal(got)
	}
	m.Update(activityMessage("21", "22", "other-guild", "other"))
	if got := ansi.Strip(m.activitySymbols()); got != "● ◆" {
		t.Fatal(got)
	}
	if m.draft.value() != "draft" || m.messageIndex != 0 || len(m.messages) != 1 || len(m.pending) != 0 {
		t.Fatal("other channel messages changed chat state")
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "● ◆") || strings.Contains(view, "private text") || strings.Contains(view, "other-guild") {
		t.Fatal("metadata exposed message details or lost symbols")
	}
	m.readIDs["11"] = "20"
	if got := ansi.Strip(m.activitySymbols()); got != "◆" {
		t.Fatal("read channel still marked")
	}
	m.Update(event("MESSAGE_ACK", map[string]any{"channel_id": "22", "message_id": "21"}))
	if m.activitySymbols() != "" {
		t.Fatal("external acknowledgement did not clear marker")
	}
}

func TestActivityCurrentChannelClearsWhenLatestIsRead(t *testing.T) {
	m := openDemo(t)
	m.selfUserID = "self"
	m.Update(activityMessage("10", m.channel.ID, m.guild.ID, "other"))
	if got := ansi.Strip(m.activitySymbols()); got != "●" {
		t.Fatalf("current channel unread marker: %q", got)
	}
	m.Update(activityMessage("20", "22", "other-guild", "other"))
	cmd := m.key(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m.Update(activityMessage("11", m.channel.ID, m.guild.ID, "other"))
	runCommand(m, cmd)
	if got := ansi.Strip(m.activitySymbols()); got != "● ◆" {
		t.Fatalf("message arriving during read save lost marker: %q", got)
	}
	runCommand(m, m.key(tea.KeyPressMsg{Code: 'G', Text: "G"}))
	if got := ansi.Strip(m.activitySymbols()); got != "◆" {
		t.Fatalf("latest read did not clear current channel alone: %q", got)
	}
}

func TestActivityIgnoresSelfOldEventsAndDisabledSetting(t *testing.T) {
	m := openDemo(t)
	m.selfUserID = "self"
	m.observeActivity(activityMessage("11", "11", m.guild.ID, "self"))
	if m.activitySymbols() != "" {
		t.Fatal("own message marked")
	}
	m.readIDs["11"] = "30"
	m.observeActivity(activityMessage("20", "11", m.guild.ID, "other"))
	if m.activitySymbols() != "" {
		t.Fatal("replayed read message marked")
	}
	m.observeActivity(activityMessage("31", "11", m.guild.ID, "other"))
	m.Update(event("MESSAGE_ACK", map[string]any{"channel_id": "11", "message_id": "30"}))
	if ansi.Strip(m.activitySymbols()) != "●" {
		t.Fatal("new message cleared by older acknowledgement")
	}
	m.config.Layout.ShowActivity = false
	if m.activitySymbols() != "" {
		t.Fatal("activity setting ignored")
	}
}

func TestActivityReadySnapshotAndBusyChatDoNotRetainOtherBodies(t *testing.T) {
	m := openDemo(t)
	m.observeActivity(gatewayEvent{fresh: true, channels: []channel{{ID: "11", GuildID: m.guild.ID, LastMessageID: "50"}, {ID: "22", GuildID: "other", LastMessageID: "80"}}})
	m.receiveReadState(gatewayEvent{fresh: true, readStates: []readState{{ChannelID: "11", LastMessageID: "40"}, {ChannelID: "22", LastMessageID: "80"}}})
	if ansi.Strip(m.activitySymbols()) != "●" {
		t.Fatal("snapshot unread state not represented")
	}
	m.busy = "sync"
	m.Update(activityMessage("81", "22", "other", "other-user"))
	if len(m.pending) != 0 || ansi.Strip(m.activitySymbols()) != "● ◆" {
		t.Fatal("foreign body retained or activity missed during sync")
	}
	m.guild = guild{ID: "other"}
	m.channel = channel{ID: "22"}
	if ansi.Strip(m.activitySymbols()) != "● ◆" {
		t.Fatal("indicators did not follow selected server")
	}
}

func TestChannelListsShowUnreadAndClearAfterAcknowledgement(t *testing.T) {
	for _, private := range []bool{false, true} {
		name := "channels"
		if private {
			name = "DM"
		}
		t.Run(name, func(t *testing.T) {
			m := openDemo(t)
			m.selfUserID = "self"
			items := []channel{{ID: "10", Name: "unread", LastMessageID: "40"}, {ID: "11", Name: "read", LastMessageID: "30"}}
			marker, color := "●", "\x1b[36m"
			if private {
				m.guild = guild{Name: "DM"}
				for i := range items {
					items[i].Type = 1
				}
				m.privateChannels = items
				marker, color = "○", "\x1b[33m"
			}
			m.readIDs["10"], m.readIDs["11"] = "39", "30"
			m.Update(loaded{kind: "channels", channels: items})
			m.config.Layout.ShowMetadata = false
			m.index = 1
			lines, _ := m.body()
			if !strings.Contains(lines[2], color+marker) || strings.Contains(ansi.Strip(lines[3]), marker) || !strings.HasPrefix(lines[3], "> ") {
				t.Fatal("unread marker missing, read channel marked, or selection lost")
			}
			m.Update(event("MESSAGE_ACK", map[string]any{"channel_id": "10", "message_id": "40"}))
			lines, _ = m.body()
			if strings.Contains(ansi.Strip(strings.Join(lines, "\n")), marker) {
				t.Fatal("read acknowledgement did not clear list marker")
			}
			m.Update(activityMessage("31", "11", m.guild.ID, "other"))
			lines, _ = m.body()
			if !strings.Contains(lines[3], color+marker) || m.index != 1 {
				t.Fatal("new message not marked or selection moved")
			}
			m.query = "read"
			m.config.Layout.ShowActivity = false
			lines, _ = m.body()
			if strings.Contains(ansi.Strip(strings.Join(lines, "\n")), marker) {
				t.Fatal("disabled markers still visible")
			}
		})
	}
}

func TestGatewayMonitorsAllGuildsWithBoundedSubscriptionPayloads(t *testing.T) {
	g, closeServer := localGateway(t, func(c *websocket.Conn) {
		socketSend(t, c, 10, "", 0, map[string]any{"heartbeat_interval": 10000})
		socketRead(t, c)
		var guilds []map[string]any
		for i := 1; i <= 45; i++ {
			guilds = append(guilds, map[string]any{"id": fmt.Sprint(i), "channels": []map[string]any{{"id": fmt.Sprint(100 + i), "type": 0, "last_message_id": "9"}}})
		}
		socketSend(t, c, 0, "READY", 1, map[string]any{"session_id": "s", "guilds": guilds})
		seen := make(map[string]bool)
		for len(seen) < 45 {
			packet := socketRead(t, c)
			if packet.Op != 37 || len(packet.Data) > 4096 {
				t.Error("invalid or oversized subscription")
				return
			}
			var body struct {
				Subscriptions map[string]struct {
					Typing   bool           `json:"typing"`
					Channels map[string]any `json:"channels"`
				} `json:"subscriptions"`
			}
			json.Unmarshal(packet.Data, &body)
			if len(body.Subscriptions) > 20 {
				t.Error("subscription not batched")
			}
			for id, sub := range body.Subscriptions {
				if !sub.Typing || len(sub.Channels) != 0 {
					t.Error("missing event stream or member ranges requested")
				}
				seen[id] = true
			}
		}
		socketSend(t, c, 0, "MESSAGE_CREATE", 2, map[string]any{"id": "10", "guild_id": "45", "channel_id": "145"})
	})
	defer closeServer()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	g.connect(ctx)
	select {
	case ready := <-g.events:
		if len(ready.channels) != 45 || ready.channels[44].GuildID != "45" {
			t.Fatal("guild channel metadata missing")
		}
	case <-ctx.Done():
		t.Fatal("READY missing")
	}
	select {
	case msg := <-g.events:
		if msg.kind != "MESSAGE_CREATE" {
			t.Fatal("other guild message missing")
		}
	case <-ctx.Done():
		t.Fatal("message missing")
	}
}
