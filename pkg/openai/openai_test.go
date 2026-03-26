package openai

import (
	"encoding/json"
	"testing"
)

func TestChatCompletionRequest_JSONMarshal(t *testing.T) {
	temp := 0.7
	maxTokens := 100
	req := ChatCompletionRequest{
		Model:       "gpt-4",
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		Stream: true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["model"] != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got %v", result["model"])
	}
	if result["temperature"] != 0.7 {
		t.Errorf("Expected temperature 0.7, got %v", result["temperature"])
	}
	if result["max_tokens"].(float64) != 100 {
		t.Errorf("Expected max_tokens 100, got %v", result["max_tokens"])
	}
	if result["stream"] != true {
		t.Errorf("Expected stream true, got %v", result["stream"])
	}
}

func TestChatCompletionRequest_JSONUnmarshal(t *testing.T) {
	jsonStr := `{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": "Hello"}],
		"temperature": 0.5,
		"max_tokens": 50,
		"stream": true,
		"stream_options": {"include_usage": true}
	}`

	var req ChatCompletionRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if req.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got %s", req.Model)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "user" {
		t.Errorf("Expected role 'user', got %s", req.Messages[0].Role)
	}
	if req.Temperature == nil || *req.Temperature != 0.5 {
		t.Errorf("Expected temperature 0.5, got %v", req.Temperature)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 50 {
		t.Errorf("Expected max_tokens 50, got %v", req.MaxTokens)
	}
	if req.StreamOptions == nil || req.StreamOptions.IncludeUsage != true {
		t.Errorf("Expected stream_options.include_usage true, got %v", req.StreamOptions)
	}
}

func TestChatCompletionResponse_JSONUnmarshal(t *testing.T) {
	jsonStr := `{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"created": 1677652288,
		"model": "gpt-4",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello there!"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 5,
			"total_tokens": 15
		}
	}`

	var resp ChatCompletionResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if resp.ID != "chatcmpl-123" {
		t.Errorf("Expected ID 'chatcmpl-123', got %s", resp.ID)
	}
	if resp.Object != "chat.completion" {
		t.Errorf("Expected object 'chat.completion', got %s", resp.Object)
	}
	if resp.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got %s", resp.Model)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Hello there!" {
		t.Errorf("Expected content 'Hello there!', got %s", resp.Choices[0].Message.Content)
	}
	if resp.Usage == nil {
		t.Fatal("Expected usage to be non-nil")
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("Expected prompt_tokens 10, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("Expected completion_tokens 5, got %d", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("Expected total_tokens 15, got %d", resp.Usage.TotalTokens)
	}
}

func TestStreamChunk_Parse(t *testing.T) {
	jsonStr := `{
		"id": "chatcmpl-123",
		"object": "chat.completion.chunk",
		"created": 1677652288,
		"model": "gpt-4",
		"choices": [
			{
				"index": 0,
				"delta": {
					"content": "Hello"
				},
				"finish_reason": null
			}
		]
	}`

	var chunk StreamChunk
	if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if chunk.ID != "chatcmpl-123" {
		t.Errorf("Expected ID 'chatcmpl-123', got %s", chunk.ID)
	}
	if chunk.Object != "chat.completion.chunk" {
		t.Errorf("Expected object 'chat.completion.chunk', got %s", chunk.Object)
	}
	if len(chunk.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(chunk.Choices))
	}
	if chunk.Choices[0].Delta.Content != "Hello" {
		t.Errorf("Expected delta content 'Hello', got %s", chunk.Choices[0].Delta.Content)
	}
}

func TestParseStreamLine_ValidData(t *testing.T) {
	line := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`

	chunk, err := ParseStreamLine(line)
	if err != nil {
		t.Fatalf("Failed to parse stream line: %v", err)
	}
	if chunk == nil {
		t.Fatal("Expected non-nil chunk")
	}
	if chunk.ID != "chatcmpl-123" {
		t.Errorf("Expected ID 'chatcmpl-123', got %s", chunk.ID)
	}
	if len(chunk.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(chunk.Choices))
	}
	if chunk.Choices[0].Delta.Content != "Hello" {
		t.Errorf("Expected delta content 'Hello', got %s", chunk.Choices[0].Delta.Content)
	}
}

func TestParseStreamLine_DoneMarker(t *testing.T) {
	line := "data: [DONE]"

	chunk, err := ParseStreamLine(line)
	if err != nil {
		t.Fatalf("Failed to parse stream line: %v", err)
	}
	if chunk != nil {
		t.Errorf("Expected nil chunk for [DONE] marker, got %+v", chunk)
	}
}

func TestParseStreamLine_InvalidPrefix(t *testing.T) {
	line := `{"id":"chatcmpl-123"}`

	chunk, err := ParseStreamLine(line)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if chunk != nil {
		t.Errorf("Expected nil chunk for invalid prefix, got %+v", chunk)
	}
}

func TestIsDoneChunk(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "valid done marker",
			line:     "data: [DONE]",
			expected: true,
		},
		{
			name:     "valid done marker with whitespace",
			line:     "  data: [DONE]  ",
			expected: true,
		},
		{
			name:     "regular data",
			line:     `data: {"id":"chatcmpl-123"}`,
			expected: false,
		},
		{
			name:     "no data prefix",
			line:     "[DONE]",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDoneChunk(tt.line)
			if result != tt.expected {
				t.Errorf("IsDoneChunk(%q) = %v, expected %v", tt.line, result, tt.expected)
			}
		})
	}
}

func TestErrorResponse_Format(t *testing.T) {
	jsonStr := `{
		"error": {
			"message": "Invalid API key",
			"type": "invalid_request_error",
			"param": null,
			"code": "invalid_api_key"
		}
	}`

	var errResp ErrorResponse
	if err := json.Unmarshal([]byte(jsonStr), &errResp); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if errResp.Error.Message != "Invalid API key" {
		t.Errorf("Expected message 'Invalid API key', got %s", errResp.Error.Message)
	}
	if errResp.Error.Type != "invalid_request_error" {
		t.Errorf("Expected type 'invalid_request_error', got %s", errResp.Error.Type)
	}
	if errResp.Error.Code != "invalid_api_key" {
		t.Errorf("Expected code 'invalid_api_key', got %s", errResp.Error.Code)
	}
}

func TestModel_Struct(t *testing.T) {
	jsonStr := `{
		"id": "gpt-4",
		"object": "model",
		"created": 1677652288,
		"owned_by": "openai"
	}`

	var model Model
	if err := json.Unmarshal([]byte(jsonStr), &model); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if model.ID != "gpt-4" {
		t.Errorf("Expected ID 'gpt-4', got %s", model.ID)
	}
	if model.Object != "model" {
		t.Errorf("Expected object 'model', got %s", model.Object)
	}
	if model.OwnedBy != "openai" {
		t.Errorf("Expected owned_by 'openai', got %s", model.OwnedBy)
	}
}

func TestModelList_Struct(t *testing.T) {
	jsonStr := `{
		"object": "list",
		"data": [
			{"id": "gpt-4", "object": "model", "created": 1677652288, "owned_by": "openai"},
			{"id": "gpt-3.5-turbo", "object": "model", "created": 1677652289, "owned_by": "openai"}
		]
	}`

	var modelList ModelList
	if err := json.Unmarshal([]byte(jsonStr), &modelList); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if modelList.Object != "list" {
		t.Errorf("Expected object 'list', got %s", modelList.Object)
	}
	if len(modelList.Data) != 2 {
		t.Fatalf("Expected 2 models, got %d", len(modelList.Data))
	}
	if modelList.Data[0].ID != "gpt-4" {
		t.Errorf("Expected first model ID 'gpt-4', got %s", modelList.Data[0].ID)
	}
	if modelList.Data[1].ID != "gpt-3.5-turbo" {
		t.Errorf("Expected second model ID 'gpt-3.5-turbo', got %s", modelList.Data[1].ID)
	}
}

func TestExtractUsageFromStream(t *testing.T) {
	t.Run("nil chunk", func(t *testing.T) {
		usage := ExtractUsageFromStream(nil)
		if usage != nil {
			t.Errorf("Expected nil usage for nil chunk, got %+v", usage)
		}
	})

	t.Run("chunk with usage", func(t *testing.T) {
		chunk := &StreamChunk{
			Usage: &Usage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}
		usage := ExtractUsageFromStream(chunk)
		if usage == nil {
			t.Fatal("Expected non-nil usage")
		}
		if usage.PromptTokens != 10 {
			t.Errorf("Expected prompt_tokens 10, got %d", usage.PromptTokens)
		}
		if usage.CompletionTokens != 5 {
			t.Errorf("Expected completion_tokens 5, got %d", usage.CompletionTokens)
		}
		if usage.TotalTokens != 15 {
			t.Errorf("Expected total_tokens 15, got %d", usage.TotalTokens)
		}
	})

	t.Run("chunk without usage", func(t *testing.T) {
		chunk := &StreamChunk{
			ID: "test",
		}
		usage := ExtractUsageFromStream(chunk)
		if usage != nil {
			t.Errorf("Expected nil usage for chunk without usage, got %+v", usage)
		}
	})
}

func TestChatMessage_WithToolCalls(t *testing.T) {
	jsonStr := `{
		"role": "assistant",
		"content": null,
		"tool_calls": [
			{
				"id": "call_123",
				"type": "function",
				"function": {
					"name": "get_weather",
					"arguments": "{\"location\": \"Boston\"}"
				}
			}
		]
	}`

	var msg ChatMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if msg.Role != "assistant" {
		t.Errorf("Expected role 'assistant', got %s", msg.Role)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].ID != "call_123" {
		t.Errorf("Expected tool call ID 'call_123', got %s", msg.ToolCalls[0].ID)
	}
	if msg.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got %s", msg.ToolCalls[0].Function.Name)
	}
}

func TestChatMessage_ToolResponse(t *testing.T) {
	jsonStr := `{
		"role": "tool",
		"content": "The weather in Boston is sunny",
		"tool_call_id": "call_123"
	}`

	var msg ChatMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if msg.Role != "tool" {
		t.Errorf("Expected role 'tool', got %s", msg.Role)
	}
	if msg.ToolCallID != "call_123" {
		t.Errorf("Expected tool_call_id 'call_123', got %s", msg.ToolCallID)
	}
}
