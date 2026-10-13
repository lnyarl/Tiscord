package main

import (
	"strings"
	"testing"
	"time"
)

func TestConsecutiveMessagesGroupByUserIDAndTime(t *testing.T) {
	stamp := time.Date(2026, 10, 9, 14, 32, 0, 0, time.Local)
	cases := []struct {
		name    string
		id      string
		gap     time.Duration
		grouped bool
	}{
		{"same user within window", "100", time.Minute, true},
		{"exactly five minutes", "100", 5 * time.Minute, true},
		{"over five minutes", "100", 5*time.Minute + time.Second, false},
		{"different users with same display name", "101", time.Minute, false},
		{"missing user ID", "", time.Minute, false},
		{"out of order timestamp", "100", -time.Minute, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := openDemo(t)
			m.messages = []message{
				{ID: "1", Timestamp: stamp, Author: author{ID: "100", Username: "alice"}, Content: "first"},
				{ID: "2", Timestamp: stamp.Add(tc.gap), Author: author{ID: tc.id, Username: "alice"}, Content: "second"},
			}
			lines, _ := m.body()
			text := strings.Join(lines, "\n")
			headers := strings.Count(text, "alice")
			if tc.grouped && headers != 1 || !tc.grouped && headers != 2 {
				t.Fatalf("wrong grouping: %q", text)
			}
			if !strings.Contains(text, "first") || !strings.Contains(text, "second") {
				t.Fatal("content lost")
			}
		})
	}
}

func TestMessageGroupingKeepsIndividualScrollTargets(t *testing.T) {
	m := openDemo(t)
	stamp := time.Now()
	m.messages = []message{
		{ID: "1", Timestamp: stamp, Author: author{ID: "100", Username: "alice"}, Content: "first\nmore"},
		{ID: "2", Timestamp: stamp.Add(time.Minute), Author: author{ID: "100", Username: "alice"}, Content: "second"},
		{ID: "3", Timestamp: stamp.Add(2 * time.Minute), Author: author{ID: "101", Username: "bob"}, Content: "third"},
		{ID: "4", Timestamp: stamp.Add(3 * time.Minute), Author: author{ID: "100", Username: "alice"}, Content: "fourth"},
	}
	for i, content := range []string{"first", "second", "third", "fourth"} {
		m.messageIndex = i
		lines, target := m.body()
		if !strings.Contains(lines[target], content) {
			t.Fatalf("message %d has wrong target %d: %q", i, target, lines)
		}
	}
	lines, _ := m.body()
	if strings.Count(strings.Join(lines, "\n"), "alice") != 2 {
		t.Fatal("grouping jumped across another author")
	}
	if lines[1] != "more" || lines[2] != "second" {
		t.Fatalf("continuations were padded instead of preserving text: %q", lines)
	}
}

func TestUnknownMessageTimesDoNotGroup(t *testing.T) {
	m := openDemo(t)
	m.messages = []message{{ID: "1", Author: author{ID: "100", Username: "alice"}, Content: "first"}, {ID: "2", Author: author{ID: "100", Username: "alice"}, Content: "second"}}
	lines, _ := m.body()
	if strings.Count(strings.Join(lines, "\n"), "alice") != 2 {
		t.Fatal("unknown timestamps grouped")
	}
}
