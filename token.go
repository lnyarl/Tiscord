package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func tokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", errors.New("Could not find the user configuration directory")
	}
	return filepath.Join(dir, "tiscord", "token"), nil
}

func loadToken(path, environment string, prompt func() (string, error)) (string, error) {
	if token := strings.TrimSpace(environment); token != "" {
		appLog.Info("auth.loaded", "source", "environment")
		return token, nil
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if token := strings.TrimSpace(string(data)); token != "" {
			appLog.Info("auth.loaded", "source", "file", "path", path)
			return token, nil
		}
		return "", fmt.Errorf("Saved token file is empty; replace or remove %s", path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("Could not read token file: %s", path)
	}
	appLog.Info("auth.prompt")
	token, err := prompt()
	if err != nil {
		return "", err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("A token is required. Use --demo for an offline preview")
	}
	if err := saveToken(path, token); err != nil {
		return "", err
	}
	appLog.Info("auth.saved", "path", path)
	return token, nil
}

func saveToken(path, token string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("Could not create token directory: %s", dir)
	}
	file, err := os.CreateTemp(dir, ".token-*")
	if err != nil {
		return fmt.Errorf("Could not create token file in %s", dir)
	}
	defer os.Remove(file.Name())
	_, writeErr := file.WriteString(token + "\n")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return fmt.Errorf("Could not write token file: %s", path)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("Could not save token file: %s", path)
	}
	return nil
}
