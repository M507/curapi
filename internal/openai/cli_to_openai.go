package openai

import (
	"strings"
)

func CreateStreamChunk(requestID, model, text string, isFirst bool) ChatChunk {
	delta := ChunkDelta{Content: text}
	if isFirst {
		delta.Role = "assistant"
	}
	return ChatChunk{
		ID:      chatID(requestID),
		Object:  "chat.completion.chunk",
		Created: nowUnix(),
		Model:   model,
		Choices: []ChunkChoice{{
			Index:        0,
			Delta:        delta,
			FinishReason: nil,
		}},
	}
}

func CreateDoneChunk(requestID, model string) ChatChunk {
	return CreateDoneChunkWithUsage(requestID, model, Usage{})
}

func CreateDoneChunkWithUsage(requestID, model string, usage Usage) ChatChunk {
	chunk := ChatChunk{
		ID:      chatID(requestID),
		Object:  "chat.completion.chunk",
		Created: nowUnix(),
		Model:   model,
		Choices: []ChunkChoice{{
			Index:        0,
			Delta:        ChunkDelta{},
			FinishReason: &stopReason,
		}},
	}
	if usage.TotalTokens > 0 || usage.PromptTokens > 0 || usage.CompletionTokens > 0 {
		chunk.Usage = &usage
	}
	return chunk
}

func CreateChatResponse(requestID, model, text string) ChatResponse {
	return CreateChatResponseWithUsage(requestID, model, text, Usage{})
}

func CreateChatResponseWithUsage(requestID, model, text string, usage Usage) ChatResponse {
	msg := AssistantMessage(text)
	return ChatResponse{
		ID:      chatID(requestID),
		Object:  "chat.completion",
		Created: nowUnix(),
		Model:   model,
		Choices: []Choice{{
			Index:        0,
			Message:      msg,
			FinishReason: &stopReason,
		}},
		Usage: usage,
	}
}

// ChatUsageFromAgent maps Cursor CLI token counts into Chat Completions usage.
func ChatUsageFromAgent(inputTokens, outputTokens, cacheReadTokens int) Usage {
	u := Usage{
		PromptTokens:     inputTokens,
		CompletionTokens: outputTokens,
		TotalTokens:      inputTokens + outputTokens,
	}
	if cacheReadTokens > 0 {
		u.PromptTokensDetails = &PromptTokensDetails{CachedTokens: cacheReadTokens}
	}
	return u
}

func CreateModelList() ModelList {
	return CreateModelListFrom(KnownCursorModels)
}

func CreateModelListFrom(ids []string) ModelList {
	ids = MergeModelIDs(ids)
	now := nowUnix()
	data := make([]Model, 0, len(ids))
	for _, id := range ids {
		data = append(data, Model{
			ID:      id,
			Object:  "model",
			OwnedBy: "cursor",
			Created: now,
		})
	}
	return ModelList{Object: "list", Data: data}
}

func ParseListModels(output string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 32)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		id, rest, ok := strings.Cut(line, " - ")
		if !ok || strings.TrimSpace(rest) == "" {
			continue
		}
		id = strings.TrimSpace(id)
		if id == "" || strings.ContainsAny(id, " \t") {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
