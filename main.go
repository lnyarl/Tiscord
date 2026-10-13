package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/term"
)

const tokenInstructions = `Finding your Discord account token (Chrome / Edge)

1. Open https://discord.com/app in your browser and sign in to your account.
2. Open Developer Tools.
   Windows / Linux: Ctrl+Shift+I    macOS: Cmd+Option+I
3. In the Network tab, select Fetch/XHR and switch Discord channels.
4. Select a request under https://discord.com/api/, such as a messages request.
5. Under Headers → Request Headers, copy only the authorization value.
   Do not include the authorization: label or quotation marks.

Paste the token below and press Enter. Your input will be hidden.
The token grants access to your account. Do not share it.`

func main() {
	demo := flag.Bool("demo", false, "Offline preview, without a token or network access")
	settingsPath := flag.String("config", "", "Path to config.json (default: user configuration directory)")
	flag.Parse()
	configPath, err := tokenPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	logPath := filepath.Join(filepath.Dir(configPath), "tiscord.log")
	if *settingsPath == "" {
		*settingsPath = filepath.Join(filepath.Dir(configPath), "config.json")
	}
	settings, err := loadConfig(*settingsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Config:", *settingsPath, "-", err)
		os.Exit(1)
	}
	var logLevel slog.LevelVar
	logLevel.Set(settings.logLevel())
	logFile, logger, err := openLog(logPath, &logLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not open log file:", err)
		os.Exit(1)
	}
	appLog = logger
	defer logFile.Close()
	appLog.Info("app.started", "demo", *demo, "os", runtime.GOOS, "arch", runtime.GOARCH, "log_path", logPath)
	appLog.Info("config.loaded", "path", *settingsPath, "settings", settings)
	fmt.Fprintln(os.Stderr, "Config:", *settingsPath)
	fmt.Fprintln(os.Stderr, "Log:", logPath)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var client chatClient
	var g *gateway
	var gatewayDone chan struct{}
	if *demo {
		client = newDemo()
	} else {
		path, err := tokenPath()
		if err != nil {
			logFatal(err)
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		token, err := loadToken(path, os.Getenv("DISCORD_TOKEN"), func() (string, error) {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return "", fmt.Errorf("Use an interactive terminal for hidden token input, or set DISCORD_TOKEN")
			}
			fmt.Println()
			fmt.Println(localize(settings.Behavior.Language, tokenInstructions))
			fmt.Println(localize(settings.Behavior.Language, "Save location:"), path, localize(settings.Behavior.Language, "(loaded automatically on subsequent runs)"))
			fmt.Print(localize(settings.Behavior.Language, "\nDiscord token: "))
			data, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return "", fmt.Errorf("Could not read token")
			}
			return string(data), nil
		})
		if err != nil {
			logFatal(err)
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		client = newREST(token)
		g = newGateway(token)
		gatewayDone = make(chan struct{})
		go func() { defer close(gatewayDone); g.Run(ctx) }()
	}
	appLog.Info("ui.started")
	ui := newModel(ctx, client, g)
	ui.config = settings
	ui.configPath = *settingsPath
	ui.logLevel = &logLevel
	err = runTerminalUI(ctx, ui, os.Stdout)
	cancel()
	if gatewayDone != nil {
		<-gatewayDone
	}
	if err != nil {
		logFatal(err)
		fmt.Fprintln(os.Stderr, "Could not run the terminal UI:", err)
		os.Exit(1)
	}
	appLog.Info("app.stopped")
}
