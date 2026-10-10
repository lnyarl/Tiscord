package main

import "strings"

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
