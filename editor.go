package main

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

type editor struct {
	text   []rune
	cursor int
}

func (e *editor) value() string    { return string(e.text) }
func (e *editor) set(value string) { e.text = []rune(value); e.cursor = len(e.text) }
func (e *editor) insert(value []rune) {
	result := append([]rune{}, e.text[:e.cursor]...)
	result = append(result, value...)
	result = append(result, e.text[e.cursor:]...)
	e.text = result
	e.cursor += len(value)
}
func (e *editor) remove(start, end int) {
	start = max(0, min(start, len(e.text)))
	end = max(start, min(end, len(e.text)))
	e.text = append(e.text[:start], e.text[end:]...)
	e.cursor = start
}
func (e *editor) key(key tea.KeyPressMsg, multiline bool) {
	if multiline && key.Code == tea.KeyEnter && key.Mod&(tea.ModCtrl|tea.ModShift) != 0 && key.Mod&(tea.ModAlt|tea.ModSuper) == 0 {
		e.insert([]rune{'\n'})
		return
	}
	if key.Text != "" && key.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0 {
		e.insert([]rune(clean(key.Text)))
		return
	}
	switch keyName(key) {
	case "space":
		e.insert([]rune{' '})
	case "enter", "ctrl+j", "ctrl+enter", "shift+enter":
		if multiline {
			e.insert([]rune{'\n'})
		}
	case "backspace", "ctrl+h":
		if e.cursor > 0 {
			e.remove(e.cursor-1, e.cursor)
		}
	case "delete":
		e.remove(e.cursor, e.cursor+1)
	case "left":
		e.cursor = max(0, e.cursor-1)
	case "right":
		e.cursor = min(len(e.text), e.cursor+1)
	case "home", "ctrl+a":
		e.lineStart()
	case "end", "ctrl+e":
		e.lineEnd()
	}
}
func (e *editor) lineStart() {
	for e.cursor > 0 && e.text[e.cursor-1] != '\n' {
		e.cursor--
	}
}
func (e *editor) lineEnd() {
	for e.cursor < len(e.text) && e.text[e.cursor] != '\n' {
		e.cursor++
	}
}
func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
func (e *editor) word(delta int) {
	if delta > 0 {
		if e.cursor < len(e.text) {
			kind := wordRune(e.text[e.cursor])
			for e.cursor < len(e.text) && !unicode.IsSpace(e.text[e.cursor]) && wordRune(e.text[e.cursor]) == kind {
				e.cursor++
			}
		}
		for e.cursor < len(e.text) && unicode.IsSpace(e.text[e.cursor]) {
			e.cursor++
		}
	} else {
		e.cursor = max(0, e.cursor-1)
		for e.cursor > 0 && unicode.IsSpace(e.text[e.cursor]) {
			e.cursor--
		}
		if e.cursor > 0 {
			kind := wordRune(e.text[e.cursor])
			for e.cursor > 0 && !unicode.IsSpace(e.text[e.cursor-1]) && wordRune(e.text[e.cursor-1]) == kind {
				e.cursor--
			}
		}
	}
}
func (e *editor) deleteLine() {
	e.lineStart()
	start := e.cursor
	e.lineEnd()
	e.remove(start, min(len(e.text), e.cursor+1))
}
func clean(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}
func singleLine(text string) string { return strings.ReplaceAll(clean(text), "\n", " ") }
