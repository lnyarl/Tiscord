package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type historyClient struct {
	*demoClient
	data    []message
	calls   int
	failure error
}

func (c *historyClient) Messages(_ context.Context, id, before string) ([]message, error) {
	c.calls++
	if c.failure != nil {
		return nil, c.failure
	}
	var result []message
	for i := len(c.data) - 1; i >= 0 && len(result) < 50; i-- {
		item := c.data[i]
		if before == "" || snowflakeLess(item.ID, before) {
			result = append(result, item)
		}
	}
	return result, nil
}

func TestRecoveryFetchesMoreThanOnePageAndReflectsEditsAndDeletes(t *testing.T) {
	m := openDemo(t)
	c := &historyClient{demoClient: newDemo()}
	for i := 1; i <= 125; i++ {
		if i != 2 {
			c.data = append(c.data, message{ID: fmt.Sprint(i), ChannelID: "10", Content: "new"})
		}
	}
	c.data[0].Content = "edited while disconnected"
	m.client = c
	m.gateway = newGateway("token")
	m.gatewayOnline = true
	m.messages = []message{{ID: "1", Content: "old"}, {ID: "2", Content: "deleted"}}
	m.messageIndex = 0
	m.draft.set("keep draft")
	runCommand(m, m.startSync())
	if c.calls != 3 || len(m.messages) != 124 || m.messages[0].Content != "edited while disconnected" || m.messages[len(m.messages)-1].ID != "125" {
		t.Fatalf("incomplete recovery: calls=%d messages=%d", c.calls, len(m.messages))
	}
	if m.draft.value() != "keep draft" || m.messages[m.messageIndex].ID != "1" || m.syncRequired {
		t.Fatal("recovery lost draft, anchor, or readiness")
	}
}

func TestDisconnectedAndSyncingStateBlocksSendAndKeepsDraft(t *testing.T) {
	m := openDemo(t)
	m.gateway = newGateway("token")
	m.hadConnection = true
	m.draft.set("keep me")
	m.Update(gatewayEvent{status: "Gateway disconnected; reconnecting"})
	if m.send() != nil || m.draft.value() != "keep me" || !strings.Contains(m.View().Content, "sending unavailable") {
		t.Fatal("disconnected send not blocked")
	}
	m.config.Layout.ShowMetadata = false
	if !strings.Contains(m.View().Content, "sending unavailable") {
		t.Fatal("connection warning disappeared with metadata hidden")
	}
	m.Update(gatewayEvent{status: "Gateway resumed"})
	if m.busy != "sync" || m.send() != nil || m.draft.value() != "keep me" {
		t.Fatal("recovery allowed premature send")
	}
	m.Update(loaded{kind: "sync", messages: m.messages, epoch: m.connectionEpoch})
	if m.syncRequired || m.send() == nil {
		t.Fatal("send was not re-enabled after recovery")
	}
}

func TestLiveEventsDuringRecoveryAreAppliedAfterSnapshot(t *testing.T) {
	m := openDemo(t)
	m.gatewayOnline = true
	m.startSync()
	m.Update(event("MESSAGE_CREATE", message{ID: "3", ChannelID: "10", Content: "live"}))
	m.Update(event("MESSAGE_DELETE", map[string]any{"id": "1", "channel_id": "10"}))
	m.Update(loaded{kind: "sync", messages: []message{{ID: "1", ChannelID: "10", Content: "snapshot"}}, epoch: m.connectionEpoch})
	if len(m.messages) != 1 || m.messages[0].ID != "3" {
		t.Fatal("snapshot overwrote live updates")
	}
}

func TestStaleRecoveryCannotEnableSendAfterNewDisconnect(t *testing.T) {
	m := openDemo(t)
	m.gateway = newGateway("token")
	m.gatewayOnline = true
	oldEpoch := m.connectionEpoch
	m.startSync()
	m.Update(gatewayEvent{status: "Gateway disconnected; reconnecting"})
	m.Update(loaded{kind: "sync", messages: []message{{ID: "999"}}, epoch: oldEpoch})
	if m.gatewayOnline || !m.syncRequired || m.messages[0].ID == "999" {
		t.Fatal("stale snapshot restored readiness")
	}
}

func TestFailedRecoveryKeepsEventsAndCanBeRetriedManually(t *testing.T) {
	m := openDemo(t)
	m.gateway = newGateway("token")
	m.gatewayOnline = true
	c := &historyClient{demoClient: newDemo(), failure: errors.New("HTTP unavailable")}
	m.client = c
	command := m.startSync()
	m.Update(event("MESSAGE_CREATE", message{ID: "3", ChannelID: "10", Content: "live"}))
	runCommand(m, command)
	if !m.syncRequired || m.busy != "" || m.messages[len(m.messages)-1].ID != "3" || m.send() != nil {
		t.Fatal("failed recovery lost events or enabled send")
	}
	m.Update(event("MESSAGE_CREATE", message{ID: "4", ChannelID: "10"}))
	if m.busy != "" {
		t.Fatal("ordinary event retried failed recovery automatically")
	}
	c.failure = nil
	c.data = m.messages
	runCommand(m, m.execute("sync"))
	if m.syncRequired {
		t.Fatal("manual retry failed")
	}
}

func TestResumedSessionStartsCanonicalRecovery(t *testing.T) {
	m := openDemo(t)
	m.gateway = newGateway("token")
	m.Update(gatewayEvent{status: "Gateway resumed"})
	if !m.gatewayOnline || !m.syncRequired || m.busy != "sync" {
		t.Fatal("resumed session did not verify history")
	}
}

func TestPendingSendDefersRecoveryAndFailurePreservesDraft(t *testing.T) {
	m := openDemo(t)
	m.gateway = newGateway("token")
	m.gatewayOnline = true
	m.draft.set("uncertain send")
	m.busy = "send"
	m.Update(gatewayEvent{status: "Gateway connected", fresh: true})
	if m.busy != "send" || !m.syncRequired {
		t.Fatal("recovery raced with pending send")
	}
	m.Update(sent{err: errors.New("Network error. Check Discord before resending")})
	if m.busy != "sync" || m.draft.value() != "uncertain send" {
		t.Fatal("failed send was resent or draft lost")
	}
}

func TestConnectionWarningFitsSmallTerminalAndClosedStreamStaysBlocked(t *testing.T) {
	m := openDemo(t)
	m.gateway = newGateway("token")
	m.gatewayOnline = false
	m.hadConnection = true
	m.receiveTyping(typingEvent("10", "100", "alice"), time.Now())
	m.height = 4
	m.config.Layout.InputMaxLines = 10
	m.config.Layout.InputGapLines = 3
	m.draft.set("one\ntwo\nthree")
	if len(strings.Split(m.View().Content, "\n")) != 4 {
		t.Fatal("connection warning overflowed terminal")
	}
	m.Update(streamClosed{})
	if !m.gatewayStopped || m.send() != nil || m.connection != "Gateway stopped" {
		t.Fatal("closed stream was still treated as connected")
	}
}
