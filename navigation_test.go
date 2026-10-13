package main

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCtrlOMovesUpAndEscapeRestoresChat(t *testing.T) {
	for _, dm := range []bool{false, true} {
		m := openDemo(t)
		if dm {
			m.execute("dm")
			runCommand(m, m.selectItem())
		}
		channelID, guildID := m.channel.ID, m.guild.ID
		m.mode = "NORMAL"
		m.draft.set("keep draft")
		up := tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}
		m.Update(up)
		if m.screen != "channels" || m.guild.ID != guildID {
			t.Fatal("Ctrl-O did not move chat to its list")
		}
		m.Update(up)
		if m.screen != "servers" {
			t.Fatal("Ctrl-O did not move list to servers")
		}
		m.Update(up)
		if m.screen != "servers" {
			t.Fatal("Ctrl-O left top-level list")
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.screen != "channels" || m.guild.ID != guildID {
			t.Fatal("Esc did not restore list after Ctrl-O")
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.screen != "chat" || m.channel.ID != channelID || m.draft.value() != "keep draft" {
			t.Fatal("Esc lost chat or draft after Ctrl-O")
		}
	}
}

func TestCurrentChannelFocusAndDraftSurviveListRoundTrip(t *testing.T) {
	m := openDemo(t)
	m.execute("channels")
	m.index = 1
	runCommand(m, m.selectItem())
	m.draft.set("unfinished draft")
	m.draft.cursor = 4
	m.execute("channels")
	if m.items()[m.index].ID != "11" {
		t.Fatal("chat did not focus its current channel")
	}
	m.execute("servers")
	if m.items()[m.index].ID != "1" {
		t.Fatal("channel list did not focus its server")
	}
	runCommand(m, m.selectItem())
	if m.items()[m.index].ID != "11" {
		t.Fatal("server selection did not restore channel focus")
	}
	runCommand(m, m.selectItem())
	if m.channel.ID != "11" || m.draft.value() != "unfinished draft" || m.draft.cursor != 4 {
		t.Fatal("reopening channel lost draft or caret")
	}
}

func TestChannelFocusIsPerServerAndUsesIDsAfterReordering(t *testing.T) {
	m := openDemo(t)
	m.servers = []guild{{ID: "1", Name: "first"}, {ID: "2", Name: "second"}}
	m.execute("channels")
	m.index = 1
	m.execute("servers")
	m.index = 1
	m.selectItem()
	m.Update(loaded{kind: "channels", channels: []channel{{ID: "20", Name: "other"}, {ID: "21", Name: "work"}}})
	m.query = "work"
	m.index = 0
	m.execute("servers")
	m.index = 0
	m.selectItem()
	m.Update(loaded{kind: "channels", channels: []channel{{ID: "11", Name: "dev"}, {ID: "10", Name: "general"}}})
	if m.items()[m.index].ID != "11" || m.index != 0 || m.query != "" {
		t.Fatal("first server focus lost after list reorder")
	}
	m.execute("servers")
	m.index = 1
	m.selectItem()
	m.Update(loaded{kind: "channels", channels: []channel{{ID: "21", Name: "work"}, {ID: "20", Name: "other"}}})
	if m.items()[m.index].ID != "21" || m.query != "work" {
		t.Fatal("second server focus or filter overwritten")
	}
}

func TestServerFocusAndSearchSurviveReloadAndEscape(t *testing.T) {
	m := openDemo(t)
	m.servers = []guild{{ID: "1", Name: "first demo"}, {ID: "2", Name: "second demo"}}
	m.execute("servers")
	m.query = "demo"
	m.index = 1
	m.selectItem()
	m.Update(loaded{kind: "channels", channels: []channel{{ID: "20", Name: "general"}}})
	m.execute("servers")
	if m.query != "demo" || m.items()[m.index].ID != "2" {
		t.Fatal("server focus or filter not restored")
	}
	m.Update(loaded{kind: "servers", guilds: []guild{{ID: "2", Name: "second demo"}, {ID: "1", Name: "first demo"}}})
	if m.index != 0 || m.items()[m.index].ID != "2" {
		t.Fatal("server reload focused wrong ID")
	}
	m.returnFromList()
	m.execute("servers")
	if m.query != "demo" || m.items()[m.index].ID != "2" {
		t.Fatal("Esc discarded server focus")
	}
}

func TestDMFocusSurvivesLiveReorder(t *testing.T) {
	m := openDemo(t)
	m.Update(gatewayEvent{fresh: true, channels: []channel{
		{ID: "12", Type: 1, Name: "alice", LastMessageID: "10"},
		{ID: "13", Type: 1, Name: "bob", LastMessageID: "20"},
	}})
	m.execute("dm")
	m.index = 1
	m.Update(activityMessage("30", "12", "", "alice"))
	if m.index != 0 || m.items()[m.index].ID != "12" {
		t.Fatal("new DM message moved focus to another conversation")
	}
	m.execute("servers")
	if m.items()[m.index].ID != "@me" {
		t.Fatal("DM entry not focused in server list")
	}
	m.selectItem()
	if m.items()[m.index].ID != "12" {
		t.Fatal("DM entry did not restore conversation focus")
	}
}

func TestMissingSelectedChannelHasValidFallback(t *testing.T) {
	m := openDemo(t)
	m.execute("channels")
	m.index = 1
	m.execute("servers")
	m.selectItem()
	m.Update(loaded{kind: "channels", channels: []channel{{ID: "10", Name: "general"}}})
	if m.index != 0 || m.items()[m.index].ID != "10" {
		t.Fatal("removed channel left invalid selection")
	}
	m.execute("servers")
	m.selectItem()
	m.Update(loaded{kind: "channels"})
	if m.index != 0 || m.selectItem() != nil {
		t.Fatal("empty channel list left invalid selection")
	}
}

func TestCtrlORespectsInputOverlaysAndBusyRequests(t *testing.T) {
	for _, state := range []string{"INSERT", "command", "help", "config", "slash", "busy"} {
		t.Run(state, func(t *testing.T) {
			m := openDemo(t)
			m.draft.set("keep draft")
			switch state {
			case "INSERT":
				m.mode = "INSERT"
			case "command":
				m.entry = "command"
			case "help":
				m.showHelp = true
			case "config":
				m.showConfig = true
			case "slash":
				m.slash.open = true
			case "busy":
				m.busy = "send"
			}
			m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
			if m.screen != "chat" || m.draft.value() != "keep draft" {
				t.Fatal("Ctrl-O changed chat during input or request")
			}
		})
	}
}

func TestChannelListEscapeRestoresChatAndDraft(t *testing.T) {
	m := openDemo(t)
	m.draft.set("작성 중인 내용")
	m.execute("channels")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != "chat" || m.guild.ID != "1" || m.channel.ID != "10" {
		t.Fatal("Esc did not restore the previous chat")
	}
	if m.draft.value() != "작성 중인 내용" {
		t.Fatal("Esc discarded the channel draft")
	}
}

func TestChatEscapeStaysInChat(t *testing.T) {
	m := openDemo(t)
	m.mode = "INSERT"
	m.draft.set("초안")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != "chat" || m.mode != "NORMAL" || m.draft.value() != "초안" {
		t.Fatal("Insert Esc unexpectedly left the chat")
	}
	m.execute("channels")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != "chat" {
		t.Fatal("second Esc did not return to channels")
	}
}

func TestCommandAndHelpEscapeKeepChatOpen(t *testing.T) {
	m := openDemo(t)
	runeKey(m, ":s")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != "chat" || m.entry != "" {
		t.Fatal("command cancellation left the chat")
	}
	m.execute("?")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != "chat" || m.showHelp {
		t.Fatal("closing help left the chat")
	}
}

func TestServerEscapeRestoresChannelListThenPreviousChat(t *testing.T) {
	m := openDemo(t)
	m.messages = append(m.messages, message{ID: "2", ChannelID: "10", Content: "second"})
	m.messageIndex = 0
	m.draft.set("작성 중")
	m.execute("channels")
	m.query = "dev"
	m.index = 0
	m.execute("servers")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != "channels" || m.guild.ID != "1" || m.query != "dev" || m.items()[m.index].Name != "dev" {
		t.Fatal("server Esc lost previous channel list state")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != "chat" || m.channel.ID != "10" || len(m.messages) != 2 || m.messageIndex != 0 || m.draft.value() != "작성 중" {
		t.Fatal("channel list Esc lost previous chat state")
	}
	m.execute("servers")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.query != "dev" {
		t.Fatal("last visited channel list was overwritten by chat restoration")
	}
}

func TestReturningToChatRestoresItsOriginalServer(t *testing.T) {
	m := openDemo(t)
	m.draft.set("original draft")
	m.execute("servers")
	m.guild = guild{ID: "2", Name: "other server"}
	m.Update(loaded{kind: "channels", channels: []channel{{ID: "20", Name: "other"}}})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != "chat" || m.guild.ID != "1" || m.channel.ID != "10" || m.channels[0].ID != "10" || m.draft.value() != "original draft" {
		t.Fatal("returned chat used the wrong server context")
	}
}

func TestEscapeWithoutHistoryDoesNotInventPreviousView(t *testing.T) {
	m := newModel(context.Background(), newDemo(), nil)
	runCommand(m, m.Init())
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != "servers" {
		t.Fatal("initial Esc left the server list")
	}
	runCommand(m, m.selectItem())
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != "channels" {
		t.Fatal("Esc returned to a chat that was never visited")
	}
}
