package main

import (
	"fmt"
	"slices"
	"strings"
)

var layoutStyles = []string{"plain", "log", "editor"}

func validLayout(style string) bool { return slices.Contains(layoutStyles, style) }

func (m *model) changeLayout(command string) {
	parts := strings.Fields(command)
	style := ""
	if len(parts) == 1 {
		style = layoutStyles[(slices.Index(layoutStyles, m.config.Layout.Style)+1)%len(layoutStyles)]
	} else if len(parts) == 2 {
		style = parts[1]
	}
	if !validLayout(style) {
		m.status = "Usage: :layout [plain|log|editor]"
		return
	}
	m.config.Layout.Style = style
	m.showHelp = false
	m.status = fmt.Sprintf("Layout: %s", style)
	if m.configPath != "" {
		if err := saveLayout(m.configPath, style); err != nil {
			m.status = fmt.Sprintf("Layout: %s · 설정 저장 실패: %v", style, err)
			appLog.Error("config.layout_save_failed", "path", m.configPath, "error", err)
		} else {
			m.status += " · 저장됨"
		}
	}
	appLog.Info("ui.layout_changed", "style", style)
}
