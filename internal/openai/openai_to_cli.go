package openai

import "strings"

var KnownCursorModels = []string{
	"auto",
	"composer-2.5",
	"composer-2.5-fast",
	"composer-1.5",
	"composer-1",
	"opus-4.6-thinking",
	"opus-4.6",
	"opus-4.5-thinking",
	"opus-4.5",
	"sonnet-4.5-thinking",
	"sonnet-4.5",
	"gpt-5.3-codex",
	"gpt-5.3-codex-fast",
	"gpt-5.3-codex-low",
	"gpt-5.3-codex-low-fast",
	"gpt-5.3-codex-high",
	"gpt-5.3-codex-high-fast",
	"gpt-5.3-codex-xhigh",
	"gpt-5.3-codex-xhigh-fast",
	"gpt-5.3-codex-spark-preview",
	"gpt-5.2",
	"gpt-5.2-codex",
	"gpt-5.2-codex-low",
	"gpt-5.2-codex-low-fast",
	"gpt-5.2-codex-high",
	"gpt-5.2-codex-xhigh",
	"gpt-5.2-codex-fast",
	"gpt-5.2-codex-high-fast",
	"gpt-5.2-codex-xhigh-fast",
	"gpt-5.1-codex-max",
	"gpt-5.1-codex-max-high",
	"gpt-5.2-high",
	"gpt-5.1-high",
	"gemini-3-pro",
	"gemini-3-flash",
	"grok",
}

var modelAliases = map[string]string{
	"composer-1":   "composer-2.5",
	"composer-1.5": "composer-2.5",
	"grok":         "cursor-grok-4.6-high-fast",
}

var knownCursorModelSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(KnownCursorModels))
	for _, id := range KnownCursorModels {
		m[id] = struct{}{}
	}
	return m
}()

type CLIInput struct {
	Prompt string
	Model  string
}

// ExtractModel resolves an OpenAI-style model string to a Cursor CLI model.
//
//	"cursor/opus-4.6"   -> "opus-4.6"
//	"cursor-opus-4.6"   -> "opus-4.6"
//	"auto"              -> "auto"
//	"opus-4.6-thinking" -> "opus-4.6-thinking"
func ExtractModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "auto"
	}
	if alias, ok := modelAliases[model]; ok {
		return alias
	}
	if strings.HasPrefix(model, "cursor/") {
		rest := model[len("cursor/"):]
		if rest == "" {
			return "auto"
		}
		if alias, ok := modelAliases[rest]; ok {
			return alias
		}
		return rest
	}
	if strings.HasPrefix(model, "cursor-") {
		rest := model[len("cursor-"):]
		if rest != "" {
			return rest
		}
	}
	if _, ok := knownCursorModelSet[model]; ok {
		return model
	}
	return "auto"
}

// MessagesToPrompt flattens OpenAI messages into a single CLI prompt.
func MessagesToPrompt(messages []ChatMessage) string {
	nonEmpty := make([]ChatMessage, 0, len(messages))
	for _, m := range messages {
		if m.Text() != "" {
			nonEmpty = append(nonEmpty, m)
		}
	}
	if len(nonEmpty) == 1 && nonEmpty[0].Role == "user" {
		return nonEmpty[0].Text()
	}

	parts := make([]string, 0, len(nonEmpty))
	for _, msg := range nonEmpty {
		text := msg.Text()
		switch msg.Role {
		case "system":
			parts = append(parts, "[System]\n"+text)
		case "user":
			parts = append(parts, "[User]\n"+text)
		case "assistant":
			parts = append(parts, "[Assistant]\n"+text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func OpenAIToCLI(req ChatRequest) CLIInput {
	model := req.Model
	if model == "" {
		model = "auto"
	}
	return CLIInput{
		Prompt: MessagesToPrompt(req.Messages),
		Model:  ExtractModel(model),
	}
}
