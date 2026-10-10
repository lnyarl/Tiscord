package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

var commands = []struct{ name, description string }{
	{"?", "명령어 목록과 설명 보기"},
	{"help", "명령어 목록과 설명 보기 (:?와 동일)"},
	{"channels", "선택한 서버의 채널 목록으로 이동 (서버 선택 필요)"},
	{"servers", "서버 목록으로 이동"},
	{"send", "작성 중인 메시지 전송 (채팅 화면에서 사용)"},
	{"older", "이전 메시지 불러오기 (채팅 화면에서 사용)"},
	{"sync", "현재 채널 기록 다시 동기화 (연결된 채팅 화면에서 사용)"},
	{"layout", "plain/log/editor 형식 순환 또는 :layout 형식명으로 선택 (자동 저장)"},
	{"config", "설정 화면 열기 · j/k 이동 · h/l 변경 · Ctrl-S 저장 · Esc 취소"},
	{"slash", "Discord 봇 슬래시 명령 입력 도우미 열기 (채팅 화면에서 /와 동일)"},
	{"q", "프로그램 종료"},
	{"quit", "프로그램 종료 (:q와 동일)"},
}

func (m *model) completeCommand(direction int) {
	if len(m.completions) == 0 {
		prefix := strings.TrimSpace(m.command.value())
		if strings.HasPrefix(m.command.value(), "layout ") {
			for _, style := range layoutStyles {
				candidate := "layout " + style
				if strings.HasPrefix(candidate, prefix) {
					m.completions = append(m.completions, candidate)
				}
			}
		}
		if !strings.HasPrefix(m.command.value(), "layout ") {
			for _, command := range commands {
				if strings.HasPrefix(command.name, prefix) {
					m.completions = append(m.completions, command.name)
				}
			}
		}
		if len(m.completions) == 0 {
			m.status = "No matching commands. :?"
			return
		}
		m.completionIndex = 0
		if direction < 0 {
			m.completionIndex = len(m.completions) - 1
		}
	} else {
		m.completionIndex = (m.completionIndex + direction + len(m.completions)) % len(m.completions)
	}
	m.command.set(m.completions[m.completionIndex])
	appLog.Debug("ui.command_completed", "command", m.command.value(), "matches", len(m.completions))
}

func (m *model) helpLines() []string {
	rows := []string{"사용 가능한 명령어", "Esc / Enter: 돌아가기 · j/k: 스크롤", ""}
	for _, command := range commands {
		rows = append(rows, fmt.Sprintf("  :%-8s %s", command.name, command.description))
	}
	rows = append(rows, "", "명령 입력 중 Tab: 다음 후보 · Shift-Tab: 이전 후보", "입력한 접두어에 맞는 명령만 순환합니다. 예: :s → :servers → :send")
	var lines []string
	for _, row := range rows {
		lines = append(lines, strings.Split(ansi.Hardwrap(row, m.width, true), "\n")...)
	}
	return lines
}
