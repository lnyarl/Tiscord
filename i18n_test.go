package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestLanguageDefaultsValidationAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"behavior":{"send_key":"enter"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil || c.Behavior.Language != "en" || c.Behavior.SendKey != "enter" {
		t.Fatal("old config did not default to English")
	}
	c.Behavior.Language = "fr"
	if c.validate() == nil {
		t.Fatal("unsupported language accepted")
	}
	m := openDemo(t)
	m.configPath = path
	m.execute("config")
	for i, field := range configFields {
		if field.field == "Language" {
			m.configIndex = i
			break
		}
	}
	m.configKey("right")
	if m.config.Behavior.Language != "en" {
		t.Fatal("language changed before save")
	}
	m.configKey("ctrl+s")
	saved, err := loadConfig(path)
	if err != nil || saved.Behavior.Language != "ko" || m.config.Behavior.Language != "ko" || !strings.Contains(m.status, "설정 저장") {
		t.Fatal("language not saved and applied immediately")
	}
}

func TestLanguagesTranslateUIAndPreserveUserContent(t *testing.T) {
	expected := map[string][3]string{
		"en":    {"Available commands", "Language: en", "한글이름 is typing…"},
		"ko":    {"사용 가능한 명령어", "언어: ko", "한글이름 입력 중…"},
		"ja":    {"利用可能なコマンド", "言語: ja", "한글이름 が入力中…"},
		"zh-CN": {"可用命令", "语言: zh-CN", "한글이름 正在输入…"},
		"es":    {"Comandos disponibles", "Idioma: es", "한글이름 está escribiendo…"},
		"de":    {"Verfügbare Befehle", "Sprache: de", "한글이름 schreibt…"},
	}
	for _, language := range supportedLanguages {
		t.Run(language, func(t *testing.T) {
			m := openDemo(t)
			m.config.Behavior.Language = language
			m.configDraft = m.config
			m.messages[0].Content = "한글 채팅 내용"
			m.messages[0].Author.Username = "한글이름"
			view := m.View().Content
			if !strings.Contains(view, "한글 채팅 내용") || !strings.Contains(view, "한글이름") {
				t.Fatal("user content translated or lost")
			}
			m.Update(event("TYPING_START", map[string]any{"channel_id": m.channel.ID, "user_id": m.messages[0].Author.ID}))
			wantHelp, wantConfig, wantTyping := expected[language][0], expected[language][1], expected[language][2]
			if !strings.Contains(strings.Join(m.helpLines(), "\n"), wantHelp) || !strings.Contains(strings.Join(m.configLines(), "\n"), wantConfig) || m.typingLabel(time.Now()) != wantTyping {
				t.Fatal("help, settings or typing language mismatch")
			}
			s := slashState{status: ""}
			if !strings.Contains(s.hint(language), localize(language, "Type to search · ↑↓/Tab: matches · Enter: select · Esc: close")) {
				t.Fatal("slash hint language mismatch")
			}
			for _, width := range []int{40, 80} {
				m.width = width
				m.execute("?")
				for _, line := range strings.Split(m.View().Content, "\n") {
					if ansi.StringWidth(line) > width {
						t.Fatal("translated help exceeds terminal width")
					}
				}
				m.showHelp = false
				m.execute("config")
				for _, line := range strings.Split(m.View().Content, "\n") {
					if ansi.StringWidth(line) > width {
						t.Fatal("translated settings exceed terminal width")
					}
				}
				m.showConfig = false
			}
		})
	}
}

func TestTranslationsPreserveFormatArgumentsAndFallback(t *testing.T) {
	formats := regexp.MustCompile(`%[-+0-9.]*[svd]`)
	for language, catalog := range messageCatalogs {
		if len(catalog) != len(koreanMessages) {
			t.Fatalf("incomplete %s catalog", language)
		}
		for source := range koreanMessages {
			translated, ok := catalog[source]
			if !ok || translated == "" {
				t.Fatalf("missing %s translation: %s", language, source)
			}
			if !slices.Equal(formats.FindAllString(source, -1), formats.FindAllString(translated, -1)) {
				t.Fatalf("format arguments changed in %s: %s", language, source)
			}
		}
	}
	if localize("ko", "untranslated") != "untranslated" || localize("ko", "%s and %d more are typing…", "alice", 2) != "alice 외 2명 입력 중…" {
		t.Fatal("fallback or formatting failed")
	}
}

func TestSettingsCycleAndPersistAllLanguages(t *testing.T) {
	m := openDemo(t)
	m.configPath = filepath.Join(t.TempDir(), "config.json")
	for _, language := range supportedLanguages[1:] {
		m.execute("config")
		for i, field := range configFields {
			if field.field == "Language" {
				m.configIndex = i
				break
			}
		}
		m.configKey("right")
		m.configKey("ctrl+s")
		saved, err := loadConfig(m.configPath)
		if err != nil || saved.Behavior.Language != language || m.config.Behavior.Language != language {
			t.Fatalf("language %s not saved and applied: %v", language, err)
		}
	}
	m.execute("config")
	for i, field := range configFields {
		if field.field == "Language" {
			m.configIndex = i
			break
		}
	}
	m.configKey("right")
	m.configKey("ctrl+s")
	if m.config.Behavior.Language != "en" {
		t.Fatal("language cycle did not wrap to English")
	}
}
