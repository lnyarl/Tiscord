package main

import (
	"encoding/json"
	"strings"
)

type channelActivity struct {
	guildID, latestID string
	live              bool
	private           bool
}

func (m *model) registerActivityChannel(item channel) {
	private := item.Type == 1 || item.Type == 3
	if item.ID == "" || !private && (item.GuildID == "" || item.Type != 0 && item.Type != 5) {
		return
	}
	if m.activity == nil {
		m.activity = make(map[string]channelActivity)
	}
	state := m.activity[item.ID]
	state.guildID = item.GuildID
	state.private = private
	if state.latestID == "" || snowflakeLess(state.latestID, item.LastMessageID) {
		state.latestID = item.LastMessageID
	}
	m.activity[item.ID] = state
}

func (m *model) observeActivity(event gatewayEvent) {
	if event.fresh {
		clear(m.activity)
		for _, item := range event.channels {
			m.registerActivityChannel(item)
		}
	}
	if event.kind != "MESSAGE_CREATE" {
		return
	}
	var item struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
		GuildID   string `json:"guild_id"`
		Type      int    `json:"type"`
		Flags     uint64 `json:"flags"`
		Author    author `json:"author"`
	}
	if json.Unmarshal(event.data, &item) != nil || item.ID == "" || item.ChannelID == "" || item.Author.ID == m.selfUserID && m.selfUserID != "" || item.Flags&64 != 0 {
		return
	}
	if item.Type != 0 && item.Type != 19 && item.Type != 20 && item.Type != 23 {
		return
	}
	if m.activity == nil {
		m.activity = make(map[string]channelActivity)
	}
	state := m.activity[item.ChannelID]
	if item.GuildID != "" {
		state.guildID = item.GuildID
	}
	if state.guildID == "" && !state.private {
		return
	}
	if state.latestID == "" || snowflakeLess(state.latestID, item.ID) {
		state.latestID = item.ID
	}
	state.live = true
	m.activity[item.ChannelID] = state
}

func (m *model) channelHasUnread(id string) bool {
	state := m.activity[id]
	if state.latestID == "" || state.guildID == "" && !state.private {
		return false
	}
	readID := m.readIDs[id]
	return readID == "" && state.live || readID != "" && snowflakeLess(readID, state.latestID)
}

func (m *model) activitySymbols() string {
	if !m.config.Layout.ShowActivity {
		return ""
	}
	same, other, private := false, false, false
	for id, state := range m.activity {
		if !m.channelHasUnread(id) {
			continue
		}
		if state.private {
			private = true
		} else if state.guildID == m.guild.ID {
			same = true
		} else {
			other = true
		}
		if same && other && private {
			break
		}
	}
	var symbols []string
	if same {
		symbols = append(symbols, "\x1b[36m●\x1b[90m")
	}
	if other {
		symbols = append(symbols, "\x1b[35m◆\x1b[90m")
	}
	if private {
		symbols = append(symbols, "\x1b[33m○\x1b[90m")
	}
	return strings.Join(symbols, " ")
}
