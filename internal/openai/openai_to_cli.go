package openai

import "strings"

var KnownCursorModels = []string{
	"auto",
	"gpt-5.3-codex-low",
	"gpt-5.3-codex-low-fast",
	"gpt-5.3-codex",
	"gpt-5.3-codex-fast",
	"gpt-5.3-codex-high",
	"gpt-5.3-codex-high-fast",
	"gpt-5.3-codex-xhigh",
	"gpt-5.3-codex-xhigh-fast",
	"gpt-5.2",
	"cursor-grok-4.6-high-fast",
	"composer-2.5",
	"claude-opus-5-thinking-high",
	"claude-opus-5-thinking-high-fast",
	"gpt-5.6-sol-high",
	"gpt-5.6-sol-high-fast",
	"gpt-5.6-sol-xhigh",
	"gpt-5.6-sol-xhigh-fast",
	"claude-fable-5-thinking-high",
	"claude-fable-5-thinking-xhigh",
	"gemini-3.7-flash-high",
	"claude-sonnet-5-thinking-high",
	"claude-sonnet-5-thinking-xhigh",
	"gpt-5.6-luna-high",
	"cursor-grok-4.6-low",
	"cursor-grok-4.6-low-fast",
	"cursor-grok-4.6-medium",
	"cursor-grok-4.6-medium-fast",
	"cursor-grok-4.6-high",
	"cursor-grok-4.6-xhigh",
	"cursor-grok-4.6-xhigh-fast",
	"composer-2.5-fast",
	"claude-opus-5-low",
	"claude-opus-5-low-fast",
	"claude-opus-5-medium",
	"claude-opus-5-medium-fast",
	"claude-opus-5-high",
	"claude-opus-5-high-fast",
	"claude-opus-5-thinking-low",
	"claude-opus-5-thinking-low-fast",
	"claude-opus-5-thinking-medium",
	"claude-opus-5-thinking-medium-fast",
	"claude-opus-5-thinking-xhigh",
	"claude-opus-5-thinking-xhigh-fast",
	"claude-opus-5-thinking-max",
	"claude-opus-5-thinking-max-fast",
	"claude-opus-4-8-low",
	"claude-opus-4-8-low-fast",
	"claude-opus-4-8-medium",
	"claude-opus-4-8-medium-fast",
	"claude-opus-4-8-high",
	"claude-opus-4-8-high-fast",
	"claude-opus-4-8-xhigh",
	"claude-opus-4-8-xhigh-fast",
	"claude-opus-4-8-max",
	"claude-opus-4-8-max-fast",
	"claude-opus-4-8-thinking-low",
	"claude-opus-4-8-thinking-low-fast",
	"claude-opus-4-8-thinking-medium",
	"claude-opus-4-8-thinking-medium-fast",
	"claude-opus-4-8-thinking-high",
	"claude-opus-4-8-thinking-high-fast",
	"claude-opus-4-8-thinking-xhigh",
	"claude-opus-4-8-thinking-xhigh-fast",
	"claude-opus-4-8-thinking-max",
	"claude-opus-4-8-thinking-max-fast",
	"gpt-5.6-sol-none",
	"gpt-5.6-sol-none-fast",
	"gpt-5.6-sol-low",
	"gpt-5.6-sol-low-fast",
	"gpt-5.6-sol-medium",
	"gpt-5.6-sol-medium-fast",
	"gpt-5.6-sol-max",
	"gpt-5.6-sol-max-fast",
	"gpt-5.5-none",
	"gpt-5.5-none-fast",
	"gpt-5.5-low",
	"gpt-5.5-low-fast",
	"gpt-5.5-medium",
	"gpt-5.5-medium-fast",
	"gpt-5.5-high",
	"gpt-5.5-high-fast",
	"gpt-5.5-extra-high",
	"gpt-5.5-extra-high-fast",
	"claude-fable-5-1-low",
	"claude-fable-5-1-medium",
	"claude-fable-5-1-high",
	"claude-fable-5-1-xhigh",
	"claude-fable-5-1-max",
	"claude-fable-5-1-thinking-low",
	"claude-fable-5-1-thinking-medium",
	"claude-fable-5-1-thinking-high",
	"claude-fable-5-1-thinking-xhigh",
	"claude-fable-5-1-thinking-max",
	"claude-fable-5-low",
	"claude-fable-5-medium",
	"claude-fable-5-high",
	"claude-fable-5-xhigh",
	"claude-fable-5-max",
	"claude-fable-5-thinking-low",
	"claude-fable-5-thinking-medium",
	"claude-fable-5-thinking-max",
	"gemini-3.8-flash-low",
	"gemini-3.8-flash-medium",
	"gemini-3.8-flash-high",
	"gemini-3.7-flash-low",
	"gemini-3.7-flash-medium",
	"gpt-5.6-terra-none",
	"gpt-5.6-terra-none-fast",
	"gpt-5.6-terra-low",
	"gpt-5.6-terra-low-fast",
	"gpt-5.6-terra-medium",
	"gpt-5.6-terra-medium-fast",
	"gpt-5.6-terra-high",
	"gpt-5.6-terra-high-fast",
	"gpt-5.6-terra-xhigh",
	"gpt-5.6-terra-xhigh-fast",
	"gpt-5.6-terra-max",
	"gpt-5.6-terra-max-fast",
	"claude-sonnet-5-low",
	"claude-sonnet-5-medium",
	"claude-sonnet-5-high",
	"claude-sonnet-5-xhigh",
	"claude-sonnet-5-max",
	"claude-sonnet-5-thinking-low",
	"claude-sonnet-5-thinking-medium",
	"claude-sonnet-5-thinking-max",
	"claude-4.6-sonnet-medium",
	"claude-4.6-sonnet-medium-thinking",
	"claude-opus-4-7-low",
	"claude-opus-4-7-low-fast",
	"claude-opus-4-7-medium",
	"claude-opus-4-7-medium-fast",
	"claude-opus-4-7-high",
	"claude-opus-4-7-high-fast",
	"claude-opus-4-7-xhigh",
	"claude-opus-4-7-xhigh-fast",
	"claude-opus-4-7-max",
	"claude-opus-4-7-max-fast",
	"claude-opus-4-7-thinking-low",
	"claude-opus-4-7-thinking-low-fast",
	"claude-opus-4-7-thinking-medium",
	"claude-opus-4-7-thinking-medium-fast",
	"claude-opus-4-7-thinking-high",
	"claude-opus-4-7-thinking-high-fast",
	"claude-opus-4-7-thinking-xhigh",
	"claude-opus-4-7-thinking-xhigh-fast",
	"claude-opus-4-7-thinking-max",
	"claude-opus-4-7-thinking-max-fast",
	"gpt-5.4-low",
	"gpt-5.4-medium",
	"gpt-5.4-medium-fast",
	"gpt-5.4-high",
	"gpt-5.4-high-fast",
	"gpt-5.4-xhigh",
	"gpt-5.4-xhigh-fast",
	"claude-4.6-opus-high",
	"claude-4.6-opus-max",
	"claude-4.6-opus-high-thinking",
	"claude-4.6-opus-max-thinking",
	"claude-4.5-opus-high",
	"claude-4.5-opus-high-thinking",
	"gpt-5.2-low",
	"gpt-5.2-low-fast",
	"gpt-5.2-fast",
	"gpt-5.2-high",
	"gpt-5.2-high-fast",
	"gpt-5.2-xhigh",
	"gpt-5.2-xhigh-fast",
	"gpt-5.6-luna-none",
	"gpt-5.6-luna-none-fast",
	"gpt-5.6-luna-low",
	"gpt-5.6-luna-low-fast",
	"gpt-5.6-luna-medium",
	"gpt-5.6-luna-medium-fast",
	"gpt-5.6-luna-high-fast",
	"gpt-5.6-luna-xhigh",
	"gpt-5.6-luna-xhigh-fast",
	"gpt-5.6-luna-max",
	"gpt-5.6-luna-max-fast",
	"gemini-3.6-flash-minimal",
	"gemini-3.6-flash-low",
	"gemini-3.6-flash-medium",
	"gemini-3.6-flash-high",
	"gemini-3.1-pro",
	"gpt-5.4-mini-none",
	"gpt-5.4-mini-low",
	"gpt-5.4-mini-medium",
	"gpt-5.4-mini-high",
	"gpt-5.4-mini-xhigh",
	"gpt-5.4-nano-none",
	"gpt-5.4-nano-low",
	"gpt-5.4-nano-medium",
	"gpt-5.4-nano-high",
	"gpt-5.4-nano-xhigh",
	"claude-4.5-sonnet",
	"claude-4.5-sonnet-thinking",
	"gpt-5.1-low",
	"gpt-5.1",
	"gpt-5.1-high",
	"gemini-3-flash",
	"gemini-3.5-flash",
	"claude-4-sonnet",
	"claude-4-sonnet-thinking",
	"gpt-5-mini",
	"kimi-k3-low",
	"kimi-k3-high",
	"kimi-k3-max",
	"kimi-k2.7-code",
	"glm-5.2-high",
	"glm-5.2-max",
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

func MergeModelIDs(ids []string) []string {
	if len(ids) == 0 {
		ids = append([]string{}, KnownCursorModels...)
	}
	seen := make(map[string]struct{}, len(ids)+len(modelAliases))
	out := make([]string, 0, len(ids)+len(modelAliases))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for alias := range modelAliases {
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		out = append(out, alias)
	}
	return out
}

type CLIInput struct {
	Prompt string
	Model  string
}

// ExtractModel resolves an OpenAI-style model string to a Cursor CLI model.
//
//	"cursor/opus-4.6"   -> "opus-4.6"
//	"cursor-opus-4.6"   -> "opus-4.6"
//	"auto"              -> "auto"
//	"claude-opus-5-low" -> "claude-opus-5-low" (passed through)
func ExtractModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "auto"
	}
	if alias, ok := modelAliases[model]; ok {
		return alias
	}
	if strings.HasPrefix(model, "cursor/") {
		rest := strings.TrimSpace(model[len("cursor/"):])
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
			if alias, ok := modelAliases[rest]; ok {
				return alias
			}
			if _, ok := knownCursorModelSet[rest]; ok {
				return rest
			}
		}
	}
	return model
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
