package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func localGateway(t *testing.T, handler func(*websocket.Conn)) (*gateway, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer c.Close()
		handler(c)
	}))
	g := newGateway("private-token")
	g.address = "ws" + strings.TrimPrefix(server.URL, "http")
	return g, server.Close
}
func socketRead(t *testing.T, c *websocket.Conn) packet {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var p packet
	if err := c.ReadJSON(&p); err != nil {
		t.Error(err)
	}
	return p
}
func socketSend(t *testing.T, c *websocket.Conn, op int, kind string, seq int, data any) {
	t.Helper()
	var sequence any
	if op == 0 {
		sequence = seq
	}
	if err := c.WriteJSON(map[string]any{"op": op, "t": kind, "s": sequence, "d": data}); err != nil {
		t.Error(err)
	}
}

func TestGatewayPushHeartbeatAndSubscription(t *testing.T) {
	done := make(chan struct{})
	g, closeServer := localGateway(t, func(c *websocket.Conn) {
		socketSend(t, c, 10, "", 0, map[string]any{"heartbeat_interval": 80})
		identify := socketRead(t, c)
		if identify.Op != 2 {
			t.Errorf("got opcode %d, want identify", identify.Op)
		}
		var auth map[string]any
		json.Unmarshal(identify.Data, &auth)
		if auth["token"] != "private-token" {
			t.Error("wrong token")
		}
		if _, ok := auth["intents"]; ok {
			t.Error("sent bot intents for a user account")
		}
		socketSend(t, c, 0, "READY", 12, map[string]any{"session_id": "session"})
		for {
			p := socketRead(t, c)
			if p.Op == 1 {
				socketSend(t, c, 11, "", 0, nil)
				continue
			}
			if p.Op != 37 {
				t.Errorf("expected guild subscription, got %d", p.Op)
			}
			var sub struct {
				Subscriptions map[string]struct {
					Typing bool `json:"typing"`
				} `json:"subscriptions"`
			}
			json.Unmarshal(p.Data, &sub)
			if !sub.Subscriptions["1"].Typing {
				t.Error("guild event subscription missing")
			}
			break
		}
		socketSend(t, c, 0, "MESSAGE_CREATE", 13, map[string]any{"id": "123", "channel_id": "10", "content": "pushed"})
		socketSend(t, c, 1, "", 0, nil)
		for beats := 0; beats < 2; {
			p := socketRead(t, c)
			if p.Op != 1 {
				t.Errorf("wrong heartbeat opcode: %d", p.Op)
				break
			}
			socketSend(t, c, 11, "", 0, nil)
			if string(p.Data) == "13" {
				beats++
			}
		}
		close(done)
	})
	defer closeServer()
	g.Watch("1", "10")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	errDone := make(chan error, 1)
	go func() { errDone <- g.connect(ctx) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("gateway handshake timed out")
	}
	select {
	case event := <-g.events:
		if event.status != "Gateway connected" {
			t.Fatal(event)
		}
	case <-ctx.Done():
		t.Fatal("missing connected event")
	}
	select {
	case event := <-g.events:
		if event.kind != "MESSAGE_CREATE" {
			t.Fatal(event)
		}
	case <-ctx.Done():
		t.Fatal("missing push event")
	}
	<-errDone
}
func TestGatewayResumeKeepsSequence(t *testing.T) {
	var connections atomic.Int32
	g, closeServer := localGateway(t, func(c *websocket.Conn) {
		n := connections.Add(1)
		socketSend(t, c, 10, "", 0, map[string]any{"heartbeat_interval": 10000})
		p := socketRead(t, c)
		if n == 1 {
			if p.Op != 2 {
				t.Error("missing identify")
			}
			socketSend(t, c, 0, "READY", 40, map[string]any{"session_id": "session"})
			socketSend(t, c, 0, "MESSAGE_CREATE", 41, map[string]any{"id": "123", "channel_id": "10"})
			socketSend(t, c, 7, "", 0, nil)
		} else {
			if p.Op != 6 {
				t.Errorf("expected resume, got %d", p.Op)
			}
			var resume struct {
				Session string `json:"session_id"`
				Seq     int    `json:"seq"`
			}
			json.Unmarshal(p.Data, &resume)
			if resume.Session != "session" || resume.Seq != 41 {
				t.Errorf("wrong resume: %+v", resume)
			}
			socketSend(t, c, 0, "RESUMED", 42, nil)
		}
	})
	defer closeServer()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	g.connect(ctx)
	g.connect(ctx)
	if connections.Load() != 2 {
		t.Fatal("did not reconnect")
	}
}
func TestGatewayStopsOnAuthenticationFailure(t *testing.T) {
	var connections atomic.Int32
	g, closeServer := localGateway(t, func(c *websocket.Conn) {
		connections.Add(1)
		socketSend(t, c, 10, "", 0, map[string]any{"heartbeat_interval": 10000})
		socketRead(t, c)
		c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4004, "private-token"), time.Now().Add(time.Second))
	})
	defer closeServer()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	g.Run(ctx)
	if connections.Load() != 1 {
		t.Error("retried rejected authentication")
	}
	for event := range g.events {
		if strings.Contains(event.status, "private-token") {
			t.Error("token leaked in status")
		}
	}
}
func TestGatewayCancellationClosesIdleSocket(t *testing.T) {
	opened := make(chan struct{})
	g, closeServer := localGateway(t, func(c *websocket.Conn) { close(opened); var p packet; c.ReadJSON(&p) })
	defer closeServer()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { g.connect(ctx); close(done) }()
	<-opened
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("quit left socket blocked on hello")
	}
}

func TestGatewayReconnectsWhenHeartbeatIsNotAcknowledged(t *testing.T) {
	g, closeServer := localGateway(t, func(c *websocket.Conn) {
		socketSend(t, c, 10, "", 0, map[string]any{"heartbeat_interval": 50})
		socketRead(t, c)
		socketSend(t, c, 0, "READY", 1, map[string]any{"session_id": "session"})
		p := socketRead(t, c)
		if p.Op != 1 {
			t.Error("heartbeat missing")
		}
		// Keep reading without an ACK until the client closes the stale connection.
		var ignored packet
		c.ReadJSON(&ignored)
	})
	defer closeServer()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := g.connect(ctx)
	if err == nil || err.Error() != "Gateway heartbeat was not acknowledged" {
		t.Fatalf("missed ACK was not detected: %v", err)
	}
}
