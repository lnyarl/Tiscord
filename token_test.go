package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTokenPersistsAndSkipsPromptOnNextRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiscord", "token")
	token, err := loadToken(path, "", func() (string, error) { return "  sample-token  ", nil })
	if err != nil || token != "sample-token" {
		t.Fatalf("initial load failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "sample-token\n" {
		t.Fatal("token was not saved")
	}
	token, err = loadToken(path, "", func() (string, error) { t.Fatal("prompt called despite saved token"); return "", nil })
	if err != nil || token != "sample-token" {
		t.Fatalf("reload failed: %v", err)
	}
	if runtime.GOOS != "windows" {
		file, _ := os.Stat(path)
		dir, _ := os.Stat(filepath.Dir(path))
		if file.Mode().Perm() != 0600 || dir.Mode().Perm() != 0700 {
			t.Fatal("token permissions are too broad")
		}
	}
}

func TestTokenEnvironmentOverridesWithoutOverwritingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := saveToken(path, "saved-token"); err != nil {
		t.Fatal(err)
	}
	token, err := loadToken(path, " env-token ", func() (string, error) { t.Fatal("unnecessary prompt"); return "", nil })
	if err != nil || token != "env-token" {
		t.Fatal("environment override failed")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "saved-token\n" {
		t.Fatal("environment token overwrote saved token")
	}
}

func TestTokenDoesNotSaveEmptyOrFailedInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiscord", "token")
	for _, prompt := range []func() (string, error){func() (string, error) { return " \n ", nil }, func() (string, error) { return "", errors.New("input failed") }} {
		if _, err := loadToken(path, "", prompt); err == nil {
			t.Fatal("invalid input accepted")
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("invalid token was saved")
		}
	}
}

func TestTokenReadErrorDoesNotPromptOrOverwrite(t *testing.T) {
	path := t.TempDir()
	if _, err := loadToken(path, "", func() (string, error) { t.Fatal("prompt called after read error"); return "", nil }); err == nil {
		t.Fatal("read error ignored")
	}
}
