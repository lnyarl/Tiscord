package main

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

var messageURL = regexp.MustCompile("https?://[^\\s<>\"\\x60]+")
var messageHyperlink = regexp.MustCompile("\x1b\\]8;;([^\x07]*)\x07")

func linkMessageURLs(text string) string {
	return messageURL.ReplaceAllStringFunc(text, func(candidate string) string {
		target := strings.TrimRight(candidate, ".,;!")
		for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}} {
			for strings.HasSuffix(target, pair[1]) && strings.Count(target, pair[1]) > strings.Count(target, pair[0]) {
				target = strings.TrimSuffix(target, pair[1])
			}
		}
		parsed, err := url.Parse(target)
		if err != nil || parsed.Host == "" {
			return candidate
		}
		return ansi.SetHyperlink(target) + target + ansi.ResetHyperlink() + candidate[len(target):]
	})
}

func wrapMessageLine(text string, width int) []string {
	text = linkMessageURLs(text)
	rows := strings.Split(ansi.Hardwrap(text, width, true), "\n")
	active := ""
	for i, row := range rows {
		prefix := ""
		if active != "" {
			prefix = ansi.SetHyperlink(active)
		}
		for _, match := range messageHyperlink.FindAllStringSubmatch(row, -1) {
			active = match[1]
		}
		rows[i] = prefix + row
		if active != "" {
			rows[i] += ansi.ResetHyperlink()
		}
	}
	return rows
}

func messageText(item message) string {
	var parts []string
	appendText := func(text string) {
		if text = clean(text); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	appendURL := func(url string) {
		url = singleLine(url)
		if url != "" && !strings.Contains(strings.Join(parts, "\n"), url) {
			parts = append(parts, url)
		}
	}
	appendText(item.Content)
	for _, attachment := range item.Attachments {
		appendURL(attachment.URL)
	}
	for _, embed := range item.Embeds {
		appendText(embed.Author.Name)
		appendText(embed.Title)
		appendText(embed.Description)
		for _, field := range embed.Fields {
			name, value := clean(field.Name), clean(field.Value)
			if strings.TrimSpace(name) != "" && strings.TrimSpace(value) != "" {
				appendText(name + ": " + value)
			} else {
				appendText(name + value)
			}
		}
		appendText(embed.Footer.Text)
		for _, url := range []string{embed.URL, embed.Image.URL, embed.Thumbnail.URL, embed.Video.URL} {
			appendURL(url)
		}
	}
	if len(parts) == 0 {
		return "[non-text message]"
	}
	return strings.Join(parts, "\n")
}
