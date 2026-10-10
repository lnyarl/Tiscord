package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gorilla/websocket"
)

func TestSlashIndexAndInteractionProtocol(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "test-token" {
			t.Error("authentication missing")
		}
		switch r.URL.Path {
		case "/guilds/g/application-command-index":
			w.Write([]byte(`{"applications":[{"id":"a","name":"Bot"}],"application_commands":[{"id":"c","application_id":"a","version":"v","name":"manage","type":1,"options":[{"name":"group","type":2,"options":[{"name":"run","type":1,"options":[{"name":"flag","type":5}]}]}]},{"id":"context","type":2}]}`))
		case "/users/@me/application-command-index":
			w.Write([]byte(`{"applications":[],"application_commands":[]}`))
		case "/interactions":
			posts++
			var request interactionRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.SessionID != "session" || request.ChannelID != "ch" || request.Type != 2 || request.Data.Version != "v" {
				t.Error("interaction routing incorrect")
			}
			leaf := request.Data.Options[0].Options[0].Options[0]
			if leaf.Name != "flag" || leaf.Value != false {
				t.Error("nested false option lost")
			}
			w.WriteHeader(204)
		default:
			t.Error("unexpected endpoint " + r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c := newREST("test-token")
	c.base = server.URL
	commands, err := c.Commands(context.Background(), "g", "ch")
	if err != nil || len(commands) != 1 || commands[0].name() != "manage group run" {
		t.Fatalf("index failed: %v %+v", err, commands)
	}
	request := interactionRequest{Type: 2, ApplicationID: "a", ChannelID: "ch", SessionID: "session", Nonce: interactionNonce(), Data: commands[0].data(map[string]any{"flag": false}, "", nil)}
	if err := c.Interact(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatal("interaction posted more than once")
	}
}

func TestSlashPostFailureIsNotRetried(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { posts++; w.WriteHeader(500) }))
	defer server.Close()
	c := newREST("test-token")
	c.base = server.URL
	if c.Interact(context.Background(), interactionRequest{}) == nil || posts != 1 {
		t.Fatal("ambiguous POST retried")
	}
}

func TestSlashOptionValidation(t *testing.T) {
	minimum, maximum := float64(1), float64(5)
	for _, test := range []struct {
		option slashOption
		input  string
		valid  bool
	}{
		{slashOption{Type: 4, MinValue: &minimum, MaxValue: &maximum}, "3", true},
		{slashOption{Type: 4}, "1.5", false}, {slashOption{Type: 10}, "NaN", false},
		{slashOption{Type: 4, MinValue: &minimum}, "0", false},
		{slashOption{Type: 5}, "false", true}, {slashOption{Type: 5}, "yes", false},
		{slashOption{Type: 6}, "<@12345>", true}, {slashOption{Type: 7}, "general", false},
		{slashOption{Type: 11}, "file", false},
		{slashOption{Type: 3, Choices: []slashChoice{{Name: "Allowed", Value: "ok"}}}, "bad", false},
		{slashOption{Type: 3, Choices: []slashChoice{{Name: "Allowed", Value: "ok"}}}, "ok", true},
	} {
		if _, err := optionValue(test.option, test.input); (err == nil) != test.valid {
			t.Errorf("type %d/%s: %v", test.option.Type, test.input, err)
		}
	}
}

func TestSlashWizardPreservesDraftAndValidatesBeforePreview(t *testing.T) {
	m := openDemo(t)
	m.draft.set("keep draft")
	m.messageIndex = 0
	runCommand(m, m.key(tea.KeyPressMsg{Code: '/', Text: "/"}))
	if !m.slash.open || !strings.Contains(m.View().Content, "Discord 봇 슬래시 명령") || strings.Contains(m.View().Content, "keep draft") {
		t.Fatal("slash chooser did not open separately")
	}
	m.key(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.key(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.slash.optionIndex != 0 || !strings.Contains(m.slash.status, "입력") {
		t.Fatal("required argument skipped")
	}
	m.Update(tea.PasteMsg{Content: "hello"})
	m.key(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.key(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.slash.input.value() != "short" {
		t.Fatal("fixed choice completion failed")
	}
	m.key(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.slash.option() != nil || m.slash.submitting {
		t.Fatal("preview executed command")
	}
	request, err := m.slashRequest(2)
	if err != nil || request.Data.Options[0].Value != "hello" || request.Data.Options[1].Value != "short" {
		t.Fatal("preview payload incorrect")
	}
	m.closeSlash()
	if m.draft.value() != "keep draft" || m.messageIndex != 0 || m.mode != "NORMAL" {
		t.Fatal("wizard changed chat state")
	}
}

func TestSlashStaleResultsAndAutocompleteCorrelation(t *testing.T) {
	m := openDemo(t)
	load := m.openSlash()
	oldEpoch := m.slashEpoch
	m.closeSlash()
	m.openSlash()
	m.Update(load())
	if !m.slash.loading {
		t.Fatal("stale index changed new chooser")
	}
	if oldEpoch == m.slashEpoch {
		t.Fatal("epoch not advanced")
	}
	m.slash.loading = false
	root := appCommand{ID: "c", ApplicationID: "a", Version: "v", Name: "search", Type: 1, Options: []slashOption{{Type: 3, Name: "query", Autocomplete: true, Required: true}}}
	command := flattenCommands(root, nil, root.Options, "Bot")[0]
	m.slash.command = &command
	m.slash.input.set("abc")
	m.slash.autocompleteNonce = "current"
	m.slashEvent(event("APPLICATION_COMMAND_AUTOCOMPLETE_RESPONSE", map[string]any{"nonce": "old", "choices": []slashChoice{{Name: "wrong", Value: "wrong"}}}))
	if len(m.slash.choices) != 0 {
		t.Fatal("stale autocomplete accepted")
	}
	m.slashEvent(event("APPLICATION_COMMAND_AUTOCOMPLETE_RESPONSE", map[string]any{"nonce": "current", "choices": []slashChoice{{Name: "result", Value: "abc-result"}}}))
	m.slashKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.slash.input.value() != "abc-result" {
		t.Fatal("dynamic autocomplete choice not inserted")
	}
	request, err := m.slashRequest(4)
	if err != nil || !request.Data.Options[0].Focused || request.Data.Options[0].Value != "abc-result" {
		t.Fatal("focused autocomplete payload incorrect")
	}
	m.Update(slashLoaded{m.slashEpoch, nil, errors.New("failed")})
	if !strings.Contains(m.slash.status, "failed") {
		t.Fatal("index error hidden")
	}
}

func TestSlashExecutionGuardAndTimeout(t *testing.T) {
	m := openDemo(t)
	runCommand(m, m.openSlash())
	m.slashKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.slash.values["text"] = "hello"
	m.slash.optionIndex = len(m.slash.command.Options)
	m.gatewayOnline = false
	if m.submitSlash() != nil || m.slash.submitting {
		t.Fatal("disconnected execution allowed")
	}
	m.gatewayOnline = true
	if m.submitSlash() == nil || !m.slash.submitting {
		t.Fatal("execution not queued")
	}
	if m.slashKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}) != nil {
		t.Fatal("duplicate execution allowed")
	}
	m.Update(slashTimeout{m.slashEpoch, m.slash.executionNonce, false})
	if m.slash.submitting || !strings.Contains(m.slash.status, "확인") {
		t.Fatal("timeout did not preserve review")
	}
	m.closeSlash()
	m.draft.set("/demo text:hello")
	if m.send() != nil || m.draft.value() == "" {
		t.Fatal("slash text sent as normal chat")
	}
}

func TestGatewayForwardsSlashEventsAndSession(t *testing.T) {
	g, closeServer := localGateway(t, func(c *websocket.Conn) {
		socketSend(t, c, 10, "", 0, map[string]any{"heartbeat_interval": 10000})
		socketRead(t, c)
		socketSend(t, c, 0, "READY", 1, map[string]any{"session_id": "session"})
		for i, kind := range []string{"APPLICATION_COMMAND_AUTOCOMPLETE_RESPONSE", "INTERACTION_CREATE", "INTERACTION_SUCCESS", "INTERACTION_FAILURE"} {
			socketSend(t, c, 0, kind, i+2, map[string]any{"nonce": "n"})
		}
	})
	defer closeServer()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	g.connect(ctx)
	for i, kind := range []string{"", "APPLICATION_COMMAND_AUTOCOMPLETE_RESPONSE", "INTERACTION_CREATE", "INTERACTION_SUCCESS", "INTERACTION_FAILURE"} {
		select {
		case received := <-g.events:
			if received.kind != kind || i == 0 && received.sessionID != "session" {
				t.Fatal("session or slash event missing")
			}
		case <-ctx.Done():
			t.Fatal("slash event timeout")
		}
	}
}

func TestSlashFailureExplainsTimeoutAndRecordsCreation(t *testing.T) {
	m := openDemo(t)
	runCommand(m, m.openSlash())
	m.slashKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.slash.submitting = true
	m.slash.executionNonce = "n"
	m.slash.startedAt = time.Now()
	m.slashEvent(event("INTERACTION_CREATE", map[string]any{"nonce": "other"}))
	if m.slash.created {
		t.Fatal("unrelated interaction acknowledged")
	}
	m.slashEvent(event("INTERACTION_CREATE", map[string]any{"nonce": "n"}))
	if !m.slash.created || !m.slash.submitting {
		t.Fatal("creation treated as completion")
	}
	m.slashEvent(event("INTERACTION_FAILURE", map[string]any{"nonce": "n", "reason_code": 2}))
	if m.slash.submitting || !strings.Contains(m.slash.status, "시간 초과") || !m.slash.open {
		t.Fatal("timeout not explained or lost input")
	}
	if label := interactionFailureLabel(nil); !strings.Contains(label, "코드 없음") {
		t.Fatal("missing reason fabricated")
	}
}

func TestSlashSmallTerminalAndLiteralSlashMessage(t *testing.T) {
	m := openDemo(t)
	runCommand(m, m.openSlash())
	m.slashKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	for _, height := range []int{4, 8, 24} {
		m.height = height
		if len(strings.Split(m.View().Content, "\n")) != height {
			t.Fatal("slash overlay overflows terminal")
		}
	}
	m.closeSlash()
	m.draft.set("//home/example")
	runCommand(m, m.send())
	if m.messages[len(m.messages)-1].Content != "/home/example" {
		t.Fatal("literal slash message escape failed")
	}
}

func TestSlashAutocompleteDebounceAndExecutionSuccess(t *testing.T) {
	m := openDemo(t)
	runCommand(m, m.openSlash())
	m.slashKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.slash.command.Options[0].Autocomplete = true
	first := m.slashKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	second := m.slashKey(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if first == nil || second == nil {
		t.Fatal("autocomplete not scheduled")
	}
	_, cmd := m.Update(first())
	if cmd != nil || m.slash.autocompleteNonce != "" {
		t.Fatal("old debounce sent a request")
	}
	_, cmd = m.Update(second())
	if cmd == nil || m.slash.autocompleteNonce == "" {
		t.Fatal("latest input not requested")
	}
	m.slash.submitting = true
	m.slash.executionNonce = "execute"
	m.slashEvent(event("INTERACTION_SUCCESS", map[string]any{"nonce": "other"}))
	if !m.slash.open {
		t.Fatal("unrelated execution closed wizard")
	}
	m.slashEvent(event("INTERACTION_SUCCESS", map[string]any{"nonce": "execute"}))
	if m.slash.open || !strings.Contains(m.status, "완료") {
		t.Fatal("successful execution not shown")
	}
}
