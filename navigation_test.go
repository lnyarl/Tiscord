package main

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
)

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
