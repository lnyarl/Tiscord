package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type typist struct {
	name    string
	expires time.Time
}
type typingExpired struct {
	channelID, userID string
	expires           time.Time
}

func (m *model) receiveTyping(event gatewayEvent, now time.Time) tea.Cmd {
	if event.kind == "MESSAGE_CREATE" {
		var sent struct {
			ChannelID string `json:"channel_id"`
			Author    author `json:"author"`
		}
		if json.Unmarshal(event.data, &sent) == nil && sent.ChannelID == m.channel.ID {
			delete(m.typists, sent.Author.ID)
		}
		return nil
	}
	if event.kind != "TYPING_START" {
		return nil
	}
	var data struct {
		ChannelID string `json:"channel_id"`
		UserID    string `json:"user_id"`
		Timestamp int64  `json:"timestamp"`
		Member    struct {
			Nick string `json:"nick"`
			User author `json:"user"`
		} `json:"member"`
	}
	if json.Unmarshal(event.data, &data) != nil || data.ChannelID != m.channel.ID || m.channel.ID == "" {
		return nil
	}
	if data.UserID == "" || data.UserID == m.selfUserID {
		return nil
	}
	expires := now.Add(10 * time.Second)
	if data.Timestamp != 0 {
		expires = time.Unix(data.Timestamp, 0).Add(10 * time.Second)
		if expires.After(now.Add(10 * time.Second)) {
			expires = now.Add(10 * time.Second)
		}
	}
	if !expires.After(now) {
		return nil
	}
	if previous, ok := m.typists[data.UserID]; ok && !expires.After(previous.expires) {
		return nil
	}
	name := m.config.authorName(data.Member.User)
	if m.config.Layout.AuthorName == "display" && data.Member.Nick != "" {
		name = data.Member.Nick
	}
	if name == "" {
		for i := len(m.messages) - 1; i >= 0; i-- {
			user := m.messages[i].Author
			if user.ID == data.UserID {
				name = m.config.authorName(user)
				break
			}
		}
	}
	if name == "" {
		name = "사용자 " + data.UserID
	}
	m.typists[data.UserID] = typist{singleLine(name), expires}
	appLog.Debug("ui.typing_started", "channel_id", data.ChannelID, "user_id", data.UserID, "active", len(m.typists))
	return tea.Tick(time.Until(expires), func(time.Time) tea.Msg { return typingExpired{data.ChannelID, data.UserID, expires} })
}

func (m *model) typingLabel(now time.Time) string {
	if m.screen != "chat" || m.showHelp || m.showConfig || !m.config.Layout.ShowTyping {
		return ""
	}
	var names []string
	for _, person := range m.typists {
		if person.expires.After(now) {
			names = append(names, person.name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	if len(names) > 2 {
		return fmt.Sprintf("%s 외 %d명 입력 중…", strings.Join(names[:2], ", "), len(names)-2)
	}
	return strings.Join(names, ", ") + " 입력 중…"
}
