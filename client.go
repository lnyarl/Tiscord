package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type guild struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type channel struct {
	Recipients    []author `json:"recipients,omitempty"`
	RecipientIDs  []string `json:"recipient_ids,omitempty"`
	GuildID       string   `json:"guild_id,omitempty"`
	LastMessageID string   `json:"last_message_id,omitempty"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Type          int      `json:"type"`
	Position      int      `json:"position"`
}
type author struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
}
type link struct {
	URL string `json:"url"`
}
type embed struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Author      struct {
		Name string `json:"name"`
	} `json:"author"`
	Fields []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"fields"`
	Footer struct {
		Text string `json:"text"`
	} `json:"footer"`
	Image     link `json:"image"`
	Thumbnail link `json:"thumbnail"`
	Video     link `json:"video"`
}
type message struct {
	ID          string    `json:"id"`
	ChannelID   string    `json:"channel_id"`
	Content     string    `json:"content"`
	Timestamp   time.Time `json:"timestamp"`
	Author      author    `json:"author"`
	Attachments []link    `json:"attachments"`
	Embeds      []embed   `json:"embeds"`
}
type apiError struct {
	status int
	text   string
}

func (e *apiError) Error() string { return e.text }

type chatClient interface {
	Guilds(context.Context) ([]guild, error)
	Channels(context.Context, string) ([]channel, error)
	Messages(context.Context, string, string) ([]message, error)
	Send(context.Context, string, string) (message, error)
}

type restClient struct {
	readMu    sync.Mutex
	readToken *string
	token     string
	base      string
	http      *http.Client
	mu        sync.Mutex
	readyAt   time.Time
}

var requestID atomic.Uint64

func newREST(token string) *restClient {
	return &restClient{token: token, base: "https://discord.com/api/v10", http: &http.Client{Timeout: 20 * time.Second}}
}

func wait(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *restClient) request(ctx context.Context, method, path string, body any, out any) error {
	id := requestID.Add(1)
	started := time.Now()
	appLog.Debug("http.queued", "request_id", id, "method", method, "path", path)
	c.mu.Lock()
	defer c.mu.Unlock()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 4; attempt++ {
		delay := time.Until(c.readyAt)
		if delay > 0 {
			appLog.Debug("http.cooldown", "request_id", id, "wait_ms", delay.Milliseconds())
		}
		if err := wait(ctx, delay); err != nil {
			appLog.Warn("http.cancelled", "request_id", id, "error", safeError(err, c.token))
			return err
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(encoded))
		if err != nil {
			return errors.New("Invalid request")
		}
		req.Header.Set("Authorization", c.token)
		req.Header.Set("Content-Type", "application/json")
		attemptStarted := time.Now()
		appLog.Debug("http.request", "request_id", id, "attempt", attempt+1, "method", method, "path", path, "body_bytes", len(encoded))
		response, err := c.http.Do(req)
		if err != nil {
			appLog.Error("http.network_error", "request_id", id, "elapsed_ms", time.Since(attemptStarted).Milliseconds(), "error", safeError(err, c.token))
			return errors.New("Network error. Check Discord before resending")
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 32<<20))
		response.Body.Close()
		appLog.Debug("http.response", "request_id", id, "status", response.StatusCode, "elapsed_ms", time.Since(attemptStarted).Milliseconds(), "body_bytes", len(data), "remaining", response.Header.Get("X-RateLimit-Remaining"), "reset_after", response.Header.Get("X-RateLimit-Reset-After"))
		if readErr != nil {
			appLog.Error("http.read_error", "request_id", id, "error", safeError(readErr, c.token))
			return errors.New("Network error. Check Discord before resending")
		}
		if response.StatusCode == 429 {
			var limit struct {
				RetryAfter float64 `json:"retry_after"`
			}
			if json.Unmarshal(data, &limit) != nil {
				limit.RetryAfter = 1
			}
			c.readyAt = time.Now().Add(time.Duration(max(0.1, limit.RetryAfter) * float64(time.Second)))
			appLog.Warn("http.rate_limited", "request_id", id, "retry_after_seconds", limit.RetryAfter, "attempt", attempt+1)
			if attempt == 3 {
				return &apiError{429, "Rate limited; try again later"}
			}
			continue
		}
		if response.Header.Get("X-RateLimit-Remaining") == "0" {
			delay, err := strconv.ParseFloat(response.Header.Get("X-RateLimit-Reset-After"), 64)
			if err != nil {
				delay = 1
			}
			c.readyAt = time.Now().Add(time.Duration(max(0, delay) * float64(time.Second)))
		}
		if response.StatusCode >= 400 {
			text := map[int]string{400: "Discord rejected this request", 401: "Authentication failed. Check your token", 403: "You cannot access or send in this channel", 404: "This resource no longer exists"}[response.StatusCode]
			if text == "" {
				text = "Discord is unavailable; try again"
			}
			var failure struct {
				Code   int            `json:"code"`
				Errors map[string]any `json:"errors"`
			}
			json.Unmarshal(data, &failure)
			appLog.Error("http.rejected", "request_id", id, "method", method, "path", path, "status", response.StatusCode, "discord_code", failure.Code, "validation", validationCodes(failure.Errors))
			prefix := fmt.Sprintf("HTTP %d", response.StatusCode)
			if failure.Code != 0 {
				prefix += fmt.Sprintf(" (Discord %d)", failure.Code)
			}
			return &apiError{response.StatusCode, prefix + ": " + text}
		}
		if out == nil {
			return nil
		}
		if response.StatusCode == http.StatusNoContent && len(data) == 0 {
			return nil
		}
		if json.Unmarshal(data, out) != nil {
			appLog.Error("http.decode_error", "request_id", id, "body_bytes", len(data))
			return errors.New("Invalid Discord response")
		}
		appLog.Debug("http.completed", "request_id", id, "elapsed_ms", time.Since(started).Milliseconds())
		return nil
	}
	return errors.New("Request failed")
}

func (c *restClient) Guilds(ctx context.Context) ([]guild, error) {
	var result []guild
	after := ""
	for {
		var page []guild
		query := url.Values{"limit": {"200"}}
		if after != "" {
			query.Set("after", after)
		}
		err := c.request(ctx, "GET", "/users/@me/guilds?"+query.Encode(), nil, &page)
		if err != nil {
			return nil, err
		}
		result = append(result, page...)
		if len(page) < 200 {
			return result, nil
		}
		sort.Slice(page, func(i, j int) bool { return snowflakeLess(page[i].ID, page[j].ID) })
		after = page[len(page)-1].ID
	}
}
func (c *restClient) Channels(ctx context.Context, id string) ([]channel, error) {
	var data []channel
	if err := c.request(ctx, "GET", "/guilds/"+url.PathEscape(id)+"/channels", nil, &data); err != nil {
		return nil, err
	}
	result := make([]channel, 0, len(data))
	for _, item := range data {
		if item.Type == 0 || item.Type == 5 {
			result = append(result, item)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Position < result[j].Position })
	return result, nil
}
func (c *restClient) Messages(ctx context.Context, id, before string) ([]message, error) {
	var result []message
	query := url.Values{"limit": {"50"}}
	if before != "" {
		query.Set("before", before)
	}
	err := c.request(ctx, "GET", "/channels/"+url.PathEscape(id)+"/messages?"+query.Encode(), nil, &result)
	return result, err
}
func (c *restClient) Send(ctx context.Context, id, content string) (message, error) {
	var result message
	err := c.request(ctx, "POST", "/channels/"+url.PathEscape(id)+"/messages", map[string]string{"content": content}, &result)
	return result, err
}
func snowflakeLess(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}
func mergeMessages(old, incoming []message) []message {
	byID := make(map[string]message, len(old)+len(incoming))
	for _, m := range old {
		byID[m.ID] = m
	}
	for _, m := range incoming {
		byID[m.ID] = m
	}
	result := make([]message, 0, len(byID))
	for _, m := range byID {
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool { return snowflakeLess(result[i].ID, result[j].ID) })
	return result
}

type demoClient struct {
	mu   sync.Mutex
	data map[string][]message
	next int
}

func newDemo() *demoClient {
	stamp := time.Date(2026, 10, 9, 14, 32, 0, 0, time.Local)
	return &demoClient{next: 3, data: map[string][]message{
		"10": {{ID: "1", ChannelID: "10", Content: "Welcome. Press i to write and Enter to send.", Timestamp: stamp, Author: author{ID: "100", Username: "alice"}}},
		"11": {{ID: "2", ChannelID: "11", Content: "Esc returns to Normal mode. Use :channels to switch channels.", Timestamp: stamp, Author: author{ID: "101", Username: "bob"}}},
	}}
}
func (c *demoClient) Guilds(context.Context) ([]guild, error) {
	return []guild{{"1", "Tiscord demo"}}, nil
}
func (c *demoClient) Channels(context.Context, string) ([]channel, error) {
	return []channel{{ID: "10", Name: "general"}, {ID: "11", Name: "dev"}}, nil
}
func (c *demoClient) Messages(_ context.Context, id, before string) ([]message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var result []message
	for _, m := range c.data[id] {
		if before == "" || snowflakeLess(m.ID, before) {
			result = append(result, m)
		}
	}
	return result, nil
}
func (c *demoClient) Send(_ context.Context, id, content string) (message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := message{ID: fmt.Sprint(c.next), ChannelID: id, Content: content, Timestamp: time.Now(), Author: author{ID: "102", Username: "demo"}}
	c.next++
	c.data[id] = append(c.data[id], m)
	return m, nil
}
