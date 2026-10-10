package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func richBotMessage(t *testing.T) message {
	t.Helper()
	var item message
	if err := json.Unmarshal([]byte(`{"id":"bot-message","channel_id":"10","author":{"id":"bot","username":"example-bot"},"content":"","embeds":[{"author":{"name":"검색 결과"},"title":"결과 제목","description":"첫째 줄\n둘째 줄","fields":[{"name":"상태","value":"완료"},{"name":"상세","value":"한 줄\n다음 줄"}],"footer":{"text":"푸터"},"image":{"url":"https://example.com/image.png"}}]}`), &item); err != nil {
		t.Fatal(err)
	}
	return item
}

func TestRichEmbedTextAcrossLayouts(t *testing.T) {
	item := richBotMessage(t)
	for _, style := range layoutStyles {
		m := openDemo(t)
		m.config.Layout.Style = style
		m.messages = []message{item}
		lines, _ := m.body()
		text := strings.Join(lines, "\n")
		for _, want := range []string{"검색 결과", "결과 제목", "첫째 줄", "둘째 줄", "상태: 완료", "상세: 한 줄", "다음 줄", "푸터", "https://example.com/image.png"} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s lost %s: %q", style, want, text)
			}
		}
		if strings.Contains(text, "[non-text message]") {
			t.Fatal("rich embed treated as empty")
		}
	}
}

func TestEmbedUpdatesPreserveAndReplaceData(t *testing.T) {
	m := openDemo(t)
	m.messages = []message{richBotMessage(t)}
	m.applyEvent(event("MESSAGE_UPDATE", map[string]any{"id": "bot-message", "channel_id": "10", "content": "추가 본문"}))
	if text := messageText(m.messages[0]); !strings.Contains(text, "추가 본문") || !strings.Contains(text, "결과 제목") {
		t.Fatal("content-only update discarded embed")
	}
	m.applyEvent(event("MESSAGE_UPDATE", map[string]any{"id": "bot-message", "channel_id": "10", "embeds": []map[string]any{{"description": "갱신된 결과"}}}))
	if text := messageText(m.messages[0]); !strings.Contains(text, "갱신된 결과") || strings.Contains(text, "결과 제목") {
		t.Fatal("embed update not replaced")
	}
	if m.messages[0].Author.Username != "example-bot" {
		t.Fatal("embed update discarded author")
	}
}

func TestEmbedURLsDeduplicateAndTextIsSanitized(t *testing.T) {
	item := message{Content: "https://example.com", Attachments: []link{{URL: "https://example.com"}}, Embeds: []embed{{URL: "https://example.com", Description: "visible\x1b[2J"}}}
	text := messageText(item)
	if strings.Count(text, "https://example.com") != 1 || strings.Contains(text, "\x1b") || !strings.Contains(text, "visible") {
		t.Fatal("duplicate URL or terminal escape rendered")
	}
	if messageText(message{}) != "[non-text message]" {
		t.Fatal("empty message fallback lost")
	}
	item = richBotMessage(t)
	m := openDemo(t)
	m.width = 25
	m.config.Layout.Style = "editor"
	m.messages = []message{item}
	lines, _ := m.body()
	for _, line := range lines {
		if ansi.StringWidth(line) > 25 {
			t.Fatal("rich embed broke wrapping")
		}
	}
}
