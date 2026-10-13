package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Bubble Tea handles input and commands; chat output uses terminal autowrap.
type terminalModel struct {
	model   *model
	output  io.Writer
	started bool
	mouse   bool
	last    string
	err     error
}

func (t *terminalModel) Init() tea.Cmd { return t.model.Init() }
func (t *terminalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := t.model.Update(msg)
	if t.err != nil {
		return t, tea.Quit
	}
	return t, cmd
}

func nativeFrame(view tea.View, chat *chatFrame) string {
	var out strings.Builder
	out.WriteString("\x1b[?7h\x1b[H\x1b[2J")
	rows := strings.Split(view.Content, "\n")
	first := 0
	if chat != nil {
		end := min(len(chat.layout.rows), chat.start+chat.height)
		for _, logical := range chat.layout.logical {
			start, stop := max(chat.start, logical.start), min(end, logical.end)
			if start >= stop {
				continue
			}
			fmt.Fprintf(&out, "\x1b[%d;1H", start-chat.start+1)
			if start == logical.start && stop == logical.end {
				out.WriteString(logical.text)
			} else {
				// Partial viewport edges still use contiguous text, never LF.
				out.WriteString(strings.Join(chat.layout.rows[start:stop], ""))
			}
			out.WriteString(ansi.ResetHyperlink())
		}
		first = chat.height
	}
	for i := first; i < len(rows); i++ {
		fmt.Fprintf(&out, "\x1b[%d;1H%s", i+1, rows[i])
	}
	return out.String()
}

func (t *terminalModel) View() tea.View {
	view, chat := t.model.viewFrame()
	frame := nativeFrame(view, chat)
	mouse := view.MouseMode != tea.MouseModeNone
	var prefix string
	if !t.started {
		prefix = "\x1b[?1049h\x1b[?25l\x1b[?2004h\x1b[>4;2m\x1b[>29u\x1b[?u"
		t.started = true
	}
	if mouse != t.mouse {
		if mouse {
			prefix += "\x1b[?1002h\x1b[?1006h"
		} else {
			prefix += "\x1b[?1002l\x1b[?1006l"
		}
		t.mouse = mouse
	}
	if frame != t.last || prefix != "" {
		_, t.err = io.WriteString(t.output, prefix+"\x1b[?2026h"+frame+"\x1b[?2026l")
		t.last = frame
	}
	return view
}

func (t *terminalModel) close() {
	if !t.started {
		return
	}
	_, err := io.WriteString(t.output, "\x1b[?2026l\x1b]8;;\x07\x1b[0m\x1b[?1002l\x1b[?1006l\x1b[?2004l\x1b[<u\x1b[>4;0m\x1b[?25h\x1b[?1049l")
	if t.err == nil {
		t.err = err
	}
	t.started = false
}

func runTerminalUI(ctx context.Context, model *model, output io.Writer) error {
	restore, err := enableTerminalOutput(output)
	if err != nil {
		return err
	}
	defer restore()
	t := &terminalModel{model: model, output: output}
	defer t.close()
	_, err = tea.NewProgram(t, tea.WithContext(ctx), tea.WithOutput(output), tea.WithoutRenderer()).Run()
	t.close()
	if err != nil {
		return err
	}
	return t.err
}
