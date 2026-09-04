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
	return ChatChunk{
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
}

func CreateChatResponse(requestID, model, text string) ChatResponse {
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
		Usage: Usage{},
	}
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
