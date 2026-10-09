package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

const tokenInstructions = `Discord 개인 계정 토큰 확인 방법 (Chrome / Edge 기준)

1. 브라우저에서 https://discord.com/app 에 접속해 본인 계정으로 로그인하세요.
2. 개발자 도구를 여세요.
   Windows / Linux: Ctrl+Shift+I    macOS: Cmd+Option+I
3. Network(네트워크) 탭에서 Fetch/XHR를 선택한 뒤 Discord 채널을 이동하세요.
4. 목록에서 messages 요청처럼 주소가 https://discord.com/api/ 로 시작하는 요청을 선택하세요.
5. Headers → Request Headers → authorization 항목의 값만 복사하세요.
   authorization:이라는 항목 이름이나 따옴표는 복사하지 마세요.

토큰을 아래에 붙여넣고 Enter를 누르세요. 입력 내용은 화면에 표시되지 않습니다.
토큰은 계정에 접근할 수 있는 비밀 값이므로 다른 사람에게 공유하지 마세요.`

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
	logFile, logger, err := openLog(logPath, settings.logLevel())
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
			fmt.Println(tokenInstructions)
			fmt.Println("저장 위치:", path, "(다음 실행부터 자동으로 읽습니다)")
			fmt.Print("\nDiscord 토큰: ")
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
	_, err = tea.NewProgram(ui, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
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
