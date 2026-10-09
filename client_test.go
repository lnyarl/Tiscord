package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRESTTokenHistoryAndSend(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "private-token" {
			t.Error("token was not passed as a raw user token")
		}
		if r.Method == "GET" {
			if r.URL.Query().Get("before") != "123" || r.URL.Query().Get("limit") != "50" {
				t.Error("wrong history parameters")
			}
			fmt.Fprint(w, `[]`)
		} else {
			fmt.Fprint(w, `{"id":"124","channel_id":"10","content":"hello"}`)
		}
	}))
	defer server.Close()
	c := newREST("private-token")
	c.base = server.URL
	if _, err := c.Messages(context.Background(), "10", "123"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Send(context.Background(), "10", "hello"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatal(requests)
	}
}
func TestRESTRateLimit(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(429)
			fmt.Fprint(w, `{"retry_after":0.001,"global":true}`)
		} else {
			fmt.Fprint(w, `[]`)
		}
	}))
	defer server.Close()
	c := newREST("token")
	c.base = server.URL
	if _, err := c.Guilds(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatal(requests)
	}
}
func TestRESTDoesNotRepeatAmbiguousSend(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	}))
	defer server.Close()
	c := newREST("private-token")
	c.base = server.URL
	_, err := c.Send(context.Background(), "10", "hello")
	if err == nil || strings.Contains(err.Error(), "private-token") {
		t.Fatalf("unsafe error: %v", err)
	}
	if requests != 1 {
		t.Fatalf("ambiguous send retried %d times", requests)
	}
}
func TestRESTPermissionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"message":"sensitive"}`)
	}))
	defer server.Close()
	c := newREST("token")
	c.base = server.URL
	_, err := c.Messages(context.Background(), "10", "")
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestInitialRequestsOmitEmptySnowflakes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for _, name := range []string{"after", "before"} {
			if query.Has(name) && query.Get(name) == "" {
				w.WriteHeader(400)
				fmt.Fprint(w, `{"code":50035,"message":"Invalid Form Body"}`)
				return
			}
		}
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()
	c := newREST("token")
	c.base = server.URL
	if _, err := c.Guilds(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Messages(context.Background(), "10", ""); err != nil {
		t.Fatal(err)
	}
}

func TestGuildPaginationIncludesValidAfter(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			if r.URL.Query().Has("after") {
				t.Error("initial cursor must be absent")
			}
			var page []guild
			for i := 1; i <= 200; i++ {
				page = append(page, guild{ID: fmt.Sprint(i)})
			}
			json.NewEncoder(w).Encode(page)
		} else {
			if r.URL.Query().Get("after") != "200" {
				t.Error("wrong pagination cursor")
			}
			fmt.Fprint(w, `[{"id":"201"}]`)
		}
	}))
	defer server.Close()
	c := newREST("token")
	c.base = server.URL
	data, err := c.Guilds(context.Background())
	if err != nil || len(data) != 201 || requests != 2 {
		t.Fatalf("pagination failed: %v", err)
	}
}

func TestRESTErrorIncludesCodesWithoutResponseSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"code":50035,"message":"private-token","errors":{"after":{"message":"sensitive"}}}`)
	}))
	defer server.Close()
	c := newREST("private-token")
	c.base = server.URL
	_, err := c.Guilds(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 400 (Discord 50035)") || strings.Contains(err.Error(), "private-token") || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("unsafe or incomplete error: %v", err)
	}
}
