package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestConfigCreatesDefaultsAndPreservesPartialOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiscord", "config.json")
	c, err := loadConfig(path)
	if err != nil || c != defaultConfig() {
		t.Fatalf("default config failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved config
	if json.Unmarshal(data, &saved) != nil || saved != c {
		t.Fatal("generated config differs from defaults")
	}
	override := []byte(`{"layout":{"show_timestamps":false,"group_spacing":1},"behavior":{"send_key":"enter"}}`)
	if err := os.WriteFile(path, override, 0600); err != nil {
		t.Fatal(err)
	}
	c, err = loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Layout.ShowTimestamps || c.Layout.GroupSpacing != 1 || c.Behavior.SendKey != "enter" || c.Layout.InputMaxLines != 3 || !c.Behavior.GroupMessages {
		t.Fatal("partial settings lost defaults")
	}
	data, _ = os.ReadFile(path)
	if string(data) != string(override) {
		t.Fatal("existing config was overwritten")
	}
}

func TestInvalidConfigIsRejected(t *testing.T) {
	for _, data := range []string{`null`, `[]`, `{`, `{} {}`, `{"layout":{"group_spcaing":1}}`, `{"layout":{"input_max_lines":0}}`, `{"layout":{"input_gap_lines":4}}`, `{"layout":{"timestamp_format":"bad"}}`, `{"layout":{"group_spacing":-1}}`, `{"behavior":{"group_window_seconds":0}}`, `{"behavior":{"send_key":"ctrl+c"}}`, `{"behavior":{"start_mode":"bad"}}`, `{"logging":{"level":"verbose"}}`} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			os.WriteFile(path, []byte(data), 0600)
			if _, err := loadConfig(path); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestConfiguredLayoutAndGrouping(t *testing.T) {
	m := openDemo(t)
	stamp := time.Date(2026, 10, 9, 14, 32, 0, 0, time.Local)
	m.messages = []message{{ID: "1", Timestamp: stamp, Author: author{ID: "100", Username: "alice"}, Content: "first"}, {ID: "2", Timestamp: stamp.Add(2 * time.Minute), Author: author{ID: "100", Username: "alice"}, Content: "second"}}
	m.config.Layout.ShowTimestamps = false
	m.config.Layout.GroupSpacing = 1
	m.config.Behavior.GroupWindowSeconds = 60
	lines, _ := m.body()
	if len(lines) != 3 || lines[1] != "" || strings.Contains(strings.Join(lines, "\n"), "14:32") || strings.Count(strings.Join(lines, "\n"), "alice") != 2 {
		t.Fatalf("settings not applied: %q", lines)
	}
	m.config.Behavior.GroupWindowSeconds = 300
	lines, _ = m.body()
	if len(lines) != 2 || strings.Count(strings.Join(lines, "\n"), "alice") != 1 {
		t.Fatal("group window not applied")
	}
	m.config.Behavior.GroupMessages = false
	lines, _ = m.body()
	if strings.Count(strings.Join(lines, "\n"), "alice") != 2 {
		t.Fatal("group toggle not applied")
	}
	m.config.Layout.ShowTimestamps = true
	m.config.Layout.TimestampFormat = "HH:mm:ss"
	lines, _ = m.body()
	if !strings.HasPrefix(lines[0], "14:32:00") {
		t.Fatal("timestamp format not applied")
	}
}

func TestConfiguredInputHeightAndMetadata(t *testing.T) {
	m := openDemo(t)
	m.draft.set("one\ntwo\nthree\nfour")
	m.config.Layout.InputMaxLines = 2
	m.config.Layout.InputGapLines = 0
	m.config.Layout.ShowMetadata = false
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	if len(rows) != m.height || rows[len(rows)-2] != "three" || rows[len(rows)-1] != "four▏" || strings.Contains(strings.Join(rows, "\n"), "NORMAL ·") {
		t.Fatalf("configured input layout failed: %q", rows)
	}
	for _, height := range []int{4, 5, 10} {
		m.height = height
		m.config.Layout.InputMaxLines = 10
		m.config.Layout.InputGapLines = 3
		m.config.Layout.ShowMetadata = true
		if len(strings.Split(m.View(), "\n")) != height {
			t.Fatal("small terminal overflow")
		}
	}
}

func TestConfiguredEnterSendAndInitialInsertMode(t *testing.T) {
	m := openDemo(t)
	m.execute("channels")
	m.config.Behavior.StartMode = "insert"
	runCommand(m, m.selectItem())
	if m.mode != "INSERT" {
		t.Fatal("initial mode not applied")
	}
	m.config.Behavior.SendKey = "enter"
	runeKey(m, "hello")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	runeKey(m, "world")
	if m.draft.value() != "hello\nworld" {
		t.Fatal("alternative newline failed")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runCommand(m, cmd)
	if m.draft.value() != "" || m.messages[len(m.messages)-1].Content != "hello\nworld" {
		t.Fatal("Enter send not applied")
	}
	m.execute("channels")
	m.mode = "NORMAL"
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runCommand(m, cmd)
	if m.screen != "chat" {
		t.Fatal("Enter send intercepted channel selection")
	}
}

func TestConfiguredLogLevelFiltersDebug(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	c := defaultConfig()
	c.Logging.Level = "info"
	file, logger, err := openLog(path, c.logLevel())
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("hidden_debug")
	logger.Info("visible_info")
	file.Close()
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "hidden_debug") || !strings.Contains(string(data), "visible_info") {
		t.Fatal("log level was not applied")
	}
}

func TestExampleConfigMatchesDefaults(t *testing.T) {
	c, err := loadConfig("config.example.json")
	if err != nil || c != defaultConfig() {
		t.Fatalf("example and defaults differ: %v", err)
	}
}
