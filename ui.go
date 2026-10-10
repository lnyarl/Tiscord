package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf16"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type loaded struct {
	unreadBoundary  string
	kind, channelID string
	guilds          []guild
	channels        []channel
	messages        []message
	err             error
	epoch           uint64
}
type sent struct {
	message message
	err     error
}
type streamClosed struct{}
type model struct {
	readWanted                                           map[string]string
	unreadAwaiting                                       bool
	readIDs                                              map[string]string
	readPending                                          map[string]string
	unreadBoundary                                       string
	unreadPinned                                         bool
	slash                                                slashState
	slashEpoch                                           uint64
	slashSession                                         string
	showConfig                                           bool
	configDraft                                          config
	configIndex                                          int
	configStatus                                         string
	logLevel                                             *slog.LevelVar
	configPath                                           string
	ctx                                                  context.Context
	client                                               chatClient
	gateway                                              *gateway
	screen, mode, entry, query, status, connection, busy string
	servers                                              []guild
	channels                                             []channel
	guild                                                guild
	channel                                              channel
	messages                                             []message
	index, messageIndex, width, height                   int
	draft, command                                       editor
	drafts                                               map[string]editor
	historyEnd                                           bool
	prefix                                               string
	pending                                              []gatewayEvent
	syncRequired                                         bool
	gatewayOnline                                        bool
	gatewayStopped                                       bool
	hadConnection                                        bool
	connectionEpoch                                      uint64
	watchReady                                           <-chan struct{}
	config                                               config
	showHelp                                             bool
	helpLine                                             int
	completions                                          []string
	completionIndex                                      int
	selfUserID                                           string
	typists                                              map[string]typist
	lastChannels                                         *channelListState
	lastChat                                             *chatState
}

func newModel(ctx context.Context, client chatClient, g *gateway) *model {
	connection := "Offline demo"
	if g != nil {
		connection = "Gateway connecting"
	}
	return &model{ctx: ctx, client: client, gateway: g, screen: "servers", mode: "NORMAL", status: "Loading", connection: connection, busy: "servers", width: 80, height: 24, drafts: map[string]editor{}, config: defaultConfig(), typists: map[string]typist{}, gatewayOnline: g == nil, readIDs: make(map[string]string), readPending: make(map[string]string), readWanted: make(map[string]string)}
}
func (m *model) Init() tea.Cmd {
	load := func() tea.Msg {
		data, err := m.client.Guilds(m.ctx)
		return loaded{kind: "servers", guilds: data, err: err}
	}
	if m.gateway == nil {
		return load
	}
	return tea.Batch(load, m.nextEvent())
}
func (m *model) nextEvent() tea.Cmd {
	if m.gateway == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case event, ok := <-m.gateway.events:
			if !ok {
				return streamClosed{}
			}
			return event
		case <-m.ctx.Done():
			return streamClosed{}
		}
	}
}
func (m *model) items() []channel {
	var items []channel
	if m.screen == "servers" {
		for _, s := range m.servers {
			items = append(items, channel{ID: s.ID, Name: s.Name})
		}
	} else {
		items = m.channels
	}
	var result []channel
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Name), strings.ToLower(m.query)) {
			result = append(result, item)
		}
	}
	return result
}
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	previousScreen, previousMode, previousBusy := m.screen, m.mode, m.busy
	defer func() {
		if previousScreen != m.screen || previousMode != m.mode || previousBusy != m.busy {
			appLog.Debug("ui.state", "screen", m.screen, "mode", m.mode, "busy", m.busy, "messages", len(m.messages))
		}
	}()
	appLog.Debug("ui.event", "type", fmt.Sprintf("%T", msg), "screen", m.screen, "busy", m.busy)
	if cmd, handled := m.updateSlash(msg); handled {
		return m, cmd
	}
	switch msg := msg.(type) {
	case readMarked:
		if m.readPending[msg.channelID] == msg.messageID {
			delete(m.readPending, msg.channelID)
		}
		if msg.err != nil {
			m.status = "읽음 위치 저장 실패: " + msg.err.Error()
			appLog.Warn("read.save_failed", "channel_id", msg.channelID, "error", msg.err)
		} else {
			if current := m.readIDs[msg.channelID]; current == "" || snowflakeLess(current, msg.messageID) {
				m.readIDs[msg.channelID] = msg.messageID
			}
			if m.channel.ID == msg.channelID {
				m.unreadBoundary = m.readIDs[msg.channelID]
				if m.firstUnread() < 0 {
					m.unreadPinned = false
				}
				m.status = "읽음 위치 저장 완료"
			}
		}
		if wanted := m.readWanted[msg.channelID]; wanted != "" {
			delete(m.readWanted, msg.channelID)
			return m, m.markReadID(msg.channelID, wanted)
		}
	case typingExpired:
		if person, ok := m.typists[msg.userID]; ok && m.channel.ID == msg.channelID && person.expires.Equal(msg.expires) {
			delete(m.typists, msg.userID)
			appLog.Debug("ui.typing_expired", "user_id", msg.userID)
		}
	case tea.WindowSizeMsg:
		m.width = max(1, msg.Width)
		m.height = max(4, msg.Height)
	case tea.MouseWheelMsg:
		if m.screen == "chat" && m.entry == "" && !m.showHelp && !m.showConfig && !m.slash.open && len(m.messages) > 0 {
			if msg.Button == tea.MouseWheelUp {
				m.messageIndex = max(0, m.messageIndex-1)
			}
			if msg.Button == tea.MouseWheelDown {
				m.messageIndex = min(len(m.messages)-1, m.messageIndex+1)
				if m.messageIndex == len(m.messages)-1 {
					return m, m.markRead()
				}
			}
		}
	case tea.KeyboardEnhancementsMsg:
		appLog.Info("keyboard.capabilities", "alternate_keys", msg.SupportsAlternateKeys(), "all_keys", msg.SupportsAllKeysAsEscapeCodes(), "associated_text", msg.SupportsAssociatedText())
	case tea.PasteMsg:
		if m.slash.open {
			if !m.slash.loading && !m.slash.submitting && (m.slash.command == nil || m.slash.option() != nil) {
				m.slash.input.insert([]rune(singleLine(msg.Content)))
				m.slash.index = 0
				return m, m.scheduleSlashAutocomplete()
			}
			break
		}
		if m.showConfig {
			break
		}
		if m.entry != "" {
			m.command.insert([]rune(singleLine(msg.Content)))
			m.completions = nil
		} else if m.mode == "INSERT" && m.screen == "chat" && m.busy != "send" && !m.showHelp {
			m.draft.insert([]rune(clean(msg.Content)))
		}
	case streamClosed: // A terminal Gateway failure remains visible; there is no polling fallback.
		m.connection = "Gateway stopped"
		m.gatewayOnline = false
		m.gatewayStopped = true
		m.connectionEpoch++
		clear(m.typists)
	case gatewayEvent:
		if m.receiveReadState(msg) {
			return m, m.nextEvent()
		}
		if msg.sessionID != "" {
			m.slashSession = msg.sessionID
		}
		m.slashEvent(msg)
		if msg.kind == "APPLICATION_COMMAND_AUTOCOMPLETE_RESPONSE" || msg.kind == "INTERACTION_CREATE" || msg.kind == "INTERACTION_SUCCESS" || msg.kind == "INTERACTION_FAILURE" {
			return m, m.nextEvent()
		}
		if msg.selfUserID != "" {
			m.selfUserID = msg.selfUserID
			delete(m.typists, msg.selfUserID)
		}
		if msg.status != "" {
			m.connectionEpoch++
			m.gatewayOnline = msg.status == "Gateway connected" || msg.status == "Gateway resumed"
			if m.gatewayOnline {
				m.hadConnection = true
				m.gatewayStopped = false
			}
			if m.channel.ID != "" {
				m.syncRequired = true
			}
			if msg.status != "Gateway connected" && msg.status != "Gateway resumed" {
				clear(m.typists)
			}
			m.connection = msg.status
		}
		typingCommand := m.receiveTyping(msg, time.Now())
		if msg.kind == "TYPING_START" {
			return m, tea.Batch(typingCommand, m.nextEvent())
		}
		if msg.status != "" {
			if command := m.maybeSync(); command != nil {
				return m, tea.Batch(command, m.nextEvent())
			}
		}
		if msg.kind != "" {
			if m.busy == "chat" || m.busy == "older" || m.busy == "sync" {
				m.pending = append(m.pending, msg)
			} else {
				m.applyEvent(msg)
			}
		}
		return m, m.nextEvent()
	case loaded:
		appLog.Debug("ui.loaded", "kind", msg.kind, "channel_id", msg.channelID, "servers", len(msg.guilds), "channels", len(msg.channels), "messages", len(msg.messages))
		m.busy = ""
		if msg.kind == "sync" && msg.epoch != m.connectionEpoch {
			appLog.Warn("ui.sync_stale", "epoch", msg.epoch, "current_epoch", m.connectionEpoch)
			return m, m.maybeSync()
		}
		if msg.err != nil {
			appLog.Error("ui.load_failed", "kind", msg.kind, "error", msg.err.Error())
			m.status = msg.kind + ": " + msg.err.Error()
			if msg.kind == "sync" {
				for _, event := range m.pending {
					m.applyEvent(event)
				}
			}
			m.pending = nil
			if msg.kind != "sync" {
				return m, m.maybeSync()
			}
			return m, nil
		}
		switch msg.kind {
		case "servers":
			m.servers = msg.guilds
			m.status = "Select a server"
		case "channels":
			m.channels = msg.channels
			m.screen = "channels"
			m.query = ""
			m.index = 0
			m.status = "Select a channel"
			m.rememberChannels()
		case "chat":
			if m.screen != "chat" {
				m.mode = strings.ToUpper(m.config.Behavior.StartMode)
			}
			m.messages = mergeMessages(nil, msg.messages)
			m.messageIndex = max(0, len(m.messages)-1)
			m.historyEnd = len(msg.messages) < 50
			m.screen = "chat"
			m.query = ""
			m.index = 0
			for _, event := range m.pending {
				m.applyEvent(event)
			}
			m.pending = nil
			m.status = "Ready"
			m.unreadBoundary = m.readIDs[m.channel.ID]
			if m.unreadBoundary != "" && m.unreadBoundary != "0" && m.config.Behavior.RestoreUnread && len(m.messages) > 0 && snowflakeLess(m.unreadBoundary, m.messages[0].ID) && msg.unreadBoundary != m.unreadBoundary {
				return m, m.loadHistory("chat")
			}
			m.restoreUnread()
			if m.hadConnection {
				m.unreadAwaiting = false
			}
		case "older":
			anchor := ""
			if len(m.messages) > 0 {
				anchor = m.messages[m.messageIndex].ID
			}
			m.messages = mergeMessages(m.messages, msg.messages)
			m.historyEnd = len(msg.messages) < 50
			for i, item := range m.messages {
				if item.ID == anchor {
					m.messageIndex = i
					break
				}
			}
			m.status = "History loaded"
			for _, event := range m.pending {
				m.applyEvent(event)
			}
			m.pending = nil
		case "sync":
			anchor := ""
			following := m.messageIndex >= len(m.messages)-1 && !m.unreadPinned
			if len(m.messages) > 0 {
				anchor = m.messages[m.messageIndex].ID
			}
			m.messages = mergeMessages(nil, msg.messages)
			m.messageIndex = min(m.messageIndex, max(0, len(m.messages)-1))
			if following {
				m.messageIndex = max(0, len(m.messages)-1)
			} else {
				for i, item := range m.messages {
					if item.ID == anchor {
						m.messageIndex = i
						break
					}
				}
			}
			for _, event := range m.pending {
				m.applyEvent(event)
			}
			m.pending = nil
			m.syncRequired = false
			m.status = "복구 완료 · 전송 가능"
			if m.unreadAwaiting {
				m.unreadBoundary = m.readIDs[m.channel.ID]
				m.restoreUnread()
				m.unreadAwaiting = false
			}
			appLog.Info("ui.sync_completed", "channel_id", msg.channelID, "messages", len(m.messages), "epoch", msg.epoch)
		}
		if command := m.maybeSync(); command != nil {
			return m, command
		}
	case sent:
		appLog.Debug("ui.send_result", "message_id", msg.message.ID, "success", msg.err == nil)
		m.busy = ""
		if msg.err != nil {
			appLog.Error("ui.send_failed", "error", msg.err.Error())
			m.status = "send: " + msg.err.Error()
		} else {
			m.messages = mergeMessages(m.messages, []message{msg.message})
			m.messageIndex = len(m.messages) - 1
			m.draft.set("")
			m.status = "Sent"
		}
		if command := m.maybeSync(); command != nil {
			return m, command
		}
	case tea.KeyPressMsg:
		return m, m.key(msg)
	}
	return m, nil
}
func (m *model) key(key tea.KeyPressMsg) tea.Cmd {
	if m.slash.open {
		if keyName(key) == "ctrl+c" || keyName(key) == "ctrl+q" {
			return tea.Quit
		}
		return m.slashKey(key)
	}
	if command, handled := m.koreanNormalKey(key); handled {
		return command
	}
	name := keyName(key)
	if m.mode == "NORMAL" && m.entry == "" {
		name = normalKeyName(key)
	}
	if name == "ctrl+c" || name == "ctrl+q" {
		return tea.Quit
	}
	if m.showConfig {
		m.configKey(name)
		return nil
	}
	if name == "esc" {
		returnToPrevious := (m.screen == "servers" || m.screen == "channels") && m.entry == "" && !m.showHelp
		m.showHelp = false
		m.completions = nil
		m.mode = "NORMAL"
		m.entry = ""
		m.prefix = ""
		if returnToPrevious {
			return m.returnFromList()
		}
		return nil
	}
	if m.entry != "" {
		if m.entry == "command" && (name == "tab" || name == "shift+tab") {
			direction := 1
			if name == "shift+tab" {
				direction = -1
			}
			m.completeCommand(direction)
			return nil
		}
		if name == "enter" {
			m.completions = nil
			value, entry := strings.TrimSpace(m.command.value()), m.entry
			m.entry = ""
			m.mode = "NORMAL"
			if entry == "search" {
				m.query = value
				m.index = 0
				return nil
			}
			return m.execute(value)
		}
		previous := m.command.value()
		m.command.key(key, false)
		if m.command.value() != previous {
			m.completions = nil
		}
		return nil
	}
	if m.showHelp {
		switch name {
		case "enter":
			m.showHelp = false
		case "j", "down":
			m.helpLine = min(len(m.helpLines())-1, m.helpLine+1)
		case "k", "up":
			m.helpLine = max(0, m.helpLine-1)
		case "g":
			m.helpLine = 0
		case "G":
			m.helpLine = len(m.helpLines()) - 1
		case ":":
			m.entry = "command"
			m.command.set("")
			m.completions = nil
		}
		return nil
	}
	if name == "ctrl+s" || name == m.config.Behavior.SendKey && m.screen == "chat" && m.mode == "INSERT" {
		return m.send()
	}
	if m.mode == "INSERT" {
		if m.busy != "send" {
			if m.draft.value() == "" && key.Text == "/" {
				return m.openSlash()
			}
			m.draft.key(key, true)
		}
		return nil
	}
	previous := m.prefix
	m.prefix = ""
	switch name {
	case "j", "down":
		if m.screen == "chat" {
			m.messageIndex = min(max(0, len(m.messages)-1), m.messageIndex+1)
			if m.messageIndex == len(m.messages)-1 {
				return m.markRead()
			}
		} else {
			m.index = min(max(0, len(m.items())-1), m.index+1)
		}
	case "k", "up":
		if m.screen == "chat" {
			m.messageIndex = max(0, m.messageIndex-1)
		} else {
			m.index = max(0, m.index-1)
		}
	case "g":
		if previous == "g" {
			m.index = 0
			m.messageIndex = 0
		} else {
			m.prefix = "g"
		}
	case "G":
		m.index = max(0, len(m.items())-1)
		m.messageIndex = max(0, len(m.messages)-1)
		if m.screen == "chat" {
			return m.markRead()
		}
	case ":":
		m.completions = nil
		m.entry = "command"
		m.command.set("")
	case "/":
		if m.screen == "chat" {
			return m.openSlash()
		} else {
			m.entry = "search"
			m.command.set("")
		}
	case "enter":
		if m.screen != "chat" {
			return m.selectItem()
		}
	case "ctrl+u":
		return m.older()
	default:
		if m.screen != "chat" || m.busy == "send" {
			return nil
		}
		switch name {
		case "i":
			m.mode = "INSERT"
		case "a":
			m.draft.cursor = min(len(m.draft.text), m.draft.cursor+1)
			m.mode = "INSERT"
		case "I":
			m.draft.lineStart()
			m.mode = "INSERT"
		case "A":
			m.draft.lineEnd()
			m.mode = "INSERT"
		case "h", "left":
			m.draft.cursor = max(0, m.draft.cursor-1)
		case "l", "right":
			m.draft.cursor = min(len(m.draft.text), m.draft.cursor+1)
		case "w":
			m.draft.word(1)
		case "b":
			m.draft.word(-1)
		case "0", "home":
			m.draft.lineStart()
		case "$", "end":
			m.draft.lineEnd()
		case "x":
			m.draft.remove(m.draft.cursor, m.draft.cursor+1)
		case "d":
			if previous == "d" {
				m.draft.deleteLine()
			} else {
				m.prefix = "d"
			}
		}
	}
	return nil
}
func (m *model) selectItem() tea.Cmd {
	items := m.items()
	if m.busy != "" || len(items) == 0 {
		return nil
	}
	item := items[min(m.index, len(items)-1)]
	appLog.Info("ui.select", "screen", m.screen, "id", item.ID)
	if m.screen == "servers" {
		m.guild = guild{ID: item.ID, Name: item.Name}
		m.busy = "channels"
		m.status = "Loading"
		if m.gateway != nil {
			m.gateway.Watch(item.ID, "")
		}
		return func() tea.Msg {
			data, err := m.client.Channels(m.ctx, item.ID)
			return loaded{kind: "channels", channels: data, err: err}
		}
	}
	m.rememberChannels()
	m.saveDraft()
	clear(m.typists)
	m.syncRequired = false
	m.channel = item
	m.unreadBoundary = ""
	m.unreadPinned = false
	m.unreadAwaiting = m.gateway != nil && !m.hadConnection
	m.draft = m.drafts[item.ID]
	m.messages = nil
	m.pending = nil
	if m.gateway != nil {
		m.watchReady = m.gateway.Watch(m.guild.ID, item.ID)
	}
	return m.loadHistory("chat")
}
func (m *model) loadHistory(kind string) tea.Cmd {
	appLog.Debug("ui.history_requested", "kind", kind, "channel_id", m.channel.ID)
	m.busy = kind
	m.status = "Loading"
	id, before := m.channel.ID, ""
	if kind == "older" {
		before = m.messages[0].ID
	}
	ready := m.watchReady
	boundary := ""
	if kind == "chat" && m.config.Behavior.RestoreUnread {
		boundary = m.readIDs[id]
	}
	return func() tea.Msg {
		if ready != nil {
			appLog.Debug("ui.wait_subscription", "channel_id", id)
			select {
			case <-ready:
				appLog.Debug("ui.subscription_ready", "channel_id", id)
			case <-m.gateway.stopped:
				return loaded{kind: kind, err: errors.New("Gateway unavailable; history load cancelled")}
			case <-m.ctx.Done():
				return loaded{kind: kind, err: m.ctx.Err()}
			}
		}
		var data []message
		var err error
		if boundary != "" && boundary != "0" {
			data, err = syncHistory(m.ctx, m.client, id, boundary)
		} else {
			data, err = m.client.Messages(m.ctx, id, before)
		}
		return loaded{kind: kind, channelID: id, messages: data, err: err, unreadBoundary: boundary}
	}
}
func (m *model) older() tea.Cmd {
	if m.screen != "chat" || m.busy != "" || len(m.messages) == 0 || m.historyEnd {
		return nil
	}
	return m.loadHistory("older")
}
func (m *model) saveDraft() {
	if m.channel.ID != "" {
		copyDraft := m.draft
		copyDraft.text = append([]rune{}, m.draft.text...)
		m.drafts[m.channel.ID] = copyDraft
	}
}
func (m *model) execute(command string) tea.Cmd {
	if command == "slash" {
		return m.openSlash()
	}
	if command == "config" {
		m.configDraft = m.config
		m.configIndex = 0
		m.configStatus = ""
		m.showConfig = true
		m.showHelp = false
		m.mode = "NORMAL"
		m.prefix = ""
		return nil
	}
	if command == "layout" || strings.HasPrefix(command, "layout ") {
		m.changeLayout(command)
		return nil
	}
	if command == "?" || command == "help" {
		m.showHelp = true
		m.helpLine = 0
		m.mode = "NORMAL"
		appLog.Info("ui.help_opened")
		return nil
	}
	if command == "q" || command == "quit" {
		return tea.Quit
	}
	if m.busy != "" {
		m.status = "Wait for the current request"
		return nil
	}
	m.showHelp = false
	switch command {
	case "channels", "servers":
		if command == "channels" && m.guild.ID == "" {
			m.status = "Select a server first"
			return nil
		}
		if m.screen == "chat" {
			m.rememberChat()
		} else if m.screen == "channels" {
			m.rememberChannels()
		}
		m.saveDraft()
		clear(m.typists)
		m.channel = channel{}
		m.draft.set("")
		m.screen = command
		m.index = 0
		m.query = ""
		if command == "channels" {
			m.rememberChannels()
		}
		if command == "servers" {
			m.guild = guild{}
			if m.gateway != nil {
				m.gateway.Watch("", "")
			}
		}
	case "send":
		return m.send()
	case "older":
		return m.older()
	case "sync":
		if !m.gatewayOnline {
			m.status = "연결 복구를 기다려주세요. 초안을 유지합니다"
			return nil
		}
		return m.startSync()
	default:
		m.status = "Unknown command. :?"
	}
	return nil
}
func (m *model) send() tea.Cmd {
	if text := strings.TrimSpace(m.draft.value()); strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "//") {
		m.status = "슬래시 명령은 / 또는 :slash 입력 도우미에서 실행해주세요"
		return nil
	}
	if m.gateway != nil && (!m.gatewayOnline || m.gatewayStopped || m.syncRequired) {
		m.status = "전송 불가 · 연결 및 메시지 복구를 기다려주세요"
		appLog.Warn("ui.send_blocked", "online", m.gatewayOnline, "sync_required", m.syncRequired)
		return nil
	}
	if m.screen != "chat" || m.busy != "" || strings.TrimSpace(m.draft.value()) == "" {
		return nil
	}
	content := m.draft.value()
	if strings.HasPrefix(strings.TrimSpace(content), "//") {
		position := strings.Index(content, "//")
		content = content[:position] + content[position+1:]
	}
	if len(utf16.Encode([]rune(content))) > 2000 {
		m.status = "Message too long (maximum 2,000 characters)"
		return nil
	}
	m.busy = "send"
	appLog.Info("ui.send_requested", "channel_id", m.channel.ID, "characters", len(utf16.Encode([]rune(content))))
	m.status = "Sending"
	id := m.channel.ID
	return func() tea.Msg { data, err := m.client.Send(m.ctx, id, content); return sent{data, err} }
}
func (m *model) applyEvent(event gatewayEvent) {
	var identity struct {
		ID        string   `json:"id"`
		ChannelID string   `json:"channel_id"`
		IDs       []string `json:"ids"`
	}
	if json.Unmarshal(event.data, &identity) != nil || identity.ChannelID != m.channel.ID || m.channel.ID == "" {
		appLog.Debug("ui.message_ignored", "event", event.kind, "reason", "other_channel_or_invalid")
		return
	}
	appLog.Debug("ui.message_applied", "event", event.kind, "channel_id", identity.ChannelID, "message_id", identity.ID)
	anchor := ""
	following := m.messageIndex >= len(m.messages)-1 && !m.unreadPinned
	if following && event.kind == "MESSAGE_CREATE" && m.unreadBoundary == "" {
		m.unreadBoundary = "0"
		if len(m.messages) > 0 {
			m.unreadBoundary = m.messages[len(m.messages)-1].ID
		}
	}
	if len(m.messages) > 0 {
		anchor = m.messages[m.messageIndex].ID
	}
	switch event.kind {
	case "MESSAGE_CREATE":
		var data message
		if json.Unmarshal(event.data, &data) == nil {
			m.messages = mergeMessages(m.messages, []message{data})
		}
	case "MESSAGE_UPDATE":
		// Update payloads are partial; preserve author, timestamp and omitted fields.
		for i, item := range m.messages {
			if item.ID == identity.ID {
				data, _ := json.Marshal(item)
				var fields map[string]json.RawMessage
				var patch map[string]json.RawMessage
				json.Unmarshal(data, &fields)
				json.Unmarshal(event.data, &patch)
				for key, value := range patch {
					fields[key] = value
				}
				data, _ = json.Marshal(fields)
				var updated message
				if json.Unmarshal(data, &updated) == nil {
					m.messages[i] = updated
				}
				break
			}
		}
	case "MESSAGE_DELETE", "MESSAGE_DELETE_BULK":
		ids := map[string]bool{identity.ID: true}
		for _, id := range identity.IDs {
			ids[id] = true
		}
		kept := make([]message, 0, len(m.messages))
		for _, item := range m.messages {
			if !ids[item.ID] {
				kept = append(kept, item)
			}
		}
		m.messages = kept
	}
	if following {
		m.messageIndex = max(0, len(m.messages)-1)
		if event.kind == "MESSAGE_CREATE" && m.firstUnread() >= 0 {
			m.messageIndex = m.firstUnread()
			m.unreadPinned = true
		}
	} else {
		m.messageIndex = min(m.messageIndex, max(0, len(m.messages)-1))
		for i, item := range m.messages {
			if item.ID == anchor {
				m.messageIndex = i
				break
			}
		}
	}
}
func (m *model) body() ([]string, int) {
	if m.slash.open {
		return m.slashLines()
	}
	if m.showConfig {
		return m.configLines(), m.configIndex + 4
	}
	if m.showHelp {
		lines := m.helpLines()
		return lines, min(m.helpLine, len(lines)-1)
	}
	if m.screen != "chat" {
		lines := []string{strings.ToUpper(m.screen) + "   / search · j/k move · Enter select", ""}
		for i, item := range m.items() {
			prefix := "  "
			if i == m.index {
				prefix = "> "
			}
			if m.screen == "channels" {
				prefix += "#"
			}
			lines = append(lines, ansi.Truncate(prefix+singleLine(item.Name), m.width, ""))
		}
		if len(lines) == 2 {
			lines = append(lines, "No matching servers or channels")
		}
		return lines, m.index + 2
	}
	if len(m.messages) == 0 {
		return []string{"No messages yet. Press i to write."}, 0
	}
	var lines []string
	target := 0
	groupPrefix := ""
	firstUnread := m.firstUnread()
	wrapWidth := m.width
	gutter := 0
	if m.config.Layout.Style == "editor" && m.width > 8 {
		gutter = 6
		wrapWidth -= gutter
	}
	for i, item := range m.messages {
		unreadStart := i == firstUnread
		unreadLine := len(lines)
		if unreadStart {
			lines = append(lines, "── 여기부터 새 메시지 ──")
		}
		grouped := false
		if i > 0 {
			previous := m.messages[i-1]
			gap := item.Timestamp.Sub(previous.Timestamp)
			grouped = m.config.Behavior.GroupMessages && item.Author.ID != "" && item.Author.ID == previous.Author.ID && !item.Timestamp.IsZero() && !previous.Timestamp.IsZero() && gap >= 0 && gap <= time.Duration(m.config.Behavior.GroupWindowSeconds)*time.Second
			if unreadStart {
				grouped = false
			}
			if !grouped {
				for n := 0; n < m.config.Layout.GroupSpacing; n++ {
					lines = append(lines, "")
				}
			}
		}
		if i == m.messageIndex {
			target = len(lines)
			if unreadStart {
				target = unreadLine
			}
		}
		name := m.config.authorName(item.Author)
		if name == "" {
			name = item.Author.ID
		}
		stamp := "--:--"
		if !item.Timestamp.IsZero() {
			stamp = item.Timestamp.Local().Format(m.config.timeFormat())
		}
		if !grouped {
			groupPrefix = singleLine(name) + "  "
			if m.config.Layout.ShowTimestamps {
				groupPrefix = stamp + " " + groupPrefix
			}
			if m.config.Layout.Style == "log" {
				groupPrefix = "INFO  " + singleLine(name) + " | "
				if m.config.Layout.ShowTimestamps {
					groupPrefix = "[" + stamp + "] " + groupPrefix
				}
			}
		}
		indent := strings.Repeat(" ", min(ansi.StringWidth(groupPrefix), max(0, wrapWidth-4)))
		prefix := groupPrefix
		if grouped {
			prefix = indent
		}
		content := messageText(item)
		if m.config.Layout.Style == "editor" {
			if !grouped {
				lines = append(lines, strings.Split(ansi.Hardwrap("// "+strings.TrimSpace(groupPrefix), wrapWidth, true), "\n")...)
			}
			prefix = ""
			indent = ""
		}
		content = strings.ReplaceAll(content, "\n", "\n"+indent)
		lines = append(lines, strings.Split(ansi.Hardwrap(prefix+content, wrapWidth, true), "\n")...)
	}
	if gutter > 0 {
		for i, line := range lines {
			lines[i] = fmt.Sprintf("%4d  %s", i+1, line)
		}
	}
	return lines, target
}
func editorLines(e editor, prefix string, width int) ([]string, int) {
	before := prefix + string(e.text[:e.cursor])
	beforeLines := strings.Split(ansi.Hardwrap(before, width, true), "\n")
	// A visible caret works consistently without terminal-specific cursor placement.
	text := before + "▏" + string(e.text[e.cursor:])
	return strings.Split(ansi.Hardwrap(text, width, true), "\n"), len(beforeLines) - 1
}
func (m *model) View() tea.View {
	input, cursor := editorLines(m.draft, "> ", m.width)
	if m.entry != "" {
		prefix := ":"
		if m.entry == "search" {
			prefix = "/"
		}
		input, cursor = editorLines(m.command, prefix, m.width)
	}
	if m.showConfig {
		input, cursor = []string{m.configStatus}, 0
		if m.configStatus == "" {
			input[0] = "j/k 이동 · h/l 변경 · Ctrl-S 저장 · Esc 취소"
		}
	}
	if m.slash.open {
		prefix := "/"
		if option := m.slash.option(); option != nil {
			prefix = singleLine(option.Name) + ": "
		}
		input, cursor = editorLines(m.slash.input, prefix, m.width)
		if m.slash.command != nil && m.slash.option() == nil {
			input, cursor = []string{"Ctrl-S: 실행 · Esc: 이전 인자"}, 0
		}
	}
	metaHeight := 0
	if m.config.Layout.ShowMetadata {
		metaHeight = 1
	}
	typingText := m.typingLabel(time.Now())
	if m.slash.open {
		typingText = m.slash.hint()
	}
	typingHeight := 0
	if typingText != "" {
		typingHeight = 1
	}
	connectionText := m.connectionHint()
	connectionHeight := 0
	if connectionText != "" {
		connectionHeight = 1
	}
	if m.height-metaHeight-typingHeight-connectionHeight < 2 {
		typingText = ""
		typingHeight = 0
	}
	gapHeight := min(m.config.Layout.InputGapLines, max(0, m.height-metaHeight-typingHeight-connectionHeight-2))
	inputHeight := min(m.config.Layout.InputMaxLines, min(len(input), max(1, m.height-metaHeight-typingHeight-connectionHeight-gapHeight-1)))
	inputStart := max(0, min(cursor-inputHeight+1, len(input)-inputHeight))
	input = input[inputStart : inputStart+inputHeight]
	bodyHeight := max(1, m.height-inputHeight-gapHeight-metaHeight-typingHeight-connectionHeight)
	body, target := m.body()
	start := max(0, min(target-bodyHeight+1, len(body)-bodyHeight))
	if !m.showHelp && !m.showConfig && !m.slash.open && m.screen == "chat" && m.unreadPinned {
		start = max(0, min(target, len(body)-bodyHeight))
	}
	if !m.showHelp && !m.showConfig && !m.slash.open && !m.unreadPinned && m.screen == "chat" && m.messageIndex == len(m.messages)-1 {
		start = max(0, len(body)-bodyHeight)
	}
	visible := append([]string{}, body[start:min(len(body), start+bodyHeight)]...)
	for len(visible) < bodyHeight {
		visible = append(visible, "")
	}
	for n := 0; n < gapHeight; n++ {
		visible = append(visible, "")
	}
	visible = append(visible, input...)
	if connectionText != "" {
		visible = append(visible, ansi.Truncate(connectionText, m.width, ""))
	}
	if typingText != "" {
		visible = append(visible, "\x1b[90m"+ansi.Truncate(typingText, m.width, "")+"\x1b[0m")
	}
	mode := m.mode
	if m.config.Layout.Style != "plain" {
		mode += " [" + m.config.Layout.Style + "]"
	}
	if m.entry != "" {
		mode = strings.ToUpper(m.entry)
	}
	location := singleLine(m.guild.Name)
	if m.channel.ID != "" {
		location += " / #" + singleLine(m.channel.Name)
	}
	if location == "" {
		location = m.screen
	}
	meta := fmt.Sprintf("%s · %s · %s · %s", mode, location, singleLine(m.connection), singleLine(m.status))
	if m.config.Layout.ShowMetadata {
		visible = append(visible, "\x1b[90m"+ansi.Truncate(meta, m.width, "")+"\x1b[0m")
	}
	for i, line := range visible {
		visible[i] = ansi.Truncate(line, m.width, "")
	}
	view := tea.NewView(strings.Join(visible, "\n"))
	view.AltScreen = true
	if m.screen == "chat" && m.entry == "" && !m.showHelp && !m.showConfig && !m.slash.open {
		view.MouseMode = tea.MouseModeCellMotion
	}
	view.KeyboardEnhancements.ReportAlternateKeys = true
	view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	view.KeyboardEnhancements.ReportAssociatedText = true
	return view
}
