package main

import (
	"fmt"
	"reflect"
	"slices"
)

type configField struct {
	section, field, label string
	choices               []string
	minimum, maximum      int
}

var configFields = []configField{
	{"Behavior", "RestoreUnread", "Start at the first unread message", nil, 0, 0},
	{"Layout", "Style", "Layout", layoutStyles, 0, 0},
	{"Layout", "AuthorName", "Author name", []string{"username", "display"}, 0, 0},
	{"Layout", "GroupSpacing", "Blank lines between message groups", nil, 0, 3},
	{"Layout", "ShowTimestamps", "Show timestamps", nil, 0, 0},
	{"Layout", "TimestampFormat", "Timestamp format", []string{"HH:mm", "HH:mm:ss", "YYYY-MM-DD HH:mm"}, 0, 0},
	{"Layout", "InputMaxLines", "Maximum input lines", nil, 1, 10},
	{"Layout", "InputGapLines", "Blank lines above input", nil, 0, 3},
	{"Layout", "ShowMetadata", "Show metadata", nil, 0, 0},
	{"Layout", "ShowTyping", "Show typing indicators", nil, 0, 0},
	{"Layout", "ShowActivity", "Show channel, server and DM activity", nil, 0, 0},
	{"Behavior", "GroupMessages", "Group consecutive messages", nil, 0, 0},
	{"Behavior", "GroupWindowSeconds", "Message grouping interval (seconds)", nil, 1, 86400},
	{"Behavior", "SendKey", "Send key", []string{"ctrl+s", "enter"}, 0, 0},
	{"Behavior", "StartMode", "Mode when opening a channel", []string{"normal", "insert"}, 0, 0},
	{"Behavior", "KoreanShortcuts", "Korean Vim shortcut conversion", nil, 0, 0},
	{"Logging", "Level", "Log level", []string{"debug", "info", "warn", "error", "off"}, 0, 0},
	{"Behavior", "Language", "Language", supportedLanguages, 0, 0},
}

func (f configField) value(c *config) reflect.Value {
	return reflect.ValueOf(c).Elem().FieldByName(f.section).FieldByName(f.field)
}

func (m *model) configKey(key string) {
	switch key {
	case "esc":
		m.showConfig = false
		m.status = m.tr("Settings changes cancelled")
	case "j", "down":
		m.configIndex = min(len(configFields)-1, m.configIndex+1)
	case "k", "up":
		m.configIndex = max(0, m.configIndex-1)
	case "g":
		m.configIndex = 0
	case "G":
		m.configIndex = len(configFields) - 1
	case "h", "left", "l", "right", "enter", "space":
		f := configFields[m.configIndex]
		v := f.value(&m.configDraft)
		direction := 1
		if key == "h" || key == "left" {
			direction = -1
		}
		switch v.Kind() {
		case reflect.Bool:
			v.SetBool(!v.Bool())
		case reflect.String:
			i := (slices.Index(f.choices, v.String()) + direction + len(f.choices)) % len(f.choices)
			v.SetString(f.choices[i])
		case reflect.Int:
			v.SetInt(int64(max(f.minimum, min(f.maximum, int(v.Int())+direction))))
		}
		m.configStatus = m.tr("Unsaved · Ctrl-S to save")
	case "ctrl+s":
		if m.configPath == "" {
			m.configStatus = m.tr("Could not save settings: no config file path")
			return
		}
		if err := saveConfig(m.configPath, m.configDraft); err != nil {
			m.configStatus = m.tr("Could not save settings: ") + m.tr(err.Error())
			return
		}
		m.config = m.configDraft
		if m.logLevel != nil {
			m.logLevel.Set(m.config.logLevel())
		}
		clear(m.typists)
		m.showConfig = false
		m.status = m.tr("Settings saved and applied")
	}
}

func (m *model) configLines() []string {
	lines := []string{m.tr("Settings · Ctrl-S to save and apply"), m.tr("j/k: move · h/l/Enter: change · Esc: cancel"), m.tr("Start mode applies the next time you open a channel"), m.configStatus}
	for i, f := range configFields {
		prefix := "  "
		if i == m.configIndex {
			prefix = "> "
		}
		v := f.value(&m.configDraft)
		value := fmt.Sprint(v.Interface())
		if v.Kind() == reflect.Bool {
			value = "OFF"
			if v.Bool() {
				value = "ON"
			}
		}
		lines = append(lines, fmt.Sprintf("%s%s: %s", prefix, m.tr(f.label), value))
	}
	return lines
}
