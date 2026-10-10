package main

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	tea "charm.land/bubbletea/v2"
)

type readState struct {
	ChannelID     string `json:"id"`
	LastMessageID string `json:"last_message_id"`
	Type          int    `json:"read_state_type"`
}

func (s *readState) UnmarshalJSON(data []byte) error {
	var raw struct {
		ChannelID     string          `json:"id"`
		LastMessageID json.RawMessage `json:"last_message_id"`
		Type          int             `json:"read_state_type"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.ChannelID, s.Type = raw.ChannelID, raw.Type
	if len(raw.LastMessageID) == 0 || string(raw.LastMessageID) == "null" {
		s.LastMessageID = ""
		return nil
	}
	if err := json.Unmarshal(raw.LastMessageID, &s.LastMessageID); err == nil {
		return nil
	}
	if _, err := strconv.ParseUint(string(raw.LastMessageID), 10, 64); err != nil {
		return err
	}
	s.LastMessageID = string(raw.LastMessageID)
	return nil
}

func decodeReadStates(raw json.RawMessage) []readState {
	var state struct {
		Entries []readState `json:"entries"`
	}
	if json.Unmarshal(raw, &state) == nil {
		return state.Entries
	}
	var entries []readState
	json.Unmarshal(raw, &entries)
	return entries
}

type readClient interface {
	MarkRead(context.Context, string, string) error
}

func (c *restClient) MarkRead(ctx context.Context, channelID, messageID string) error {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	var result struct {
		Token *string `json:"token"`
	}
	err := c.request(ctx, "POST", "/channels/"+url.PathEscape(channelID)+"/messages/"+url.PathEscape(messageID)+"/ack", map[string]any{"token": c.readToken}, &result)
	if err != nil {
		return err
	}
	c.readToken = result.Token
	return nil
}
func (c *demoClient) MarkRead(context.Context, string, string) error { return nil }

type readMarked struct {
	channelID, messageID string
	err                  error
}

func (m *model) markRead() tea.Cmd {
	if m.screen != "chat" || len(m.messages) == 0 || m.busy != "" {
		return nil
	}
	if !m.gatewayOnline || m.gatewayStopped || m.syncRequired {
		m.status = "연결 복구 후 읽음 처리가 가능합니다"
		return nil
	}
	id := m.messages[m.messageIndex].ID
	return m.markReadID(m.channel.ID, id)
}
func (m *model) markReadID(channelID, id string) tea.Cmd {
	if !m.gatewayOnline || m.gatewayStopped || m.syncRequired {
		return nil
	}
	client, ok := m.client.(readClient)
	if !ok {
		return nil
	}
	if current := m.readIDs[channelID]; current != "" && !snowflakeLess(current, id) {
		return nil
	}
	if pending := m.readPending[channelID]; pending != "" {
		if snowflakeLess(pending, id) && (m.readWanted[channelID] == "" || snowflakeLess(m.readWanted[channelID], id)) {
			m.readWanted[channelID] = id
		}
		return nil
	}
	m.readPending[channelID] = id
	m.status = "읽음 위치 저장 중"
	return func() tea.Msg { return readMarked{channelID, id, client.MarkRead(m.ctx, channelID, id)} }
}
func (m *model) receiveReadState(event gatewayEvent) bool {
	if event.fresh {
		clear(m.readIDs)
		for _, state := range event.readStates {
			if state.Type == 0 && state.ChannelID != "" {
				m.readIDs[state.ChannelID] = state.LastMessageID
			}
		}
	}
	if event.kind != "MESSAGE_ACK" {
		return false
	}
	var data struct {
		ChannelID string `json:"channel_id"`
		MessageID string `json:"message_id"`
		Type      int    `json:"read_state_type"`
	}
	if json.Unmarshal(event.data, &data) == nil && data.Type == 0 && data.ChannelID != "" && data.MessageID != "" {
		m.readIDs[data.ChannelID] = data.MessageID
		if data.ChannelID == m.channel.ID {
			m.unreadBoundary = data.MessageID
			if m.firstUnread() < 0 {
				m.unreadPinned = false
			}
		}
	}
	return true
}
func (m *model) firstUnread() int {
	if m.unreadBoundary == "" {
		return -1
	}
	for i, item := range m.messages {
		if snowflakeLess(m.unreadBoundary, item.ID) {
			return i
		}
	}
	return -1
}
func (m *model) restoreUnread() {
	if m.unreadBoundary == "0" {
		m.unreadBoundary = ""
	}
	m.unreadPinned = false
	if index := m.firstUnread(); index >= 0 && m.config.Behavior.RestoreUnread {
		m.messageIndex = index
		m.unreadPinned = true
	}
}
