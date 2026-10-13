package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type slashState struct {
	startedAt                                 time.Time
	created                                   bool
	open, loading, submitting                 bool
	channelID, guildID, previousMode          string
	commands                                  []slashCommand
	command                                   *slashCommand
	index, optionIndex                        int
	input                                     editor
	values                                    map[string]any
	choices                                   []slashChoice
	choiceIndex                               int
	status, autocompleteNonce, executionNonce string
	revision                                  uint64
}
type slashLoaded struct {
	epoch    uint64
	commands []slashCommand
	err      error
}
type slashPosted struct {
	epoch        uint64
	nonce        string
	autocomplete bool
	err          error
}
type slashSuggest struct{ epoch, revision uint64 }
type slashTimeout struct {
	epoch        uint64
	nonce        string
	autocomplete bool
}

func (m *model) openSlash() tea.Cmd {
	client, ok := m.client.(slashClient)
	if !ok || m.screen != "chat" || m.busy != "" {
		m.status = m.tr("Slash commands are unavailable on this screen")
		return nil
	}
	m.slashEpoch++
	m.slash = slashState{open: true, loading: true, channelID: m.channel.ID, guildID: m.guild.ID, previousMode: m.mode, values: make(map[string]any), status: m.tr("Loading commands…")}
	m.mode = "INSERT"
	epoch, guildID, channelID := m.slashEpoch, m.guild.ID, m.channel.ID
	return func() tea.Msg {
		commands, err := client.Commands(m.ctx, guildID, channelID)
		return slashLoaded{epoch, commands, err}
	}
}
func (m *model) closeSlash() {
	m.mode = m.slash.previousMode
	m.slash.open = false
	m.slashEpoch++
}
func (s *slashState) matches() []slashCommand {
	query := strings.ToLower(strings.TrimPrefix(s.input.value(), "/"))
	var result []slashCommand
	for _, command := range s.commands {
		if strings.Contains(strings.ToLower(command.name()+" "+command.Root.Description+" "+command.Application), query) {
			result = append(result, command)
		}
	}
	return result
}
func (s *slashState) option() *slashOption {
	if s.command == nil || s.optionIndex >= len(s.command.Options) {
		return nil
	}
	return &s.command.Options[s.optionIndex]
}
func (s *slashState) resetOption() {
	s.input.set("")
	s.choices = nil
	s.choiceIndex = 0
	s.autocompleteNonce = ""
	s.revision++
	s.status = ""
	if option := s.option(); option != nil {
		s.choices = append([]slashChoice{}, option.Choices...)
		if option.Type == 5 {
			s.choices = []slashChoice{{Name: "true", Value: true}, {Name: "false", Value: false}}
		}
		if value, ok := s.values[option.Name]; ok {
			s.input.set(fmt.Sprint(value))
		}
	}
}
func (s *slashState) hint(language string) string {
	if s.status != "" {
		return s.status
	}
	if s.command == nil {
		return localize(language, "Type to search · ↑↓/Tab: matches · Enter: select · Esc: close")
	}
	if option := s.option(); option != nil {
		required := localize(language, "Optional · Enter with an empty value to skip")
		if option.Required {
			required = localize(language, "Required")
		}
		typeName := map[int]string{3: localize(language, "String"), 4: localize(language, "Integer"), 5: "true/false", 6: localize(language, "User ID"), 7: localize(language, "Channel ID"), 8: localize(language, "Role ID"), 9: localize(language, "User/role ID"), 10: localize(language, "Number"), 11: localize(language, "Attachment (unsupported)")}[option.Type]
		return fmt.Sprintf("%s · %s · %s", required, typeName, singleLine(option.Description))
	}
	return localize(language, "Ctrl-S: run · Esc: previous argument · responses appear as bot messages")
}

func (m *model) slashKey(key tea.KeyPressMsg) tea.Cmd {
	s := &m.slash
	name := keyName(key)
	if name == "esc" {
		if s.submitting {
			m.closeSlash()
			return nil
		}
		if s.command != nil {
			if s.optionIndex > 0 {
				s.optionIndex--
				s.resetOption()
			} else {
				s.command = nil
				s.revision++
				s.autocompleteNonce = ""
				s.choices = nil
				s.input.set("")
				s.index = 0
				s.status = ""
			}
		} else {
			m.closeSlash()
		}
		return nil
	}
	if s.loading || s.submitting {
		return nil
	}
	if s.command == nil {
		matches := s.matches()
		s.index = min(s.index, max(0, len(matches)-1))
		switch name {
		case "tab", "down", "ctrl+n":
			if len(matches) > 0 {
				s.index = (s.index + 1) % len(matches)
			}
		case "shift+tab", "up", "ctrl+p":
			if len(matches) > 0 {
				s.index = (s.index + len(matches) - 1) % len(matches)
			}
		case "enter":
			if len(matches) > 0 {
				chosen := matches[s.index]
				s.command = &chosen
				s.values = make(map[string]any)
				s.optionIndex = 0
				s.resetOption()
			}
		default:
			s.input.key(key, false)
			s.index = 0
		}
		return nil
	}
	option := s.option()
	if option == nil {
		if name == "ctrl+s" {
			return m.submitSlash()
		}
		return nil
	}
	if name == "tab" || name == "shift+tab" || name == "up" || name == "down" {
		if len(s.choices) > 0 {
			if name == "up" || name == "shift+tab" {
				s.choiceIndex = (s.choiceIndex + len(s.choices) - 1) % len(s.choices)
			}
			choice := s.choices[s.choiceIndex]
			s.input.set(fmt.Sprint(choice.Value))
			if name == "tab" || name == "down" {
				s.choiceIndex = (s.choiceIndex + 1) % len(s.choices)
			}
		}
		return nil
	}
	if name == "enter" {
		if strings.TrimSpace(s.input.value()) == "" && !option.Required {
			delete(s.values, option.Name)
		} else {
			value, err := optionValue(*option, s.input.value())
			if err != nil {
				s.status = m.tr(err.Error())
				return nil
			}
			s.values[option.Name] = value
		}
		s.optionIndex++
		s.resetOption()
		return nil
	}
	if name == "ctrl+s" {
		return nil
	}
	before := s.input.value()
	s.input.key(key, false)
	if s.input.value() != before {
		return m.scheduleSlashAutocomplete()
	}
	return nil
}
func (m *model) scheduleSlashAutocomplete() tea.Cmd {
	s := &m.slash
	s.status = ""
	s.revision++
	s.autocompleteNonce = ""
	option := s.option()
	if option == nil || !option.Autocomplete {
		return nil
	}
	s.choices = nil
	s.choiceIndex = 0
	epoch, revision := m.slashEpoch, s.revision
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg { return slashSuggest{epoch, revision} })
}
func (m *model) slashRequest(kind int) (interactionRequest, error) {
	s := &m.slash
	if !m.gatewayOnline || m.gatewayStopped || m.syncRequired || s.channelID != m.channel.ID {
		return interactionRequest{}, errors.New(m.tr("Wait for connection and channel recovery"))
	}
	session := m.slashSession
	if m.gateway == nil {
		session = "offline-demo"
	}
	if session == "" {
		return interactionRequest{}, errors.New(m.tr("Gateway session is unavailable"))
	}
	focused := ""
	var query any
	if kind == 4 {
		option := s.option()
		if option == nil || !option.Autocomplete {
			return interactionRequest{}, errors.New(m.tr("This argument does not support autocomplete"))
		}
		focused = option.Name
		query = s.input.value()
		if option.Type == 4 || option.Type == 10 {
			var err error
			query, err = optionValue(*option, s.input.value())
			if err != nil {
				return interactionRequest{}, err
			}
		}
	}
	return interactionRequest{kind, s.command.Root.ApplicationID, s.guildID, s.channelID, session, interactionNonce(), s.command.data(s.values, focused, query)}, nil
}
func (m *model) postSlash(request interactionRequest) tea.Cmd {
	client := m.client.(slashClient)
	epoch := m.slashEpoch
	return func() tea.Msg {
		timeout := 10 * time.Second
		if request.Type == 4 {
			timeout = 3 * time.Second
		}
		ctx, cancel := context.WithTimeout(m.ctx, timeout)
		defer cancel()
		err := client.Interact(ctx, request)
		return slashPosted{epoch, request.Nonce, request.Type == 4, err}
	}
}
func (m *model) submitSlash() tea.Cmd {
	request, err := m.slashRequest(2)
	if err != nil {
		m.slash.status = m.tr(err.Error())
		return nil
	}
	m.slash.submitting = true
	m.slash.startedAt = time.Now()
	m.slash.created = false
	m.slash.executionNonce = request.Nonce
	m.slash.status = m.tr("Submitting… · no automatic retries")
	epoch := m.slashEpoch
	return tea.Batch(m.postSlash(request), tea.Tick(15*time.Second, func(time.Time) tea.Msg { return slashTimeout{epoch, request.Nonce, false} }))
}
func (m *model) updateSlash(msg tea.Msg) (tea.Cmd, bool) {
	s := &m.slash
	switch msg := msg.(type) {
	case slashLoaded:
		if msg.epoch != m.slashEpoch || !s.open {
			return nil, true
		}
		s.loading = false
		if msg.err != nil {
			s.status = m.tr("Could not load commands: ") + m.tr(msg.err.Error())
		} else {
			s.commands = msg.commands
			s.status = ""
		}
		return nil, true
	case slashSuggest:
		if msg.epoch != m.slashEpoch || msg.revision != s.revision || !s.open {
			return nil, true
		}
		request, err := m.slashRequest(4)
		if err != nil {
			s.status = m.tr(err.Error())
			return nil, true
		}
		s.autocompleteNonce = request.Nonce
		return tea.Batch(m.postSlash(request), tea.Tick(4*time.Second, func(time.Time) tea.Msg { return slashTimeout{msg.epoch, request.Nonce, true} })), true
	case slashPosted:
		if msg.epoch != m.slashEpoch || !s.open {
			return nil, true
		}
		if msg.autocomplete {
			if msg.nonce == s.autocompleteNonce && msg.err != nil {
				s.status = m.tr("Autocomplete failed: ") + m.tr(msg.err.Error())
				s.autocompleteNonce = ""
			}
		} else if msg.nonce == s.executionNonce {
			if msg.err != nil {
				s.status = m.tr("Execution needs verification: ") + m.tr(msg.err.Error())
				s.submitting = false
				s.executionNonce = ""
			} else if m.gateway == nil {
				m.status = m.tr("Offline demo command completed")
				m.closeSlash()
			} else {
				s.status = m.tr("Request accepted · waiting for bot response…")
			}
		}
		return nil, true
	case slashTimeout:
		if msg.epoch != m.slashEpoch || !s.open {
			return nil, true
		}
		if msg.autocomplete && msg.nonce == s.autocompleteNonce {
			s.status = m.tr("No autocomplete response · enter a value manually")
			s.autocompleteNonce = ""
		}
		if !msg.autocomplete && msg.nonce == s.executionNonce && s.submitting {
			s.submitting = false
			s.executionNonce = ""
			s.status = m.tr("Check the result in Discord before retrying")
		}
		return nil, true
	}
	return nil, false
}
func (m *model) slashEvent(event gatewayEvent) {
	s := &m.slash
	if !s.open {
		return
	}
	var data struct {
		Nonce      string        `json:"nonce"`
		Choices    []slashChoice `json:"choices"`
		ReasonCode *int          `json:"reason_code"`
	}
	if json.Unmarshal(event.data, &data) != nil {
		return
	}
	if event.kind == "APPLICATION_COMMAND_AUTOCOMPLETE_RESPONSE" && data.Nonce != "" && data.Nonce == s.autocompleteNonce {
		s.choices = data.Choices
		s.choiceIndex = 0
		s.autocompleteNonce = ""
		s.status = ""
	}
	if data.Nonce != "" && data.Nonce == s.executionNonce {
		switch event.kind {
		case "INTERACTION_CREATE":
			s.created = true
			s.status = m.tr("Discord created the command · waiting for bot response…")
			appLog.Info("slash.created", "application_id", s.command.Root.ApplicationID, "command_id", s.command.Root.ID, "elapsed_ms", time.Since(s.startedAt).Milliseconds())
		case "INTERACTION_SUCCESS":
			appLog.Info("slash.succeeded", "application_id", s.command.Root.ApplicationID, "command_id", s.command.Root.ID, "elapsed_ms", time.Since(s.startedAt).Milliseconds())
			m.status = m.tr("Slash command completed")
			m.closeSlash()
		case "INTERACTION_FAILURE":
			s.submitting = false
			s.executionNonce = ""
			s.status = interactionFailureLabel(data.ReasonCode, m.config.Behavior.Language)
			reason := "missing"
			if data.ReasonCode != nil {
				reason = fmt.Sprint(*data.ReasonCode)
			}
			appLog.Warn("slash.failed", "application_id", s.command.Root.ApplicationID, "command_id", s.command.Root.ID, "reason_code", reason, "created", s.created, "elapsed_ms", time.Since(s.startedAt).Milliseconds())
		}
	}
}

func interactionFailureLabel(reason *int, language string) string {
	if reason == nil {
		return localize(language, "Discord command failed · no reason code")
	}
	if *reason == 2 {
		return localize(language, "Command response timed out (code 2) · check the same command in Discord")
	}
	return localize(language, "Discord command failed (code %d)", *reason)
}
func (m *model) slashLines() ([]string, int) {
	s := &m.slash
	lines := []string{m.tr("Discord bot slash commands"), m.tr("Type to search · ↑↓/Tab: matches · Enter: select · Esc: return"), ""}
	if s.command == nil {
		matches := s.matches()
		if len(matches) == 0 {
			lines = append(lines, m.tr("No matching commands · ")+s.status)
		}
		for i, command := range matches {
			prefix := "  "
			if i == s.index {
				prefix = "> "
			}
			lines = append(lines, fmt.Sprintf("%s/%s [%s] · %s", prefix, command.name(), singleLine(command.Application), singleLine(command.Root.Description)))
		}
		return lines, min(len(lines)-1, s.index+3)
	}
	lines = append(lines, "/"+s.command.name()+" ["+singleLine(s.command.Application)+"]")
	target := len(lines) - 1
	for i, option := range s.command.Options {
		prefix := "  "
		if i == s.optionIndex {
			prefix = "> "
			target = len(lines)
		}
		value := m.tr("(not entered)")
		if v, ok := s.values[option.Name]; ok {
			value = singleLine(fmt.Sprint(v))
		}
		lines = append(lines, prefix+option.Name+": "+value)
		if i == s.optionIndex {
			for _, choice := range s.choices {
				name := choice.LocalizedName
				if name == "" {
					name = choice.Name
				}
				lines = append(lines, "    "+singleLine(name)+" → "+singleLine(fmt.Sprint(choice.Value)))
			}
		}
	}
	if s.option() == nil {
		lines = append(lines, "", m.tr("Review the arguments and press Ctrl-S to run"))
		target = len(lines) - 1
	}
	return lines, target
}
