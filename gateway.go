package main

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type gatewayEvent struct {
	channels   []channel
	readStates []readState
	kind       string
	data       json.RawMessage
	status     string
	fresh      bool
	selfUserID string
	sessionID  string
}
type packet struct {
	Op   int             `json:"op"`
	Data json.RawMessage `json:"d"`
	Seq  *int64          `json:"s"`
	Type string          `json:"t"`
}
type watch struct {
	guildID, channelID string
	applied            chan struct{}
}
type gateway struct {
	token, address, session, resumeURL string
	seq                                *int64
	events                             chan gatewayEvent
	changed                            chan struct{}
	stopped                            chan struct{}
	mu                                 sync.Mutex
	watch                              watch
	dialer                             *websocket.Dialer
}

func newGateway(token string) *gateway {
	return &gateway{token: token, address: "wss://gateway.discord.gg/", events: make(chan gatewayEvent, 256), changed: make(chan struct{}, 1), stopped: make(chan struct{}), dialer: websocket.DefaultDialer}
}
func (g *gateway) Watch(guildID, channelID string) <-chan struct{} {
	appLog.Debug("gateway.watch", "guild_id", guildID, "channel_id", channelID)
	applied := make(chan struct{})
	g.mu.Lock()
	g.watch = watch{guildID, channelID, applied}
	g.mu.Unlock()
	select {
	case g.changed <- struct{}{}:
	default:
	}
	return applied
}
func (g *gateway) emit(ctx context.Context, event gatewayEvent) {
	if event.status != "" {
		appLog.Info("gateway.status", "status", event.status, "fresh", event.fresh)
	}
	select {
	case g.events <- event:
	case <-ctx.Done():
	}
}
func (g *gateway) Run(ctx context.Context) {
	defer close(g.events)
	defer close(g.stopped)
	backoff := time.Second
	for ctx.Err() == nil {
		g.emit(ctx, gatewayEvent{status: "Gateway connecting"})
		connectedAt := time.Now()
		err := g.connect(ctx)
		if time.Since(connectedAt) >= 30*time.Second {
			backoff = time.Second
		}
		var logClose *websocket.CloseError
		if errors.As(err, &logClose) {
			appLog.Warn("gateway.closed", "close_code", logClose.Code)
		} else if err != nil {
			appLog.Warn("gateway.connection_error", "error", safeError(err, g.token, g.session))
		}
		if ctx.Err() != nil {
			return
		}
		var closed *websocket.CloseError
		if errors.As(err, &closed) {
			switch closed.Code {
			case 4004, 4010, 4011, 4012, 4013, 4014:
				g.emit(ctx, gatewayEvent{status: "Gateway rejected authentication or protocol"})
				return
			case 4007, 4009:
				g.session = ""
				g.seq = nil
				g.resumeURL = ""
			}
		}
		g.emit(ctx, gatewayEvent{status: "Gateway disconnected; reconnecting"})
		appLog.Info("gateway.reconnect", "backoff_ms", backoff.Milliseconds(), "resume", g.session != "")
		if err := wait(ctx, backoff+time.Duration(rand.IntN(500))*time.Millisecond); err != nil {
			return
		}
		backoff = min(30*time.Second, backoff*2)
	}
}

func (g *gateway) connect(ctx context.Context) error {
	address := g.address
	if g.session != "" && g.resumeURL != "" {
		address = g.resumeURL
	}
	u, err := url.Parse(address)
	if err != nil {
		return err
	}
	query := u.Query()
	query.Set("v", "9")
	query.Set("encoding", "json")
	u.RawQuery = query.Encode()
	appLog.Debug("gateway.dial", "host", u.Host, "resume", g.session != "")
	c, response, err := g.dialer.DialContext(ctx, u.String(), nil)
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		return err
	}
	defer c.Close()
	stopClose := context.AfterFunc(ctx, func() { c.Close() })
	defer stopClose()
	c.SetReadLimit(32 << 20)
	c.SetReadDeadline(time.Now().Add(15 * time.Second))
	var hello packet
	if err := c.ReadJSON(&hello); err != nil {
		return err
	}
	var timing struct {
		Interval float64 `json:"heartbeat_interval"`
	}
	if hello.Op != 10 || json.Unmarshal(hello.Data, &timing) != nil || timing.Interval <= 0 {
		return errors.New("Invalid Gateway hello")
	}
	c.SetReadDeadline(time.Time{})
	interval := time.Duration(timing.Interval * float64(time.Millisecond))
	appLog.Debug("gateway.hello", "heartbeat_interval_ms", interval.Milliseconds())
	send := func(op int, data any) error {
		appLog.Debug("gateway.send", "opcode", op)
		c.SetWriteDeadline(time.Now().Add(10 * time.Second))
		err := c.WriteJSON(map[string]any{"op": op, "d": data})
		if err != nil {
			appLog.Error("gateway.write_error", "opcode", op, "error", safeError(err, g.token, g.session))
		}
		return err
	}
	if g.session != "" {
		err = send(6, map[string]any{"token": g.token, "session_id": g.session, "seq": g.seq})
	} else {
		err = send(2, map[string]any{"token": g.token, "properties": map[string]string{"os": runtime.GOOS, "browser": "Tiscord", "device": "Tiscord"}, "compress": false, "capabilities": 0, "client_state": map[string]any{"guild_versions": map[string]any{}}})
	}
	if err != nil {
		return err
	}

	connectionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type received struct {
		packet packet
		err    error
	}
	packets := make(chan received, 32)
	go func() {
		for {
			var p packet
			err := c.ReadJSON(&p)
			select {
			case packets <- received{p, err}:
			case <-connectionCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	// Heartbeats maintain the socket; they never query chat history.
	heartbeat := time.NewTimer(time.Duration(rand.Float64() * float64(interval)))
	defer heartbeat.Stop()
	acked := true
	ready := false
	var subscription <-chan time.Time
	var subscriptionTimer *time.Timer
	defer func() {
		if subscriptionTimer != nil {
			subscriptionTimer.Stop()
		}
	}()
	monitoredGuilds := make(map[string]bool)
	subscribedGuilds := make(map[string]bool)
	subscribe := func() error {
		g.mu.Lock()
		current := g.watch
		g.mu.Unlock()
		desired := make(map[string]bool)
		for id := range monitoredGuilds {
			desired[id] = true
		}
		if current.guildID != "" {
			desired[current.guildID] = true
		}
		subs := map[string]any{}
		for id := range subscribedGuilds {
			if !desired[id] {
				subs[id] = map[string]any{"typing": false, "threads": false, "activities": false, "channels": map[string]any{}}
			}
		}
		for id := range desired {
			if !subscribedGuilds[id] {
				// typing=true enables the user guild event stream; no member ranges are requested.
				subs[id] = map[string]any{"typing": true, "threads": false, "activities": false, "channels": map[string]any{}}
			}
		}
		if len(subs) > 0 {
			// Keep each Gateway payload below its size limit for accounts with many guilds.
			batch := make(map[string]any)
			for id, value := range subs {
				batch[id] = value
				if len(batch) == 20 {
					if err := send(37, map[string]any{"subscriptions": batch}); err != nil {
						return err
					}
					batch = make(map[string]any)
				}
			}
			if len(batch) > 0 {
				if err := send(37, map[string]any{"subscriptions": batch}); err != nil {
					return err
				}
			}
		}
		subscribedGuilds = desired
		appLog.Info("gateway.subscribed", "guild_id", current.guildID, "channel_id", current.channelID)
		if current.applied != nil {
			select {
			case <-current.applied:
			default:
				close(current.applied)
			}
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-g.changed:
			if ready && subscription == nil {
				subscriptionTimer = time.NewTimer(time.Second)
				subscription = subscriptionTimer.C
			}
		case <-subscription:
			subscription = nil
			if err := subscribe(); err != nil {
				return err
			}
		case <-heartbeat.C:
			if !acked {
				appLog.Warn("gateway.heartbeat_timeout")
				return errors.New("Gateway heartbeat was not acknowledged")
			}
			if err := send(1, g.seq); err != nil {
				return err
			}
			acked = false
			heartbeat.Reset(interval)
		case incoming := <-packets:
			if incoming.err != nil {
				return incoming.err
			}
			p := incoming.packet
			appLog.Debug("gateway.receive", "opcode", p.Op, "event", p.Type, "sequence", p.Seq, "data_bytes", len(p.Data))
			if p.Op == 0 && p.Seq != nil {
				g.seq = p.Seq
			}
			switch p.Op {
			case 1:
				if err := send(1, g.seq); err != nil {
					return err
				}
				acked = false
				// Give this newly requested heartbeat a full interval to be acknowledged.
				heartbeat.Reset(interval)
			case 11:
				appLog.Debug("gateway.heartbeat_ack")
				acked = true
			case 7:
				return errors.New("Gateway requested reconnect")
			case 9:
				var resumable bool
				json.Unmarshal(p.Data, &resumable)
				appLog.Warn("gateway.invalid_session", "resumable", resumable)
				if !resumable {
					g.session = ""
					g.seq = nil
					g.resumeURL = ""
				}
				if err := wait(ctx, time.Duration(1+rand.IntN(4))*time.Second); err != nil {
					return err
				}
				return errors.New("Gateway session invalidated")
			case 0:
				if p.Type == "READY" {
					var data struct {
						Session         string          `json:"session_id"`
						URL             string          `json:"resume_gateway_url"`
						User            author          `json:"user"`
						ReadStates      json.RawMessage `json:"read_state"`
						PrivateChannels []channel       `json:"private_channels"`
						Users           []author        `json:"users"`
						Guilds          []struct {
							ID       string    `json:"id"`
							Channels []channel `json:"channels"`
						} `json:"guilds"`
					}
					if err := json.Unmarshal(p.Data, &data); err != nil {
						return err
					}
					g.session = data.Session
					channels := append([]channel{}, data.PrivateChannels...)
					users := make(map[string]author, len(data.Users))
					for _, user := range data.Users {
						users[user.ID] = user
					}
					for i := range channels {
						if len(channels[i].Recipients) > 0 {
							continue
						}
						for _, id := range channels[i].RecipientIDs {
							user, ok := users[id]
							if !ok {
								user = author{ID: id}
							}
							channels[i].Recipients = append(channels[i].Recipients, user)
						}
					}
					for _, guild := range data.Guilds {
						if guild.ID == "" {
							continue
						}
						monitoredGuilds[guild.ID] = true
						for _, item := range guild.Channels {
							item.GuildID = guild.ID
							channels = append(channels, item)
						}
					}
					resume, err := url.Parse(data.URL)
					if err == nil && resume.Scheme == "wss" && (resume.Hostname() == "gateway.discord.gg" || strings.HasSuffix(resume.Hostname(), ".discord.gg")) {
						g.resumeURL = data.URL
					}
					ready = true
					if err := subscribe(); err != nil {
						return err
					}
					g.emit(ctx, gatewayEvent{status: "Gateway connected", fresh: true, selfUserID: data.User.ID, sessionID: data.Session, readStates: decodeReadStates(data.ReadStates), channels: channels})
				} else if p.Type == "RESUMED" {
					ready = true
					if err := subscribe(); err != nil {
						return err
					}
					g.emit(ctx, gatewayEvent{status: "Gateway resumed", sessionID: g.session})
				} else if strings.HasPrefix(p.Type, "MESSAGE_") || p.Type == "CHANNEL_CREATE" || p.Type == "CHANNEL_UPDATE" || p.Type == "CHANNEL_DELETE" || p.Type == "TYPING_START" || p.Type == "APPLICATION_COMMAND_AUTOCOMPLETE_RESPONSE" || p.Type == "INTERACTION_CREATE" || p.Type == "INTERACTION_SUCCESS" || p.Type == "INTERACTION_FAILURE" {
					g.emit(ctx, gatewayEvent{kind: p.Type, data: p.Data})
				}
			}
		}
	}
}
