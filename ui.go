package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type loaded struct {
	kind, channelID string
	guilds          []guild
	channels        []channel
	messages        []message
	err             error
}
type sent struct {
	message message
	err     error
}
type streamClosed struct{}
type model struct {
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
	refreshWanted                                        bool
	watchReady                                           <-chan struct{}
	config                                               config
	showHelp                                             bool
	helpLine                                             int
	completions                                          []string
	completionIndex                                      int
}

func newModel(ctx context.Context, client chatClient, g *gateway) *model {
	connection := "Offline demo"
	if g != nil {
		connection = "Gateway connecting"
	}
	return &model{ctx: ctx, client: client, gateway: g, screen: "servers", mode: "NORMAL", status: "Loading", connection: connection, busy: "servers", width: 80, height: 24, drafts: map[string]editor{}, config: defaultConfig()}
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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(1, msg.Width)
		m.height = max(4, msg.Height)
	case streamClosed: // A terminal Gateway failure remains visible; there is no polling fallback.
	case gatewayEvent:
		if msg.status != "" {
			m.connection = msg.status
		}
		if msg.fresh && m.screen == "chat" {
			if m.busy == "" {
				return m, tea.Batch(m.loadHistory("chat"), m.nextEvent())
			}
			m.refreshWanted = true
		}
		if msg.kind != "" {
			if m.busy == "chat" || m.busy == "older" {
				m.pending = append(m.pending, msg)
			} else {
				m.applyEvent(msg)
			}
		}
		return m, m.nextEvent()
	case loaded:
		appLog.Debug("ui.loaded", "kind", msg.kind, "channel_id", msg.channelID, "servers", len(msg.guilds), "channels", len(msg.channels), "messages", len(msg.messages))
		m.busy = ""
		if msg.err != nil {
			appLog.Error("ui.load_failed", "kind", msg.kind, "error", msg.err.Error())
			m.status = msg.kind + ": " + msg.err.Error()
			m.pending = nil
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
		}
		if m.refreshWanted && m.screen == "chat" {
			m.refreshWanted = false
			return m, m.loadHistory("chat")
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
		if m.refreshWanted && m.screen == "chat" {
			m.refreshWanted = false
			return m, m.loadHistory("chat")
		}
	case tea.KeyMsg:
		return m, m.key(msg)
	}
	return m, nil
}
func (m *model) key(key tea.KeyMsg) tea.Cmd {
	name := key.String()
	if name == "ctrl+c" || name == "ctrl+q" {
		return tea.Quit
	}
	if name == "esc" {
		m.showHelp = false
		m.completions = nil
		m.mode = "NORMAL"
		m.entry = ""
		m.prefix = ""
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
	if !key.Paste && (name == "ctrl+s" || name == m.config.Behavior.SendKey && m.screen == "chat" && m.mode == "INSERT") {
		return m.send()
	}
	if m.mode == "INSERT" {
		if m.busy != "send" {
			m.draft.key(key, true)
		}
		return nil
	}
	if key.Paste {
		return nil
	}
	previous := m.prefix
	m.prefix = ""
	switch name {
	case "j", "down":
		if m.screen == "chat" {
			m.messageIndex = min(max(0, len(m.messages)-1), m.messageIndex+1)
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
	case ":":
		m.completions = nil
		m.entry = "command"
		m.command.set("")
	case "/":
		if m.screen != "chat" {
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
	m.saveDraft()
	m.channel = item
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
		data, err := m.client.Messages(m.ctx, id, before)
		return loaded{kind: kind, channelID: id, messages: data, err: err}
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
		m.saveDraft()
		m.channel = channel{}
		m.draft.set("")
		m.screen = command
		m.index = 0
		m.query = ""
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
	default:
		m.status = "Unknown command. :?"
	}
	return nil
}
func (m *model) send() tea.Cmd {
	if m.screen != "chat" || m.busy != "" || strings.TrimSpace(m.draft.value()) == "" {
		return nil
	}
	content := m.draft.value()
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
	following := m.messageIndex >= len(m.messages)-1
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
				json.Unmarshal(data, &m.messages[i])
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
	for i, item := range m.messages {
		grouped := false
		if i > 0 {
			previous := m.messages[i-1]
			gap := item.Timestamp.Sub(previous.Timestamp)
			grouped = m.config.Behavior.GroupMessages && item.Author.ID != "" && item.Author.ID == previous.Author.ID && !item.Timestamp.IsZero() && !previous.Timestamp.IsZero() && gap >= 0 && gap <= time.Duration(m.config.Behavior.GroupWindowSeconds)*time.Second
			if !grouped {
				for n := 0; n < m.config.Layout.GroupSpacing; n++ {
					lines = append(lines, "")
				}
			}
		}
		if i == m.messageIndex {
			target = len(lines)
		}
		name := item.Author.GlobalName
		if name == "" {
			name = item.Author.Username
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
		}
		indent := strings.Repeat(" ", min(ansi.StringWidth(groupPrefix), max(0, m.width-4)))
		prefix := groupPrefix
		if grouped {
			prefix = indent
		}
		content := clean(item.Content)
		for _, link := range append(append([]link{}, item.Attachments...), item.Embeds...) {
			if link.URL != "" && !strings.Contains(content, link.URL) {
				content += "\n" + singleLine(link.URL)
			}
		}
		if strings.TrimSpace(content) == "" {
			content = "[non-text message]"
		}
		content = strings.ReplaceAll(content, "\n", "\n"+indent)
		lines = append(lines, strings.Split(ansi.Hardwrap(prefix+content, m.width, true), "\n")...)
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
func (m *model) View() string {
	input, cursor := editorLines(m.draft, "> ", m.width)
	if m.entry != "" {
		prefix := ":"
		if m.entry == "search" {
			prefix = "/"
		}
		input, cursor = editorLines(m.command, prefix, m.width)
	}
	metaHeight := 0
	if m.config.Layout.ShowMetadata {
		metaHeight = 1
	}
	gapHeight := min(m.config.Layout.InputGapLines, max(0, m.height-metaHeight-2))
	inputHeight := min(m.config.Layout.InputMaxLines, min(len(input), max(1, m.height-metaHeight-gapHeight-1)))
	inputStart := max(0, min(cursor-inputHeight+1, len(input)-inputHeight))
	input = input[inputStart : inputStart+inputHeight]
	bodyHeight := max(1, m.height-inputHeight-gapHeight-metaHeight)
	body, target := m.body()
	start := max(0, min(target-bodyHeight+1, len(body)-bodyHeight))
	if !m.showHelp && m.screen == "chat" && m.messageIndex == len(m.messages)-1 {
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
	mode := m.mode
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
	return strings.Join(visible, "\n")
}
