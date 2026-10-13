package main

import tea "charm.land/bubbletea/v2"

type channelListState struct {
	guild    guild
	channels []channel
	index    int
	query    string
}

type listFocus struct {
	id, query string
	index     int
}

func (m *model) listKey() string {
	if m.guild.ID == "" && m.guild.Name == "DM" {
		return "@me"
	}
	return m.guild.ID
}

func (m *model) captureFocus() listFocus {
	focus := listFocus{query: m.query, index: m.index}
	items := m.items()
	if len(items) > 0 {
		focus.id = items[max(0, min(m.index, len(items)-1))].ID
	}
	return focus
}

func (m *model) restoreFocus(focus listFocus, preferred string) {
	if preferred != "" {
		focus.id = preferred
	}
	m.query = focus.query
	find := func() bool {
		for i, item := range m.items() {
			if item.ID == focus.id {
				m.index = i
				return true
			}
		}
		return false
	}
	if focus.id != "" {
		if find() {
			return
		}
		m.query = ""
		if find() {
			return
		}
		m.query = focus.query
	}
	m.index = max(0, min(focus.index, len(m.items())-1))
}

func (m *model) rememberServers() {
	if m.screen == "servers" {
		m.serverFocus = m.captureFocus()
	}
}

func (m *model) restoreChannelFocus(preferred string) {
	m.restoreFocus(m.channelFocus[m.listKey()], preferred)
}

type chatState struct {
	unreadBoundary string
	unreadPinned   bool
	list           channelListState
	channel        channel
	messages       []message
	messageIndex   int
	historyEnd     bool
	chatScrolling  bool
	chatOffset     int
}

func (m *model) rememberChannels() {
	if m.guild.ID == "" && m.guild.Name != "DM" {
		return
	}
	m.lastChannels = &channelListState{m.guild, append([]channel{}, m.channels...), m.index, m.query}
	if m.channelFocus == nil {
		m.channelFocus = make(map[string]listFocus)
	}
	m.channelFocus[m.listKey()] = m.captureFocus()
}

func (m *model) rememberChat() {
	if m.screen != "chat" || m.channel.ID == "" {
		return
	}
	list := channelListState{guild: m.guild, channels: append([]channel{}, m.channels...)}
	if m.lastChannels != nil && m.lastChannels.guild.ID == m.guild.ID {
		list = *m.lastChannels
	}
	m.lastChat = &chatState{m.unreadBoundary, m.unreadPinned, list, m.channel, append([]message{}, m.messages...), m.messageIndex, m.historyEnd, m.chatScrolling, m.chatOffset}
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
		m.rememberServers()
		m.guild = saved.guild
		m.channels = append([]channel{}, saved.channels...)
		m.index = saved.index
		m.query = saved.query
		m.screen = "channels"
		m.restoreChannelFocus("")
		if m.gateway != nil {
			m.gateway.Watch(m.guild.ID, "")
		}
		m.status = m.tr("Returned to the previous channel list")
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
		m.chatScrolling = saved.chatScrolling
		m.chatOffset = saved.chatOffset
		m.unreadBoundary = saved.unreadBoundary
		m.unreadPinned = saved.unreadPinned
		m.draft = m.drafts[m.channel.ID]
		m.screen = "chat"
		m.index = 0
		m.query = ""
		clear(m.typists)
		m.status = m.tr("Returned to the previous chat")
		if m.gateway != nil {
			m.watchReady = m.gateway.Watch(m.guild.ID, m.channel.ID)
			m.syncRequired = true
			return m.maybeSync()
		}
	}
	return nil
}
