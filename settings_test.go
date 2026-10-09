package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestConfigScreenEditsAllFieldsWithinValidRanges(t *testing.T) {
	m := openDemo(t)
	m.execute("config")
	for i, f := range configFields {
		m.configIndex = i
		before := f.value(&m.configDraft).Interface()
		m.key(tea.KeyPressMsg{Code: 'l', Text: "l"})
		if reflect.DeepEqual(before, f.value(&m.configDraft).Interface()) {
			t.Fatalf("field %s did not change", f.field)
		}
		if err := m.configDraft.validate(); err != nil {
			t.Fatal(err)
		}
		m.key(tea.KeyPressMsg{Code: 'h', Text: "h"})
		if !reflect.DeepEqual(before, f.value(&m.configDraft).Interface()) {
			t.Fatalf("field %s did not reverse", f.field)
		}
		if f.value(&m.configDraft).Kind() == reflect.Int {
			f.value(&m.configDraft).SetInt(int64(f.minimum))
			m.configKey("h")
			if f.value(&m.configDraft).Int() != int64(f.minimum) {
				t.Fatal("below minimum")
			}
			f.value(&m.configDraft).SetInt(int64(f.maximum))
			m.configKey("l")
			if f.value(&m.configDraft).Int() != int64(f.maximum) {
				t.Fatal("above maximum")
			}
		}
	}
}

func TestConfigCancelPreservesChatAndBlocksPaste(t *testing.T) {
	m := openDemo(t)
	m.draft.set("keep draft")
	m.messageIndex = 0
	original := m.config
	m.execute("config")
	m.configKey("l")
	m.Update(tea.PasteMsg{Content: "unwanted"})
	m.configKey("esc")
	if m.showConfig || m.config != original || m.draft.value() != "keep draft" || m.messageIndex != 0 || m.screen != "chat" {
		t.Fatal("cancel modified chat or active config")
	}
}

func TestConfigSaveAppliesAndPersistsSettings(t *testing.T) {
	m := openDemo(t)
	m.configPath = filepath.Join(t.TempDir(), "config.json")
	var level slog.LevelVar
	m.logLevel = &level
	m.execute("config")
	m.configDraft.Layout.Style = "editor"
	m.configDraft.Layout.ShowMetadata = false
	m.configDraft.Logging.Level = "warn"
	m.key(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	saved, err := loadConfig(m.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if m.showConfig || saved != m.config || level.Level() != slog.LevelWarn || m.busy == "send" {
		t.Fatal("save did not apply config or sent chat draft")
	}
}

func TestConfigSaveFailureRemainsEditableAndVisible(t *testing.T) {
	m := openDemo(t)
	original := m.config
	m.configPath = filepath.Join(t.TempDir(), "missing", "config.json")
	m.execute("config")
	m.configKey("l")
	m.configKey("G")
	m.configKey("ctrl+s")
	if !m.showConfig || m.config != original || !strings.Contains(m.View().Content, "설정 저장 실패") {
		t.Fatal("failed save lost editable state or error")
	}
	if _, err := os.Stat(m.configPath); !os.IsNotExist(err) {
		t.Fatal("unexpected file created")
	}
	for _, height := range []int{4, 8, 24} {
		m.height = height
		if len(strings.Split(m.View().Content, "\n")) != height {
			t.Fatal("config screen overflow")
		}
	}
}
