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
		KoreanShortcuts    bool   `json:"korean_shortcuts"`
	} `json:"behavior"`
	Layout struct {
		Style           string `json:"style"`
		AuthorName      string `json:"author_name"`
		GroupSpacing    int    `json:"group_spacing"`
		ShowTimestamps  bool   `json:"show_timestamps"`
		TimestampFormat string `json:"timestamp_format"`
		InputMaxLines   int    `json:"input_max_lines"`
		InputGapLines   int    `json:"input_gap_lines"`
		ShowMetadata    bool   `json:"show_metadata"`
		ShowTyping      bool   `json:"show_typing"`
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
	c.Behavior.KoreanShortcuts = true
	c.Layout.ShowTimestamps = true
	c.Layout.Style = "plain"
	c.Layout.AuthorName = "username"
	c.Layout.TimestampFormat = "HH:mm"
	c.Layout.InputMaxLines = 3
	c.Layout.InputGapLines = 1
	c.Layout.ShowMetadata = true
	c.Layout.ShowTyping = true
	c.Logging.Level = "debug"
	return c
}

func (c config) validate() error {
	switch {
	case !validLayout(c.Layout.Style):
		return errors.New("layout.style must be plain, log or editor")
	case c.Layout.AuthorName != "username" && c.Layout.AuthorName != "display":
		return errors.New("layout.author_name must be username or display")
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

func (c config) authorName(user author) string {
	if c.Layout.AuthorName == "display" && user.GlobalName != "" {
		return user.GlobalName
	}
	return user.Username
}

func saveLayout(path, style string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		return err
	}
	if settings == nil {
		return errors.New("config must be a JSON object")
	}
	layout := make(map[string]json.RawMessage)
	if raw, ok := settings["layout"]; ok {
		if err := json.Unmarshal(raw, &layout); err != nil {
			return err
		}
		if layout == nil {
			return errors.New("layout must be a JSON object")
		}
	}
	layout["style"], _ = json.Marshal(style)
	settings["layout"], _ = json.Marshal(layout)
	data, err = json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return writeConfig(path, data)
}

func saveConfig(path string, c config) error {
	if err := c.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeConfig(path, data)
}

func writeConfig(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
