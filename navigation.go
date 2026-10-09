package main

import tea "charm.land/bubbletea/v2"

type channelListState struct {
	guild    guild
	channels []channel
	index    int
	query    string
}
type chatState struct {
	list         channelListState
	channel      channel
	messages     []message
	messageIndex int
	historyEnd   bool
}

func (m *model) rememberChannels() {
	if m.guild.ID == "" {
		return
	}
	m.lastChannels = &channelListState{m.guild, append([]channel{}, m.channels...), m.index, m.query}
}

func (m *model) rememberChat() {
	if m.screen != "chat" || m.channel.ID == "" {
		return
	}
	list := channelListState{guild: m.guild, channels: append([]channel{}, m.channels...)}
	if m.lastChannels != nil && m.lastChannels.guild.ID == m.guild.ID {
		list = *m.lastChannels
	}
	m.lastChat = &chatState{list, m.channel, append([]message{}, m.messages...), m.messageIndex, m.historyEnd}
}

func (m *model) returnFromList() tea.Cmd {
	if m.busy != "" {
		m.status = "Wait for the current request"
		return nil
	}
	switch m.screen {
	case "servers":
		if m.lastChannels == nil {
			return nil
		}
		saved := m.lastChannels
		m.guild = saved.guild
		m.channels = append([]channel{}, saved.channels...)
		m.index = saved.index
		m.query = saved.query
		m.screen = "channels"
		if m.gateway != nil {
			m.gateway.Watch(m.guild.ID, "")
		}
		m.status = "이전 채널 목록으로 복귀"
	case "channels":
		if m.lastChat == nil {
			return nil
		}
		m.rememberChannels()
		saved := m.lastChat
		m.guild = saved.list.guild
		m.channels = append([]channel{}, saved.list.channels...)
		m.channel = saved.channel
		m.messages = append([]message{}, saved.messages...)
		m.messageIndex = saved.messageIndex
		m.historyEnd = saved.historyEnd
		m.draft = m.drafts[m.channel.ID]
		m.screen = "chat"
		m.index = 0
		m.query = ""
		clear(m.typists)
		m.status = "이전 채팅으로 복귀"
		if m.gateway != nil {
			m.watchReady = m.gateway.Watch(m.guild.ID, m.channel.ID)
			m.syncRequired = true
			return m.maybeSync()
		}
	}
	return nil
}
