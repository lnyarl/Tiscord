package main

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var appLog = slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

func openLog(path string, level slog.Level) (*os.File, *slog.Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, nil, err
	}
	logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: level}))
	return file, logger, nil
}

func safeError(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return text
}

// Validation codes are useful for diagnosis; messages can echo private input.
func validationCodes(fields map[string]any) map[string][]string {
	result := map[string][]string{}
	for _, name := range []string{"after", "before", "limit", "content", "nonce", "message_reference"} {
		field, ok := fields[name].(map[string]any)
		if !ok {
			continue
		}
		items, _ := field["_errors"].([]any)
		for _, item := range items {
			entry, _ := item.(map[string]any)
			code, _ := entry["code"].(string)
			if code != "" && strings.IndexFunc(code, func(r rune) bool { return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') }) == -1 {
				result[name] = append(result[name], code)
			}
		}
	}
	return result
}

func logFatal(err error) {
	if err == nil {
		err = errors.New("Unknown startup error")
	}
	appLog.Error("app.failed", "error", err.Error())
}
