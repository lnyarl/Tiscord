package main

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
)

// Read backwards from the newest page until the entire cached range is covered.
// A single latest-page fetch can lose messages after a long disconnection.
func syncHistory(ctx context.Context, client chatClient, channelID, boundary string) ([]message, error) {
	var result []message
	before := ""
	for {
		page, err := client.Messages(ctx, channelID, before)
		if err != nil {
			return nil, err
		}
		result = mergeMessages(result, page)
		if len(page) < 50 {
			return result, nil
		}
		oldest := page[0].ID
		for _, item := range page {
			if snowflakeLess(item.ID, oldest) {
				oldest = item.ID
			}
		}
		if !snowflakeLess(boundary, oldest) {
			return result, nil
		}
		if before != "" && !snowflakeLess(oldest, before) {
			return nil, errors.New("History pagination did not advance")
		}
		before = oldest
	}
}

func (m *model) startSync() tea.Cmd {
	if m.screen != "chat" || m.busy != "" || !m.gatewayOnline {
		return nil
	}
	m.syncRequired = true
	m.busy = "sync"
	m.status = m.tr("Recovering messages")
	m.pending = nil
	id, epoch := m.channel.ID, m.connectionEpoch
	ready := m.watchReady
	boundary := "0"
	if len(m.messages) > 0 {
		boundary = m.messages[0].ID
	}
	if m.unreadAwaiting && m.config.Behavior.RestoreUnread {
		readID := m.readIDs[id]
		if readID != "" && readID != "0" && snowflakeLess(readID, boundary) {
			boundary = readID
		}
	}
	appLog.Info("ui.sync_started", "channel_id", id, "boundary", boundary, "epoch", epoch)
	return func() tea.Msg {
		if ready != nil {
			select {
			case <-ready:
			case <-m.gateway.stopped:
				return loaded{kind: "sync", channelID: id, epoch: epoch, err: errors.New("Gateway unavailable; recovery cancelled")}
			case <-m.ctx.Done():
				return loaded{kind: "sync", channelID: id, epoch: epoch, err: m.ctx.Err()}
			}
		}
		data, err := syncHistory(m.ctx, m.client, id, boundary)
		return loaded{kind: "sync", channelID: id, messages: data, err: err, epoch: epoch}
	}
}

func (m *model) maybeSync() tea.Cmd {
	if m.syncRequired && m.gatewayOnline && !m.gatewayStopped && m.screen == "chat" && m.busy == "" {
		return m.startSync()
	}
	return nil
}

func (m *model) connectionHint() string {
	if m.gateway == nil {
		return ""
	}
	if m.gatewayStopped {
		return m.tr("Connection stopped · sending unavailable (restart the app)")
	}
	if !m.gatewayOnline {
		if m.hadConnection {
			return m.tr("Disconnected · reconnecting · sending unavailable")
		}
		return m.tr("Connecting · sending unavailable")
	}
	if m.syncRequired {
		if m.busy == "sync" {
			return m.tr("Recovering messages · sending unavailable")
		}
		return m.tr("Message recovery needed · sending unavailable (retry with :sync)")
	}
	return ""
}
