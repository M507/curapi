// Package cursorcli parses Cursor CLI stream-json messages.
package cursorcli

import "encoding/json"

type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Message struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype,omitempty"`
	Model   string          `json:"model,omitempty"`
	Result  string          `json:"result,omitempty"`
	Message *AssistantBody  `json:"message,omitempty"`
	Raw     json.RawMessage `json:"-"`
}

type AssistantBody struct {
	Content []ContentPart `json:"content"`
}

func ParseLine(line []byte) (Message, error) {
	var msg Message
	if err := json.Unmarshal(line, &msg); err != nil {
		return Message{}, err
	}
	msg.Raw = append([]byte(nil), line...)
	return msg, nil
}

func (m Message) IsSystemInit() bool {
	return m.Type == "system" && m.Subtype == "init"
}

func (m Message) IsAssistant() bool {
	return m.Type == "assistant"
}

func (m Message) IsToolCall() bool {
	return m.Type == "tool_call"
}

func (m Message) IsResult() bool {
	return m.Type == "result"
}

func (m Message) AssistantText() string {
	if m.Message == nil {
		return ""
	}
	out := ""
	for _, part := range m.Message.Content {
		if part.Type == "text" {
			out += part.Text
		}
	}
	return out
}
