package main

import (
	"fmt"
	"reflect"
	"slices"
)

type configField struct {
	section, field, label string
	choices               []string
	minimum, maximum      int
}

var configFields = []configField{
	{"Layout", "Style", "레이아웃", layoutStyles, 0, 0},
	{"Layout", "AuthorName", "작성자 이름", []string{"username", "display"}, 0, 0},
	{"Layout", "GroupSpacing", "메시지 그룹 사이 빈 줄", nil, 0, 3},
	{"Layout", "ShowTimestamps", "시간 표시", nil, 0, 0},
	{"Layout", "TimestampFormat", "시간 형식", []string{"HH:mm", "HH:mm:ss", "YYYY-MM-DD HH:mm"}, 0, 0},
	{"Layout", "InputMaxLines", "입력창 최대 줄 수", nil, 1, 10},
	{"Layout", "InputGapLines", "입력창 위 빈 줄", nil, 0, 3},
	{"Layout", "ShowMetadata", "메타정보 표시", nil, 0, 0},
	{"Layout", "ShowTyping", "입력 중 표시", nil, 0, 0},
	{"Behavior", "GroupMessages", "연속 메시지 묶기", nil, 0, 0},
	{"Behavior", "GroupWindowSeconds", "메시지 묶기 간격 (초)", nil, 1, 86400},
	{"Behavior", "SendKey", "전송 키", []string{"ctrl+s", "enter"}, 0, 0},
	{"Behavior", "StartMode", "채널 진입 모드", []string{"normal", "insert"}, 0, 0},
	{"Behavior", "KoreanShortcuts", "한글 Vim 키 변환", nil, 0, 0},
	{"Logging", "Level", "로그 수준", []string{"debug", "info", "warn", "error", "off"}, 0, 0},
}

func (f configField) value(c *config) reflect.Value {
	return reflect.ValueOf(c).Elem().FieldByName(f.section).FieldByName(f.field)
}

func (m *model) configKey(key string) {
	switch key {
	case "esc":
		m.showConfig = false
		m.status = "설정 변경 취소"
	case "j", "down":
		m.configIndex = min(len(configFields)-1, m.configIndex+1)
	case "k", "up":
		m.configIndex = max(0, m.configIndex-1)
	case "g":
		m.configIndex = 0
	case "G":
		m.configIndex = len(configFields) - 1
	case "h", "left", "l", "right", "enter", "space":
		f := configFields[m.configIndex]
		v := f.value(&m.configDraft)
		direction := 1
		if key == "h" || key == "left" {
			direction = -1
		}
		switch v.Kind() {
		case reflect.Bool:
			v.SetBool(!v.Bool())
		case reflect.String:
			i := (slices.Index(f.choices, v.String()) + direction + len(f.choices)) % len(f.choices)
			v.SetString(f.choices[i])
		case reflect.Int:
			v.SetInt(int64(max(f.minimum, min(f.maximum, int(v.Int())+direction))))
		}
		m.configStatus = "저장 전 · Ctrl-S로 저장"
	case "ctrl+s":
		if m.configPath == "" {
			m.configStatus = "설정 저장 실패: 설정 파일 경로 없음"
			return
		}
		if err := saveConfig(m.configPath, m.configDraft); err != nil {
			m.configStatus = "설정 저장 실패: " + err.Error()
			return
		}
		m.config = m.configDraft
		if m.logLevel != nil {
			m.logLevel.Set(m.config.logLevel())
		}
		clear(m.typists)
		m.showConfig = false
		m.status = "설정 저장 및 적용 완료"
	}
}

func (m *model) configLines() []string {
	lines := []string{"설정 · 변경 후 Ctrl-S로 저장 및 적용", "j/k: 이동 · h/l/Enter: 변경 · Esc: 취소", "채널 진입 모드는 다음 채널 진입부터 적용", m.configStatus}
	for i, f := range configFields {
		prefix := "  "
		if i == m.configIndex {
			prefix = "> "
		}
		v := f.value(&m.configDraft)
		value := fmt.Sprint(v.Interface())
		if v.Kind() == reflect.Bool {
			value = "OFF"
			if v.Bool() {
				value = "ON"
			}
		}
		lines = append(lines, fmt.Sprintf("%s%s: %s", prefix, f.label, value))
	}
	return lines
}
