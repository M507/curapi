// Package openai converts between OpenAI Chat Completions and Cursor CLI formats.
package openai

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

type ChatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	User        string        `json:"user,omitempty"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason *string     `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
}

type ChunkChoice struct {
	Index        int        `json:"index"`
	Delta        ChunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"`
}

type ChunkDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
	Created int64  `json:"created"`
}

type ModelList struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

type ErrorBody struct {
	Error APIError `json:"error"`
}

func NewError(statusMessage, errType string, code any) ErrorBody {
	return ErrorBody{Error: APIError{Message: statusMessage, Type: errType, Code: code}}
}

func (m ChatMessage) Text() string {
	if len(m.Content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	var parts []ContentPart
	if err := json.Unmarshal(m.Content, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}

func AssistantMessage(text string) ChatMessage {
	raw, _ := json.Marshal(text)
	return ChatMessage{Role: "assistant", Content: raw}
}

func nowUnix() int64 {
	return time.Now().Unix()
}

func chatID(requestID string) string {
	return fmt.Sprintf("chatcmpl-%s", requestID)
}

var stopReason = "stop"
