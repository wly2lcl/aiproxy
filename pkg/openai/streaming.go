package openai

import (
	"encoding/json"
	"strings"
)

type StreamChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
	Usage   *Usage         `json:"usage,omitempty"`
}

type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        StreamDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
	Logprobs     interface{} `json:"logprobs,omitempty"`
}

type StreamDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

func ParseStreamLine(line string) (*StreamChunk, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data: ") {
		return nil, nil
	}

	data := strings.TrimPrefix(line, "data: ")
	if data == "[DONE]" {
		return nil, nil
	}

	var chunk StreamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil, err
	}

	return &chunk, nil
}

func IsDoneChunk(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data: ") {
		return false
	}
	data := strings.TrimPrefix(line, "data: ")
	return data == "[DONE]"
}

func ExtractUsageFromStream(chunk *StreamChunk) *Usage {
	if chunk == nil {
		return nil
	}
	return chunk.Usage
}
