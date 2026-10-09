package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestLayoutStylesAndPreview(t *testing.T) {
	m := openDemo(t)
	stamp := time.Date(2026, 10, 9, 14, 32, 0, 0, time.Local)
	m.messages = []message{
		{ID: "1", Timestamp: stamp, Author: author{ID: "100", Username: "alice"}, Content: "오늘 배포 언제 해?"},
		{ID: "2", Timestamp: stamp.Add(time.Minute), Author: author{ID: "100", Username: "alice"}, Content: "세 시쯤 가능해?"},
		{ID: "3", Timestamp: stamp.Add(2 * time.Minute), Author: author{ID: "101", Username: "bob"}, Content: "응, 가능해"},
	}
	m.draft.set("작성 중")
	m.messageIndex = 1
	for _, style := range layoutStyles {
		m.execute("layout " + style)
		lines, target := m.body()
		text := strings.Join(lines, "\n")
		if strings.Count(text, "alice") != 1 || !strings.Contains(lines[target], "세 시쯤 가능해?") {
			t.Fatalf("style %s lost grouping or target: %q", style, lines)
		}
		if m.draft.value() != "작성 중" || m.messageIndex != 1 || len(m.messages) != 3 {
			t.Fatal("style switch changed chat state")
		}
		switch style {
		case "plain":
			if !strings.HasPrefix(lines[0], "14:32 alice") {
				t.Fatal("plain changed")
			}
		case "log":
			if !strings.HasPrefix(lines[0], "[14:32] INFO") {
				t.Fatal("log header missing")
			}
		case "editor":
			if !strings.HasPrefix(lines[0], "   1  // 14:32 alice") {
				t.Fatal("editor gutter missing")
			}
		}
		fmt.Printf("\n[%s]\n%s\n", style, text)
	}
}

func TestLayoutCycleAndArgumentCompletion(t *testing.T) {
	m := openDemo(t)
	for _, expected := range []string{"log", "editor", "plain"} {
		m.execute("layout")
		if m.config.Layout.Style != expected {
			t.Fatal("layout cycle failed")
		}
	}
	runeKey(m, ":layout ")
	for _, expected := range []string{"layout plain", "layout log", "layout editor", "layout plain"} {
		m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if m.command.value() != expected {
			t.Fatalf("want %s got %s", expected, m.command.value())
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.execute("layout bad")
	if m.config.Layout.Style != "plain" {
		t.Fatal("invalid layout changed configuration")
	}
	c := defaultConfig()
	c.Layout.Style = "bad"
	if c.validate() == nil {
		t.Fatal("invalid style accepted")
	}
}

func TestEditorLayoutWrapsWithoutLosingText(t *testing.T) {
	m := openDemo(t)
	m.config.Layout.Style = "editor"
	m.width = 30
	m.messages[0].Content = strings.Repeat("한글", 30)
	lines, _ := m.body()
	count := 0
	for _, line := range lines {
		if ansi.StringWidth(line) > m.width {
			t.Fatal("editor gutter broke wrapping")
		}
		count += strings.Count(line, "한글")
	}
	if count != 30 {
		t.Fatal("editor wrapping lost message content")
	}
}

func TestAuthorNameAcrossLayouts(t *testing.T) {
	m := openDemo(t)
	m.messages = []message{{ID: "1", Author: author{ID: "100", Username: "alice", GlobalName: "한글별명"}, Content: "hello"}}
	for _, style := range layoutStyles {
		m.config.Layout.Style = style
		for _, choice := range []string{"username", "display"} {
			m.config.Layout.AuthorName = choice
			lines, _ := m.body()
			text := strings.Join(lines, "\n")
			want, unwanted := "alice", "한글별명"
			if choice == "display" {
				want, unwanted = unwanted, want
			}
			if !strings.Contains(text, want) || strings.Contains(text, unwanted) {
				t.Fatalf("%s/%s: %q", style, choice, text)
			}
		}
	}
	m.config.Layout.AuthorName = "username"
	m.messages[0].Author.Username = ""
	lines, _ := m.body()
	if text := strings.Join(lines, "\n"); !strings.Contains(text, "100") || strings.Contains(text, "한글별명") {
		t.Fatal("missing username did not fall back to user ID")
	}
	c := defaultConfig()
	c.Layout.AuthorName = "bad"
	if c.validate() == nil {
		t.Fatal("invalid author name setting accepted")
	}
}

func TestLayoutSelectionPersistsAndPreservesOtherSettings(t *testing.T) {
	m := openDemo(t)
	m.configPath = filepath.Join(t.TempDir(), "custom.json")
	// Disk settings can differ from the running session after a manual edit.
	if err := os.WriteFile(m.configPath, []byte(`{"behavior":{"send_key":"enter"},"layout":{"author_name":"display","show_typing":false},"logging":{"level":"warn"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"layout editor", "layout"} {
		m.execute(command)
		saved, err := loadConfig(m.configPath)
		if err != nil {
			t.Fatal(err)
		}
		if saved.Layout.Style != m.config.Layout.Style || saved.Layout.AuthorName != "display" || saved.Layout.ShowTyping || saved.Behavior.SendKey != "enter" || saved.Logging.Level != "warn" {
			t.Fatalf("selected layout or other settings lost: %+v", saved)
		}
		if !strings.Contains(m.status, "저장됨") {
			t.Fatal("save success not shown")
		}
	}
	before, _ := os.ReadFile(m.configPath)
	m.execute("layout bad")
	after, _ := os.ReadFile(m.configPath)
	if string(before) != string(after) {
		t.Fatal("invalid layout changed saved settings")
	}
	files, _ := os.ReadDir(filepath.Dir(m.configPath))
	if len(files) != 1 {
		t.Fatal("temporary settings file left behind")
	}
}

func TestLayoutSaveFailureKeepsSelectionAndReportsError(t *testing.T) {
	m := openDemo(t)
	m.configPath = filepath.Join(t.TempDir(), "missing.json")
	m.execute("layout log")
	if m.config.Layout.Style != "log" || !strings.Contains(m.status, "설정 저장 실패") {
		t.Fatal("failed save did not retain current layout and report error")
	}
}
