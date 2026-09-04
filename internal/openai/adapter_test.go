package openai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractModel(t *testing.T) {
	cases := map[string]string{
		"cursor/opus-4.6":    "opus-4.6",
		"cursor/":            "auto",
		"cursor-opus-4.6":    "opus-4.6",
		"cursor-unknown-foo": "unknown-foo",
		"auto":               "auto",
		"opus-4.6-thinking":  "opus-4.6-thinking",
		"not-a-model":        "auto",
		"":                   "auto",
		"  gpt-5.2  ":        "gpt-5.2",
		"composer-1":         "composer-2.5",
		"grok":               "cursor-grok-4.6-high-fast",
		"cursor/composer-1":  "composer-2.5",
	}
	for in, want := range cases {
		if got := ExtractModel(in); got != want {
			t.Errorf("ExtractModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMessagesToPromptSingleUser(t *testing.T) {
	msg := mustMessage("user", "Hello!")
	got := MessagesToPrompt([]ChatMessage{msg})
	if got != "Hello!" {
		t.Fatalf("got %q", got)
	}
}

func TestMessagesToPromptMultiTurn(t *testing.T) {
	msgs := []ChatMessage{
		mustMessage("system", "Be brief"),
		mustMessage("user", "Hi"),
		mustMessage("assistant", "Hello"),
		mustMessage("user", ""),
	}
	got := MessagesToPrompt(msgs)
	want := "[System]\nBe brief\n\n[User]\nHi\n\n[Assistant]\nHello"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestMessagesToPromptArrayContent(t *testing.T) {
	raw, _ := json.Marshal([]ContentPart{
		{Type: "text", Text: "A"},
		{Type: "image_url"},
		{Type: "text", Text: "B"},
	})
	msg := ChatMessage{Role: "user", Content: raw}
	if got := MessagesToPrompt([]ChatMessage{msg}); got != "AB" {
		t.Fatalf("got %q", got)
	}
}

func TestOpenAIToCLI(t *testing.T) {
	req := ChatRequest{
		Model:    "cursor/sonnet-4.5",
		Messages: []ChatMessage{mustMessage("user", "ping")},
	}
	got := OpenAIToCLI(req)
	if got.Model != "sonnet-4.5" || got.Prompt != "ping" {
		t.Fatalf("%+v", got)
	}
}

func TestCreateStreamAndChat(t *testing.T) {
	first := CreateStreamChunk("abc", "auto", "He", true)
	if first.Choices[0].Delta.Role != "assistant" || first.ID != "chatcmpl-abc" {
		t.Fatalf("%+v", first)
	}
	next := CreateStreamChunk("abc", "auto", "llo", false)
	if next.Choices[0].Delta.Role != "" {
		t.Fatal("subsequent chunk should omit role")
	}
	done := CreateDoneChunk("abc", "auto")
	if done.Choices[0].FinishReason == nil || *done.Choices[0].FinishReason != "stop" {
		t.Fatalf("%+v", done)
	}
	resp := CreateChatResponse("abc", "auto", "Hello")
	if resp.Choices[0].Message.Text() != "Hello" {
		t.Fatalf("%+v", resp)
	}
}

func TestCreateModelList(t *testing.T) {
	list := CreateModelList()
	if list.Object != "list" || len(list.Data) == 0 {
		t.Fatalf("%+v", list)
	}
	found := false
	for _, m := range list.Data {
		if m.ID == "auto" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing auto model")
	}
}

func TestParseListModels(t *testing.T) {
	ids := ParseListModels("Available models\n\nauto - Auto (default)\ncomposer-2.5 - Composer 2.5\nnot a model line\n")
	if len(ids) != 2 || ids[0] != "auto" || ids[1] != "composer-2.5" {
		t.Fatalf("%v", ids)
	}
}

func TestResponsesInputString(t *testing.T) {
	raw := json.RawMessage(`"hello"`)
	req := ResponsesRequest{Model: "composer-1", Input: raw, Instructions: "Be brief"}
	chat, err := req.ToChatRequest()
	if err != nil {
		t.Fatal(err)
	}
	if chat.Model != "composer-1" {
		t.Fatalf("model %s", chat.Model)
	}
	prompt := MessagesToPrompt(chat.Messages)
	if !strings.Contains(prompt, "hello") || !strings.Contains(prompt, "Be brief") {
		t.Fatalf("prompt %q", prompt)
	}
}

func TestResponsesInputMessages(t *testing.T) {
	raw := json.RawMessage(`[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"Hi"}]}
	]`)
	req := ResponsesRequest{Model: "auto", Input: raw}
	chat, err := req.ToChatRequest()
	if err != nil {
		t.Fatal(err)
	}
	if MessagesToPrompt(chat.Messages) != "Hi" {
		t.Fatalf("%q", MessagesToPrompt(chat.Messages))
	}
}

func TestCreateResponsesResult(t *testing.T) {
	res := CreateResponsesResult("abc", "composer-1", "Hello")
	if res.Object != "response" || res.Status != "completed" {
		t.Fatalf("%+v", res)
	}
	if res.Output[0].Content[0].Text != "Hello" {
		t.Fatalf("%+v", res)
	}
}

func mustMessage(role, text string) ChatMessage {
	raw, _ := json.Marshal(text)
	return ChatMessage{Role: role, Content: raw}
}
