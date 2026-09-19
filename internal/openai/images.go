package openai

import (
	"encoding/json"
	"strings"
)

// ImageURLsFromMessages collects image URLs from OpenAI chat message content
// (image_url parts). Order is preserved; duplicates are kept only once.
func ImageURLsFromMessages(messages []ChatMessage) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, m := range messages {
		for _, url := range imageURLsFromContent(m.Content) {
			if _, ok := seen[url]; ok {
				continue
			}
			seen[url] = struct{}{}
			out = append(out, url)
		}
	}
	return out
}

func imageURLsFromContent(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var parts []ContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil
	}
	var out []string
	for _, p := range parts {
		switch p.Type {
		case "image_url":
			if p.ImageURL != nil {
				if u := strings.TrimSpace(p.ImageURL.URL); u != "" {
					out = append(out, u)
				}
			}
		}
	}
	return out
}

// AppendImagePaths adds absolute image paths to a CLI prompt so the Cursor
// agent can open them via tool calls (see Cursor headless CLI docs).
func AppendImagePaths(prompt string, paths []string) string {
	if len(paths) == 0 {
		return prompt
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(prompt, "\n"))
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	if len(paths) == 1 {
		b.WriteString("Attached image (read this file): ")
		b.WriteString(paths[0])
		return b.String()
	}
	b.WriteString("Attached images (read these files):\n")
	for _, p := range paths {
		b.WriteString("- ")
		b.WriteString(p)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func marshalMessageContent(text string, imageURLs []string) (json.RawMessage, error) {
	text = strings.TrimSpace(text)
	if len(imageURLs) == 0 {
		return json.Marshal(text)
	}
	parts := make([]ContentPart, 0, 1+len(imageURLs))
	if text != "" {
		parts = append(parts, ContentPart{Type: "text", Text: text})
	}
	for _, u := range imageURLs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		parts = append(parts, ContentPart{
			Type: "image_url",
			ImageURL: &struct {
				URL string `json:"url"`
			}{URL: u},
		})
	}
	if len(parts) == 0 {
		return json.Marshal("")
	}
	return json.Marshal(parts)
}
