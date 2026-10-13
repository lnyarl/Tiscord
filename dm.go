package main

import (
	"encoding/json"
	"sort"
	"strings"
)

func (m *model) dmName(item channel) string {
	if item.Type == 3 && item.Name != "" {
		return item.Name
	}
	var names []string
	for _, user := range item.Recipients {
		if user.ID != m.selfUserID {
			names = append(names, m.config.authorName(user))
		}
	}
	if len(names) > 0 {
		return strings.Join(names, ", ")
	}
	if item.Name != "" {
		return item.Name
	}
	return item.ID
}

func (m *model) dmChannels() []channel {
	items := append([]channel{}, m.privateChannels...)
	for i := range items {
		items[i].Name = m.dmName(items[i])
	}
	sort.SliceStable(items, func(i, j int) bool { return snowflakeLess(items[j].LastMessageID, items[i].LastMessageID) })
	return items
}

func (m *model) observePrivateChannels(event gatewayEvent) {
	if event.fresh {
		m.privateChannels = nil
		for _, item := range event.channels {
			if item.Type == 1 || item.Type == 3 {
				m.privateChannels = append(m.privateChannels, item)
			}
		}
		return
	}
	if event.kind != "CHANNEL_CREATE" && event.kind != "CHANNEL_UPDATE" && event.kind != "CHANNEL_DELETE" && event.kind != "MESSAGE_CREATE" {
		return
	}
	var identity struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
	}
	if json.Unmarshal(event.data, &identity) != nil {
		return
	}
	id := identity.ID
	if event.kind == "MESSAGE_CREATE" {
		id = identity.ChannelID
	}
	if id == "" {
		return
	}
	index := -1
	for i, item := range m.privateChannels {
		if item.ID == id {
			index = i
			break
		}
	}
	if event.kind == "CHANNEL_DELETE" {
		if index >= 0 {
			m.privateChannels = append(m.privateChannels[:index], m.privateChannels[index+1:]...)
			delete(m.activity, id)
		}
		return
	}
	var item channel
	if index >= 0 {
		item = m.privateChannels[index]
	}
	if event.kind == "MESSAGE_CREATE" {
		if index < 0 {
			return
		}
		if snowflakeLess(item.LastMessageID, identity.ID) {
			item.LastMessageID = identity.ID
		}
	} else {
		if json.Unmarshal(event.data, &item) != nil || item.Type != 1 && item.Type != 3 {
			return
		}
	}
	if index < 0 {
		m.privateChannels = append(m.privateChannels, item)
	} else {
		m.privateChannels[index] = item
	}
	if m.channel.ID == item.ID {
		m.channel = item
	}
	if event.kind != "MESSAGE_CREATE" {
		m.registerActivityChannel(item)
	}
}
