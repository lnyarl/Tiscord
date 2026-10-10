package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf16"
)

type slashChoice struct {
	Name          string `json:"name"`
	LocalizedName string `json:"name_localized"`
	Value         any    `json:"value"`
}
type slashOption struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Type         int           `json:"type"`
	Required     bool          `json:"required"`
	Autocomplete bool          `json:"autocomplete"`
	Choices      []slashChoice `json:"choices,omitempty"`
	Options      []slashOption `json:"options,omitempty"`
	MinValue     *float64      `json:"min_value,omitempty"`
	MaxValue     *float64      `json:"max_value,omitempty"`
	MinLength    *int          `json:"min_length,omitempty"`
	MaxLength    *int          `json:"max_length,omitempty"`
}
type appCommand struct {
	ID            string        `json:"id"`
	ApplicationID string        `json:"application_id"`
	GuildID       string        `json:"guild_id,omitempty"`
	Version       string        `json:"version"`
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	Type          int           `json:"type"`
	Options       []slashOption `json:"options,omitempty"`
}
type slashCommand struct {
	Root        appCommand
	Path        []slashOption
	Options     []slashOption
	Application string
}

func (c slashCommand) name() string {
	name := c.Root.Name
	for _, p := range c.Path {
		name += " " + p.Name
	}
	return name
}

type interactionOption struct {
	Type    int                 `json:"type"`
	Name    string              `json:"name"`
	Value   any                 `json:"value,omitempty"`
	Options []interactionOption `json:"options,omitempty"`
	Focused bool                `json:"focused,omitempty"`
}
type interactionData struct {
	ID      string              `json:"id"`
	Version string              `json:"version"`
	Name    string              `json:"name"`
	Type    int                 `json:"type"`
	GuildID string              `json:"guild_id,omitempty"`
	Options []interactionOption `json:"options"`
	Command appCommand          `json:"application_command"`
}
type interactionRequest struct {
	Type          int             `json:"type"`
	ApplicationID string          `json:"application_id"`
	GuildID       string          `json:"guild_id,omitempty"`
	ChannelID     string          `json:"channel_id"`
	SessionID     string          `json:"session_id"`
	Nonce         string          `json:"nonce"`
	Data          interactionData `json:"data"`
}
type slashClient interface {
	Commands(context.Context, string, string) ([]slashCommand, error)
	Interact(context.Context, interactionRequest) error
}

func (c *restClient) Commands(ctx context.Context, guildID, channelID string) ([]slashCommand, error) {
	path := "/channels/" + url.PathEscape(channelID) + "/application-command-index"
	if guildID != "" {
		path = "/guilds/" + url.PathEscape(guildID) + "/application-command-index"
	}
	var commands []slashCommand
	seen := make(map[string]bool)
	for _, endpoint := range []string{path, "/users/@me/application-command-index"} {
		var index struct {
			Commands     []appCommand `json:"application_commands"`
			Applications []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"applications"`
		}
		if err := c.request(ctx, "GET", endpoint, nil, &index); err != nil {
			return nil, err
		}
		if index.Commands == nil {
			return nil, errors.New("명령 목록 준비 중 · 잠시 후 /로 다시 열어주세요")
		}
		names := make(map[string]string)
		for _, app := range index.Applications {
			names[app.ID] = app.Name
		}
		for _, root := range index.Commands {
			if root.Type != 1 || seen[root.ID] {
				continue
			}
			seen[root.ID] = true
			commands = append(commands, flattenCommands(root, nil, root.Options, names[root.ApplicationID])...)
		}
	}
	slices.SortFunc(commands, func(a, b slashCommand) int { return strings.Compare(a.name()+a.Application, b.name()+b.Application) })
	return commands, nil
}
func flattenCommands(root appCommand, path []slashOption, options []slashOption, app string) []slashCommand {
	var result []slashCommand
	for _, option := range options {
		if option.Type == 1 || option.Type == 2 {
			next := append(append([]slashOption{}, path...), option)
			result = append(result, flattenCommands(root, next, option.Options, app)...)
		}
	}
	if len(result) == 0 {
		result = append(result, slashCommand{root, path, options, app})
	}
	return result
}
func (c *restClient) Interact(ctx context.Context, request interactionRequest) error {
	return c.request(ctx, "POST", "/interactions", request, nil)
}
func (c *demoClient) Commands(context.Context, string, string) ([]slashCommand, error) {
	root := appCommand{ID: "demo-command", ApplicationID: "demo-app", Version: "1", Name: "demo", Type: 1, Description: "오프라인 입력 도우미", Options: []slashOption{
		{Name: "text", Type: 3, Required: true, Description: "표시할 텍스트"},
		{Name: "style", Type: 3, Description: "형식", Choices: []slashChoice{{Name: "짧게", Value: "short"}, {Name: "길게", Value: "long"}}},
	}}
	return flattenCommands(root, nil, root.Options, "Demo bot"), nil
}
func (c *demoClient) Interact(context.Context, interactionRequest) error { return nil }

func optionValue(option slashOption, text string) (any, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("값을 입력해주세요")
	}
	var value any
	switch option.Type {
	case 3:
		length := len(utf16.Encode([]rune(text)))
		if option.MinLength != nil && length < *option.MinLength || option.MaxLength != nil && length > *option.MaxLength {
			return nil, errors.New("문자열 길이 범위를 벗어났습니다")
		}
		value = text
	case 4, 10:
		number, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, errors.New("숫자를 입력해주세요")
		}
		if option.Type == 4 && (math.Trunc(number) != number || math.Abs(number) > 9007199254740991) {
			return nil, errors.New("유효한 정수를 입력해주세요")
		}
		if option.MinValue != nil && number < *option.MinValue || option.MaxValue != nil && number > *option.MaxValue {
			return nil, errors.New("숫자 범위를 벗어났습니다")
		}
		value = number
	case 5:
		if text != "true" && text != "false" {
			return nil, errors.New("true 또는 false를 입력해주세요")
		}
		value = text == "true"
	case 6, 7, 8, 9:
		id := strings.Trim(text, "<@!#&>")
		if number, err := strconv.ParseUint(id, 10, 64); err != nil || number == 0 {
			return nil, errors.New("사용자·채널·역할의 숫자 ID를 입력해주세요")
		}
		value = id
	default:
		return nil, errors.New("첨부파일 인자는 아직 지원하지 않습니다")
	}
	if len(option.Choices) > 0 {
		for _, choice := range option.Choices {
			if fmt.Sprint(choice.Value) == fmt.Sprint(value) {
				return value, nil
			}
		}
		return nil, errors.New("제공된 선택지를 골라주세요")
	}
	return value, nil
}
func (c slashCommand) data(values map[string]any, focused string, query any) interactionData {
	options := []interactionOption{}
	for _, option := range c.Options {
		value, exists := values[option.Name]
		if option.Name == focused {
			value, exists = query, true
		}
		if exists {
			options = append(options, interactionOption{Type: option.Type, Name: option.Name, Value: value, Focused: option.Name == focused})
		}
	}
	for i := len(c.Path) - 1; i >= 0; i-- {
		options = []interactionOption{{Type: c.Path[i].Type, Name: c.Path[i].Name, Options: options}}
	}
	return interactionData{c.Root.ID, c.Root.Version, c.Root.Name, c.Root.Type, c.Root.GuildID, options, c.Root}
}

var interactionSequence atomic.Uint64

func interactionNonce() string {
	return strconv.FormatUint(uint64(time.Now().UnixMilli()-1420070400000)<<22|interactionSequence.Add(1)&((1<<22)-1), 10)
}
