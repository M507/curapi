package openai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ResponsesRequest is the subset of POST /v1/responses that Open WebUI sends.
type ResponsesRequest struct {
	Model        string          `json:"model"`
	Input        json.RawMessage `json:"input"`
	Instructions string          `json:"instructions"`
	Stream       bool            `json:"stream"`
}

type ResponsesResult struct {
	ID        string         `json:"id"`
	Object    string         `json:"object"`
	CreatedAt int64          `json:"created_at"`
	Status    string         `json:"status"`
	Error     any            `json:"error"`
	Model     string         `json:"model"`
	Output    []ResponseItem `json:"output"`
	Usage     ResponseUsage  `json:"usage"`
}

type ResponseItem struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	Status  string             `json:"status,omitempty"`
	Role    string             `json:"role,omitempty"`
	Content []ResponseTextPart `json:"content,omitempty"`
}

type ResponseTextPart struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Annotations []any  `json:"annotations"`
}

type ResponseUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type ResponseStreamEvent struct {
	Type         string           `json:"type"`
	Delta        string           `json:"delta,omitempty"`
	Text         string           `json:"text,omitempty"`
	Item         any              `json:"item,omitempty"`
	Part         any              `json:"part,omitempty"`
	Response     *ResponsesResult `json:"response,omitempty"`
	OutputIndex  int              `json:"output_index,omitempty"`
	ContentIndex int              `json:"content_index,omitempty"`
}

func (r ResponsesRequest) ToChatRequest() (ChatRequest, error) {
	msgs := make([]ChatMessage, 0, 4)
	if strings.TrimSpace(r.Instructions) != "" {
		raw, _ := json.Marshal(r.Instructions)
		msgs = append(msgs, ChatMessage{Role: "system", Content: raw})
	}
	if len(r.Input) == 0 || string(r.Input) == "null" {
		if len(msgs) == 0 {
			return ChatRequest{}, fmt.Errorf("input is required")
		}
		return ChatRequest{Model: r.Model, Messages: msgs, Stream: r.Stream}, nil
	}

	var asString string
	if err := json.Unmarshal(r.Input, &asString); err == nil {
		if strings.TrimSpace(asString) != "" {
			raw, _ := json.Marshal(asString)
			msgs = append(msgs, ChatMessage{Role: "user", Content: raw})
		}
		if len(msgs) == 0 {
			return ChatRequest{}, fmt.Errorf("input is required")
		}
		return ChatRequest{Model: r.Model, Messages: msgs, Stream: r.Stream}, nil
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(r.Input, &items); err != nil {
		var one map[string]json.RawMessage
		if err := json.Unmarshal(r.Input, &one); err != nil {
			return ChatRequest{}, fmt.Errorf("invalid input")
		}
		items = []map[string]json.RawMessage{one}
	}
	for _, item := range items {
		role := "user"
		if v, ok := item["role"]; ok {
			_ = json.Unmarshal(v, &role)
		}
		typ := ""
		if v, ok := item["type"]; ok {
			_ = json.Unmarshal(v, &typ)
		}
		switch typ {
		case "function_call", "function_call_output", "reasoning":
			continue
		}
		if role == "developer" {
			role = "system"
		}
		text := extractInputText(item["content"])
		if text == "" {
			text = extractInputText(item["text"])
		}
		if text == "" {
			continue
		}
		raw, _ := json.Marshal(text)
		msgs = append(msgs, ChatMessage{Role: role, Content: raw})
	}
	if len(msgs) == 0 {
		return ChatRequest{}, fmt.Errorf("input is required")
	}
	return ChatRequest{Model: r.Model, Messages: msgs, Stream: r.Stream}, nil
}

func extractInputText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		typ, _ := p["type"].(string)
		switch typ {
		case "input_text", "output_text", "text":
			if t, ok := p["text"].(string); ok {
				b.WriteString(t)
			}
		}
	}
	return b.String()
}

func CreateResponsesResult(requestID, model, text string) ResponsesResult {
	msgID := fmt.Sprintf("msg_%s", requestID)
	return ResponsesResult{
		ID:        fmt.Sprintf("resp_%s", requestID),
		Object:    "response",
		CreatedAt: nowUnix(),
		Status:    "completed",
		Model:     model,
		Output: []ResponseItem{{
			ID:     msgID,
			Type:   "message",
			Status: "completed",
			Role:   "assistant",
			Content: []ResponseTextPart{{
				Type:        "output_text",
				Text:        text,
				Annotations: []any{},
			}},
		}},
		Usage: ResponseUsage{},
	}
}

func ResponsesTextDelta(delta string) ResponseStreamEvent {
	return ResponseStreamEvent{
		Type:         "response.output_text.delta",
		Delta:        delta,
		OutputIndex:  0,
		ContentIndex: 0,
	}
}

func ResponsesCompleted(res ResponsesResult) ResponseStreamEvent {
	return ResponseStreamEvent{
		Type:     "response.completed",
		Response: &res,
	}
}
