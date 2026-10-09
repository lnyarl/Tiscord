package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func typingEvent(channelID, userID, name string) gatewayEvent {
	return event("TYPING_START", map[string]any{"channel_id": channelID, "user_id": userID, "member": map[string]any{"user": map[string]any{"id": userID, "username": name}}})
}

func TestTypingShowsCurrentChannelAndIgnoresSelf(t *testing.T) {
	m := openDemo(t)
	m.selfUserID = "102"
	now := time.Now()
	m.receiveTyping(typingEvent("11", "100", "alice"), now)
	m.receiveTyping(typingEvent("10", "102", "me"), now)
	if len(m.typists) != 0 {
		t.Fatal("self or another channel is shown")
	}
	m.receiveTyping(typingEvent("10", "100", "alice"), now)
	if m.typingLabel(now) != "alice 입력 중…" || !strings.Contains(m.View().Content, "alice 입력 중…") {
		t.Fatal("typing indication missing")
	}
	m.config.Layout.ShowTyping = false
	if strings.Contains(m.View().Content, "입력 중…") {
		t.Fatal("typing setting ignored")
	}
}

func TestTypingExpiryDoesNotEraseRefreshedState(t *testing.T) {
	m := openDemo(t)
	now := time.Now()
	m.receiveTyping(typingEvent("10", "100", "alice"), now)
	old := m.typists["100"].expires
	m.receiveTyping(typingEvent("10", "100", "alice"), now.Add(time.Second))
	m.Update(typingExpired{"10", "100", old})
	if len(m.typists) != 1 {
		t.Fatal("old timeout erased newer typing event")
	}
	latest := m.typists["100"].expires
	m.Update(typingExpired{"10", "100", latest})
	if len(m.typists) != 0 {
		t.Fatal("typing did not expire")
	}
}

func TestTypingClearsAfterMessageChannelChangeOrDisconnect(t *testing.T) {
	m := openDemo(t)
	now := time.Now()
	m.receiveTyping(typingEvent("10", "100", "alice"), now)
	m.Update(event("MESSAGE_CREATE", message{ID: "3", ChannelID: "10", Author: author{ID: "100"}}))
	if len(m.typists) != 0 {
		t.Fatal("sending a message did not clear typing")
	}
	m.receiveTyping(typingEvent("10", "100", "alice"), now)
	m.execute("channels")
	if len(m.typists) != 0 {
		t.Fatal("changing channels retained typing")
	}
	runCommand(m, m.selectItem())
	m.receiveTyping(typingEvent("10", "100", "alice"), now)
	m.Update(gatewayEvent{status: "Gateway disconnected; reconnecting"})
	if len(m.typists) != 0 {
		t.Fatal("disconnect retained typing")
	}
}

func TestTypingNamesMultipleUsersAndSmallTerminal(t *testing.T) {
	m := openDemo(t)
	now := time.Now()
	for _, user := range []struct{ id, name string }{{"103", "carol"}, {"100", "alice"}, {"101", "bob"}} {
		m.receiveTyping(typingEvent("10", user.id, user.name), now)
	}
	if m.typingLabel(now) != "alice, bob 외 1명 입력 중…" {
		t.Fatal(m.typingLabel(now))
	}
	m.height = 4
	m.config.Layout.InputGapLines = 3
	m.draft.set("one\ntwo\nthree")
	if len(strings.Split(m.View().Content, "\n")) != 4 {
		t.Fatal("typing row overflows small terminal")
	}
}

func TestOldTypingEventIsIgnoredAndNicknamePreferred(t *testing.T) {
	m := openDemo(t)
	m.config.Layout.AuthorName = "display"
	now := time.Now()
	old := event("TYPING_START", map[string]any{"channel_id": "10", "user_id": "100", "timestamp": now.Add(-20 * time.Second).Unix()})
	if m.receiveTyping(old, now) != nil || len(m.typists) != 0 {
		t.Fatal("replayed stale typing was displayed")
	}
	e := event("TYPING_START", map[string]any{"channel_id": "10", "user_id": "100", "member": map[string]any{"nick": "별명", "user": map[string]any{"username": "alice", "global_name": "Alice"}}})
	m.receiveTyping(e, now)
	if m.typingLabel(now) != "별명 입력 중…" {
		t.Fatal("nickname was not used")
	}
}

func TestTypingTimeoutCommandSchedulesExpiry(t *testing.T) {
	m := openDemo(t)
	now := time.Now()
	e := event("TYPING_START", map[string]any{"channel_id": "10", "user_id": "100", "timestamp": now.Add(-9 * time.Second).Unix()})
	command := m.receiveTyping(e, now)
	if command == nil {
		t.Fatal("expiry was not scheduled")
	}
	runCommand(m, command)
	if len(m.typists) != 0 {
		t.Fatal("scheduled timeout did not clear typing")
	}
}

func TestTypingPrefersUsernameAndUsesCachedUsername(t *testing.T) {
	for _, includeUsername := range []bool{true, false} {
		m := openDemo(t)
		m.messages[0].Author = author{ID: "100", Username: "alice", GlobalName: "한글별명"}
		user := map[string]any{"global_name": "한글별명"}
		if includeUsername {
			user["username"] = "alice"
		}
		e := event("TYPING_START", map[string]any{"channel_id": "10", "user_id": "100", "member": map[string]any{"nick": "서버별명", "user": user}})
		now := time.Now()
		m.receiveTyping(e, now)
		if m.typingLabel(now) != "alice 입력 중…" {
			t.Fatalf("username missing (included=%t): %s", includeUsername, m.typingLabel(now))
		}
	}
}

func TestGatewayForwardsTypingAndSelfIdentity(t *testing.T) {
	g, closeServer := localGateway(t, func(c *websocket.Conn) {
		socketSend(t, c, 10, "", 0, map[string]any{"heartbeat_interval": 10000})
		socketRead(t, c)
		socketSend(t, c, 0, "READY", 1, map[string]any{"session_id": "session", "user": map[string]any{"id": "102"}})
		socketSend(t, c, 0, "TYPING_START", 2, map[string]any{"channel_id": "10", "user_id": "100"})
	})
	defer closeServer()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	g.connect(ctx)
	select {
	case ready := <-g.events:
		if ready.selfUserID != "102" {
			t.Fatal("self identity not forwarded")
		}
	case <-ctx.Done():
		t.Fatal("missing READY")
	}
	select {
	case typed := <-g.events:
		if typed.kind != "TYPING_START" {
			t.Fatal("typing not forwarded")
		}
	case <-ctx.Done():
		t.Fatal("missing typing event")
	}
}
