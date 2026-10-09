package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type config struct {
	Behavior struct {
		GroupMessages      bool   `json:"group_messages"`
		GroupWindowSeconds int    `json:"group_window_seconds"`
		SendKey            string `json:"send_key"`
		StartMode          string `json:"start_mode"`
	} `json:"behavior"`
	Layout struct {
		GroupSpacing    int    `json:"group_spacing"`
		ShowTimestamps  bool   `json:"show_timestamps"`
		TimestampFormat string `json:"timestamp_format"`
		InputMaxLines   int    `json:"input_max_lines"`
		InputGapLines   int    `json:"input_gap_lines"`
		ShowMetadata    bool   `json:"show_metadata"`
	} `json:"layout"`
	Logging struct {
		Level string `json:"level"`
	} `json:"logging"`
}

func defaultConfig() config {
	var c config
	c.Behavior.GroupMessages = true
	c.Behavior.GroupWindowSeconds = 300
	c.Behavior.SendKey = "ctrl+s"
	c.Behavior.StartMode = "normal"
	c.Layout.ShowTimestamps = true
	c.Layout.TimestampFormat = "HH:mm"
	c.Layout.InputMaxLines = 3
	c.Layout.InputGapLines = 1
	c.Layout.ShowMetadata = true
	c.Logging.Level = "debug"
	return c
}

func (c config) validate() error {
	switch {
	case c.Behavior.GroupWindowSeconds < 1 || c.Behavior.GroupWindowSeconds > 86400:
		return errors.New("behavior.group_window_seconds must be between 1 and 86400")
	case c.Behavior.SendKey != "ctrl+s" && c.Behavior.SendKey != "enter":
		return errors.New("behavior.send_key must be ctrl+s or enter")
	case c.Behavior.StartMode != "normal" && c.Behavior.StartMode != "insert":
		return errors.New("behavior.start_mode must be normal or insert")
	case c.Layout.GroupSpacing < 0 || c.Layout.GroupSpacing > 3:
		return errors.New("layout.group_spacing must be between 0 and 3")
	case c.Layout.TimestampFormat != "HH:mm" && c.Layout.TimestampFormat != "HH:mm:ss" && c.Layout.TimestampFormat != "YYYY-MM-DD HH:mm":
		return errors.New("layout.timestamp_format must be HH:mm, HH:mm:ss or YYYY-MM-DD HH:mm")
	case c.Layout.InputMaxLines < 1 || c.Layout.InputMaxLines > 10:
		return errors.New("layout.input_max_lines must be between 1 and 10")
	case c.Layout.InputGapLines < 0 || c.Layout.InputGapLines > 3:
		return errors.New("layout.input_gap_lines must be between 0 and 3")
	case c.Logging.Level != "debug" && c.Logging.Level != "info" && c.Logging.Level != "warn" && c.Logging.Level != "error" && c.Logging.Level != "off":
		return errors.New("logging.level must be debug, info, warn, error or off")
	}
	return nil
}

func loadConfig(path string) (config, error) {
	c := defaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return c, err
		}
		data, _ = json.MarshalIndent(c, "", "  ")
		return c, os.WriteFile(path, append(data, '\n'), 0600)
	}
	if err != nil {
		return c, err
	}
	if trimmed := bytes.TrimSpace(data); len(trimmed) == 0 || trimmed[0] != '{' {
		return c, errors.New("config must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return c, fmt.Errorf("invalid config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return c, errors.New("config must contain exactly one JSON object")
	}
	return c, c.validate()
}

func (c config) logLevel() slog.Level {
	return map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError, "off": 100}[c.Logging.Level]
}
func (c config) timeFormat() string {
	return map[string]string{"HH:mm": "15:04", "HH:mm:ss": "15:04:05", "YYYY-MM-DD HH:mm": "2006-01-02 15:04"}[c.Layout.TimestampFormat]
}
