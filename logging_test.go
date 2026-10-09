package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDetailedLogsOmitCredentialsAndMessageBodies(t *testing.T) {
	var output bytes.Buffer
	previous := appLog
	appLog = slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	defer func() { appLog = previous }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"code":50035,"message":"private-token message-secret","errors":{"content":{"_errors":[{"code":"BASE_TYPE_MAX_LENGTH","message":"message-secret"}]}}}`)
	}))
	defer server.Close()
	c := newREST("private-token")
	c.base = server.URL
	_, err := c.Send(context.Background(), "10", "message-secret")
	if err == nil {
		t.Fatal("expected rejection")
	}
	m := openDemo(t)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("input-secret")})
	m.applyEvent(event("MESSAGE_CREATE", message{ID: "3", ChannelID: "10", Content: "message-secret"}))
	text := output.String()
	for _, secret := range []string{"private-token", "message-secret", "input-secret", "Authorization"} {
		if strings.Contains(text, secret) {
			t.Fatalf("private input appeared in logs: %s", secret)
		}
	}
	for _, expected := range []string{"http.request", "http.response", "http.rejected", "BASE_TYPE_MAX_LENGTH", "ui.state", "ui.message_applied", "request_id"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("diagnostic missing: %s", expected)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) != nil {
			t.Fatal("invalid JSON Lines log")
		}
	}
}

func TestLogFileAppendsAcrossRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiscord", "tiscord.log")
	for _, event := range []string{"first_run", "second_run"} {
		file, logger, err := openLog(path, slog.LevelDebug)
		if err != nil {
			t.Fatal(err)
		}
		logger.Debug(event)
		file.Close()
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "first_run") || !strings.Contains(string(data), "second_run") {
		t.Fatal("logs did not survive restart")
	}
}

func TestNetworkErrorRedactsSecrets(t *testing.T) {
	text := safeError(errors.New("private-token session-secret"), "private-token", "session-secret")
	if strings.Contains(text, "private-token") || strings.Contains(text, "session-secret") {
		t.Fatal("secret not redacted")
	}
}
