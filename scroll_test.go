package main

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestJKScrollInsideOneLongMessage(t *testing.T) {
	for _, style := range layoutStyles {
		t.Run(style, func(t *testing.T) {
			m := openDemo(t)
			m.config.Layout.Style = style
			m.height = 8
			var content []string
			for i := 0; i < 30; i++ {
				content = append(content, fmt.Sprintf("line-%02d", i))
			}
			m.messages[0].Content = strings.Join(content, "\n")
			before := m.View().Content
			m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
			up := m.View().Content
			if before == up || strings.Contains(up, "line-29") || m.messageIndex != 0 {
				t.Fatal("k did not scroll within a single message")
			}
			_, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
			runCommand(m, cmd)
			if !strings.Contains(m.View().Content, "line-29") {
				t.Fatal("j did not return to latest line")
			}
			runeKey(m, "gg")
			if !strings.Contains(m.View().Content, "line-00") || strings.Contains(m.View().Content, "line-29") {
				t.Fatal("gg did not show the top of a single message")
			}
			runeKey(m, "G")
			if !strings.Contains(m.View().Content, "line-29") {
				t.Fatal("G did not show the last line")
			}
		})
	}
}

func TestScrollingUnreadLongMessageAcknowledgesOnlyAtVisibleEnd(t *testing.T) {
	m := openDemo(t)
	m.height = 8
	c := &unreadTestClient{demoClient: newDemo()}
	m.client = c
	m.messages = unreadHistory(2)
	m.messages[1].Content = strings.Repeat("long line\n", 20) + "last line"
	m.readIDs["10"] = "1"
	m.unreadBoundary = "1"
	m.restoreUnread()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	runCommand(m, cmd)
	if len(c.acks) != 0 || m.readIDs["10"] != "1" {
		t.Fatal("long message marked read before its final line")
	}
	for i := 0; i < 30 && len(c.acks) == 0; i++ {
		_, cmd := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
		runCommand(m, cmd)
	}
	if len(c.acks) != 1 || c.acks[0] != "10:2" || !strings.Contains(m.View().Content, "last line") {
		t.Fatal("reaching latest visible line did not mark read")
	}
}

func TestNewArrivalDoesNotMoveScrolledLongMessage(t *testing.T) {
	m := openDemo(t)
	m.height = 8
	m.messages[0].Content = strings.Repeat("existing\n", 30) + "old end"
	m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	before, offset := m.View().Content, m.chatOffset
	m.applyEvent(event("MESSAGE_CREATE", message{ID: "2", ChannelID: m.channel.ID, Content: "NEW ARRIVAL"}))
	if m.messageIndex != 0 || m.chatOffset != offset || strings.Contains(m.View().Content, "NEW ARRIVAL") || strings.Split(m.View().Content, "\n")[0] != strings.Split(before, "\n")[0] {
		t.Fatal("arrival moved the scrolled viewport")
	}
}
