package provider

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wangluyao/aiproxy/pkg/openai"
)

func TestProvider_Interface(t *testing.T) {
	var _ Provider = (*OpenAIProvider)(nil)
	var _ Provider = (*OpenRouterProvider)(nil)
	var _ Provider = (*GroqProvider)(nil)
}

func TestOpenAI_TransformRequest(t *testing.T) {
	provider := NewOpenAIProvider("test-api-key", nil, []string{"gpt-4", "gpt-3.5-turbo"})

	req := &openai.ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []openai.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	httpReq, err := provider.TransformRequest(req, "test-api-key")
	if err != nil {
		t.Fatalf("TransformRequest failed: %v", err)
	}

	if httpReq.Method != http.MethodPost {
		t.Errorf("Expected method POST, got %s", httpReq.Method)
	}

	expectedURL := "https://api.openai.com/v1/chat/completions"
	if !strings.Contains(httpReq.URL.String(), expectedURL) {
		t.Errorf("Expected URL to contain %s, got %s", expectedURL, httpReq.URL.String())
	}

	auth := httpReq.Header.Get("Authorization")
	if auth != "Bearer test-api-key" {
		t.Errorf("Expected Authorization header 'Bearer test-api-key', got %s", auth)
	}

	contentType := httpReq.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got %s", contentType)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read body: %v", err)
	}

	var decodedReq openai.ChatCompletionRequest
	if err := json.Unmarshal(body, &decodedReq); err != nil {
		t.Fatalf("Failed to decode body: %v", err)
	}

	if decodedReq.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got %s", decodedReq.Model)
	}
}

func TestOpenAI_SupportsModel(t *testing.T) {
	provider := NewOpenAIProvider("test-api-key", nil, []string{"gpt-4", "gpt-3.5-turbo"})

	tests := []struct {
		model    string
		expected bool
	}{
		{"gpt-4", true},
		{"gpt-3.5-turbo", true},
		{"gpt-4-turbo", false},
		{"claude-3", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := provider.SupportsModel(tt.model)
			if result != tt.expected {
				t.Errorf("SupportsModel(%s) = %v, expected %v", tt.model, result, tt.expected)
			}
		})
	}
}

func TestOpenAI_SupportsModel_EmptyList(t *testing.T) {
	provider := NewOpenAIProvider("test-api-key", nil, []string{})

	if !provider.SupportsModel("any-model") {
		t.Error("Expected empty model list to support all models")
	}
}

func TestOpenAI_GetTimeout(t *testing.T) {
	provider := NewOpenAIProvider("test-api-key", nil, nil)

	normalTimeout := provider.GetTimeout(false)
	streamTimeout := provider.GetTimeout(true)

	if normalTimeout != DefaultTimeout {
		t.Errorf("Expected normal timeout %v, got %v", DefaultTimeout, normalTimeout)
	}

	if streamTimeout != DefaultStreamTimeout {
		t.Errorf("Expected stream timeout %v, got %v", DefaultStreamTimeout, streamTimeout)
	}
}

func TestOpenAI_TransformResponse(t *testing.T) {
	provider := NewOpenAIProvider("test-api-key", nil, nil)

	resp := &openai.ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []openai.ChatCompletionChoice{
			{
				Index: 0,
				Message: openai.ChatMessage{
					Role:    "assistant",
					Content: "Hello!",
				},
				FinishReason: "stop",
			},
		},
	}

	respBody, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	httpResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	result, err := provider.TransformResponse(httpResp)
	if err != nil {
		t.Fatalf("TransformResponse failed: %v", err)
	}

	if result.ID != "chatcmpl-123" {
		t.Errorf("Expected ID 'chatcmpl-123', got %s", result.ID)
	}

	if result.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got %s", result.Model)
	}

	if len(result.Choices) != 1 {
		t.Errorf("Expected 1 choice, got %d", len(result.Choices))
	}
}

func TestOpenAI_TransformResponse_Error(t *testing.T) {
	provider := NewOpenAIProvider("test-api-key", nil, nil)

	httpResp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(bytes.NewReader([]byte("{}"))),
	}

	_, err := provider.TransformResponse(httpResp)
	if err == nil {
		t.Error("Expected error for non-2xx status code")
	}
}

func TestOpenAI_NilRequest(t *testing.T) {
	provider := NewOpenAIProvider("test-api-key", nil, nil)

	_, err := provider.TransformRequest(nil, "test-api-key")
	if err == nil {
		t.Error("Expected error for nil request")
	}
}

func TestOpenAI_NilResponse(t *testing.T) {
	provider := NewOpenAIProvider("test-api-key", nil, nil)

	_, err := provider.TransformResponse(nil)
	if err == nil {
		t.Error("Expected error for nil response")
	}
}

func TestOpenRouter_TransformRequest(t *testing.T) {
	extraHeaders := map[string]string{
		"HTTP-Referer": "https://example.com",
		"X-Title":      "TestApp",
	}
	provider := NewOpenRouterProvider("test-api-key", nil, nil, extraHeaders)

	req := &openai.ChatCompletionRequest{
		Model: "openai/gpt-4o-mini",
		Messages: []openai.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	httpReq, err := provider.TransformRequest(req, "test-api-key")
	if err != nil {
		t.Fatalf("TransformRequest failed: %v", err)
	}

	if httpReq.Method != http.MethodPost {
		t.Errorf("Expected method POST, got %s", httpReq.Method)
	}

	expectedURL := "https://openrouter.ai/api/v1/chat/completions"
	if !strings.Contains(httpReq.URL.String(), expectedURL) {
		t.Errorf("Expected URL to contain %s, got %s", expectedURL, httpReq.URL.String())
	}

	auth := httpReq.Header.Get("Authorization")
	if auth != "Bearer test-api-key" {
		t.Errorf("Expected Authorization header 'Bearer test-api-key', got %s", auth)
	}

	referer := httpReq.Header.Get("HTTP-Referer")
	if referer != "https://example.com" {
		t.Errorf("Expected HTTP-Referer header 'https://example.com', got %s", referer)
	}

	title := httpReq.Header.Get("X-Title")
	if title != "TestApp" {
		t.Errorf("Expected X-Title header 'TestApp', got %s", title)
	}
}

func TestOpenRouter_Headers(t *testing.T) {
	tests := []struct {
		name         string
		extraHeaders map[string]string
		checkHeader  string
		expected     string
	}{
		{
			name:         "with referer",
			extraHeaders: map[string]string{"HTTP-Referer": "https://test.com"},
			checkHeader:  "HTTP-Referer",
			expected:     "https://test.com",
		},
		{
			name:         "with title",
			extraHeaders: map[string]string{"X-Title": "MyApp"},
			checkHeader:  "X-Title",
			expected:     "MyApp",
		},
		{
			name:         "without extra headers",
			extraHeaders: nil,
			checkHeader:  "HTTP-Referer",
			expected:     "",
		},
		{
			name:         "empty extra headers",
			extraHeaders: map[string]string{},
			checkHeader:  "HTTP-Referer",
			expected:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewOpenRouterProvider("test-key", nil, nil, tt.extraHeaders)
			headers := provider.GetHeaders("test-key")

			if headers[tt.checkHeader] != tt.expected {
				t.Errorf("Expected %s header '%s', got '%s'", tt.checkHeader, tt.expected, headers[tt.checkHeader])
			}
		})
	}
}

func TestOpenRouter_SupportsModel(t *testing.T) {
	provider := NewOpenRouterProvider("test-key", nil, []string{"openai/*", "anthropic/claude-3"}, nil)

	tests := []struct {
		model    string
		expected bool
	}{
		{"openai/gpt-4", true},
		{"openai/gpt-3.5-turbo", true},
		{"anthropic/claude-3", true},
		{"anthropic/claude-2", false},
		{"google/gemini", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := provider.SupportsModel(tt.model)
			if result != tt.expected {
				t.Errorf("SupportsModel(%s) = %v, expected %v", tt.model, result, tt.expected)
			}
		})
	}
}

func TestOpenRouter_APIBase(t *testing.T) {
	provider := NewOpenRouterProvider("test-key", nil, nil, nil)

	if provider.APIBase() != OpenRouterBaseURL {
		t.Errorf("Expected API base %s, got %s", OpenRouterBaseURL, provider.APIBase())
	}

	if provider.Name() != "openrouter" {
		t.Errorf("Expected name 'openrouter', got %s", provider.Name())
	}
}

func TestGroq_TransformRequest(t *testing.T) {
	provider := NewGroqProvider("test-api-key", nil, []string{"llama-3-70b", "mixtral-8x7b"})

	req := &openai.ChatCompletionRequest{
		Model: "llama-3-70b",
		Messages: []openai.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	httpReq, err := provider.TransformRequest(req, "test-api-key")
	if err != nil {
		t.Fatalf("TransformRequest failed: %v", err)
	}

	if httpReq.Method != http.MethodPost {
		t.Errorf("Expected method POST, got %s", httpReq.Method)
	}

	expectedURL := "https://api.groq.com/openai/v1/chat/completions"
	if !strings.Contains(httpReq.URL.String(), expectedURL) {
		t.Errorf("Expected URL to contain %s, got %s", expectedURL, httpReq.URL.String())
	}

	auth := httpReq.Header.Get("Authorization")
	if auth != "Bearer test-api-key" {
		t.Errorf("Expected Authorization header 'Bearer test-api-key', got %s", auth)
	}
}

func TestGroq_SupportsModel(t *testing.T) {
	provider := NewGroqProvider("test-key", nil, []string{"llama-3-70b", "mixtral-8x7b"})

	tests := []struct {
		model    string
		expected bool
	}{
		{"llama-3-70b", true},
		{"mixtral-8x7b", true},
		{"gpt-4", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := provider.SupportsModel(tt.model)
			if result != tt.expected {
				t.Errorf("SupportsModel(%s) = %v, expected %v", tt.model, result, tt.expected)
			}
		})
	}
}

func TestGroq_APIBase(t *testing.T) {
	provider := NewGroqProvider("test-key", nil, nil)

	if provider.APIBase() != GroqBaseURL {
		t.Errorf("Expected API base %s, got %s", GroqBaseURL, provider.APIBase())
	}

	if provider.Name() != "groq" {
		t.Errorf("Expected name 'groq', got %s", provider.Name())
	}
}

func TestGroq_GetHeaders(t *testing.T) {
	provider := NewGroqProvider("test-key", nil, nil)
	headers := provider.GetHeaders("my-api-key")

	if headers["Authorization"] != "Bearer my-api-key" {
		t.Errorf("Expected Authorization header 'Bearer my-api-key', got %s", headers["Authorization"])
	}

	if headers["Content-Type"] != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got %s", headers["Content-Type"])
	}
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	provider := NewOpenAIProvider("test-key", nil, nil)
	err := registry.Register(provider)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if len(registry.List()) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(registry.List()))
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	registry := NewRegistry()

	provider1 := NewOpenAIProvider("test-key-1", nil, nil)
	provider2 := NewOpenAIProvider("test-key-2", nil, nil)

	_ = registry.Register(provider1)
	err := registry.Register(provider2)

	if err != ErrProviderExists {
		t.Errorf("Expected ErrProviderExists, got %v", err)
	}
}

func TestRegistry_Register_Nil(t *testing.T) {
	registry := NewRegistry()

	err := registry.Register(nil)
	if err == nil {
		t.Error("Expected error for nil provider")
	}
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()
	provider := NewOpenAIProvider("test-key", nil, nil)
	_ = registry.Register(provider)

	got, err := registry.Get("openai")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Name() != "openai" {
		t.Errorf("Expected name 'openai', got %s", got.Name())
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	registry := NewRegistry()

	_, err := registry.Get("nonexistent")
	if err != ErrProviderNotFound {
		t.Errorf("Expected ErrProviderNotFound, got %v", err)
	}
}

func TestRegistry_GetByModel(t *testing.T) {
	registry := NewRegistry()

	openai := NewOpenAIProvider("test-key", nil, []string{"gpt-4", "gpt-3.5-turbo"})
	groq := NewGroqProvider("test-key", nil, []string{"llama-3-70b"})

	_ = registry.Register(openai)
	_ = registry.Register(groq)

	provider, err := registry.GetByModel("gpt-4")
	if err != nil {
		t.Fatalf("GetByModel failed: %v", err)
	}

	if provider.Name() != "openai" {
		t.Errorf("Expected provider 'openai', got %s", provider.Name())
	}
}

func TestRegistry_GetByModel_NoMatch(t *testing.T) {
	registry := NewRegistry()

	openai := NewOpenAIProvider("test-key", nil, []string{"gpt-4"})
	_ = registry.Register(openai)

	_, err := registry.GetByModel("claude-3")
	if err != ErrNoMatchingProvider {
		t.Errorf("Expected ErrNoMatchingProvider, got %v", err)
	}
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()

	openai := NewOpenAIProvider("test-key", nil, nil)
	groq := NewGroqProvider("test-key", nil, nil)

	_ = registry.Register(openai)
	_ = registry.Register(groq)

	providers := registry.List()
	if len(providers) != 2 {
		t.Errorf("Expected 2 providers, got %d", len(providers))
	}
}

func TestRegistry_Default(t *testing.T) {
	registry := NewRegistry()

	openai := NewOpenAIProvider("test-key", nil, nil)
	groq := NewGroqProvider("test-key", nil, nil)

	_ = registry.Register(openai)
	_ = registry.Register(groq)

	defaultProvider := registry.GetDefault()
	if defaultProvider == nil {
		t.Error("Expected default provider, got nil")
	}

	if defaultProvider.Name() != "openai" {
		t.Errorf("Expected default provider 'openai', got %s", defaultProvider.Name())
	}

	err := registry.SetDefault("groq")
	if err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}

	newDefault := registry.GetDefault()
	if newDefault.Name() != "groq" {
		t.Errorf("Expected default provider 'groq', got %s", newDefault.Name())
	}
}

func TestRegistry_SetDefault_NotFound(t *testing.T) {
	registry := NewRegistry()

	err := registry.SetDefault("nonexistent")
	if err != ErrProviderNotFound {
		t.Errorf("Expected ErrProviderNotFound, got %v", err)
	}
}

func TestRegistry_Remove(t *testing.T) {
	registry := NewRegistry()

	openai := NewOpenAIProvider("test-key", nil, nil)
	groq := NewGroqProvider("test-key", nil, nil)

	_ = registry.Register(openai)
	_ = registry.Register(groq)
	_ = registry.SetDefault("groq")

	err := registry.Remove("groq")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if len(registry.List()) != 1 {
		t.Errorf("Expected 1 provider after remove, got %d", len(registry.List()))
	}

	_, err = registry.Get("groq")
	if err != ErrProviderNotFound {
		t.Errorf("Expected ErrProviderNotFound after remove, got %v", err)
	}
}

func TestRegistry_Remove_DefaultSwitch(t *testing.T) {
	registry := NewRegistry()

	openai := NewOpenAIProvider("test-key", nil, nil)
	groq := NewGroqProvider("test-key", nil, nil)

	_ = registry.Register(openai)
	_ = registry.Register(groq)
	_ = registry.SetDefault("openai")

	_ = registry.Remove("openai")

	defaultProvider := registry.GetDefault()
	if defaultProvider == nil {
		t.Error("Expected default provider to switch to remaining provider")
		return
	}

	if defaultProvider.Name() != "groq" {
		t.Errorf("Expected default to switch to 'groq', got %s", defaultProvider.Name())
	}
}

func TestRegistry_Remove_NotFound(t *testing.T) {
	registry := NewRegistry()

	err := registry.Remove("nonexistent")
	if err != ErrProviderNotFound {
		t.Errorf("Expected ErrProviderNotFound, got %v", err)
	}
}

func TestRegistry_Empty(t *testing.T) {
	registry := NewRegistry()

	if len(registry.List()) != 0 {
		t.Error("Expected empty registry")
	}

	if registry.GetDefault() != nil {
		t.Error("Expected nil default for empty registry")
	}

	_, err := registry.Get("any")
	if err != ErrProviderNotFound {
		t.Errorf("Expected ErrProviderNotFound for empty registry, got %v", err)
	}
}

func TestBaseProvider_Timeout(t *testing.T) {
	provider := &BaseProvider{
		name:     "test",
		apiBase:  "https://api.test.com",
		timeout:  45 * time.Second,
		timeoutS: 3 * time.Minute,
	}

	if provider.GetTimeout(false) != 45*time.Second {
		t.Errorf("Expected normal timeout 45s, got %v", provider.GetTimeout(false))
	}

	if provider.GetTimeout(true) != 3*time.Minute {
		t.Errorf("Expected stream timeout 3m, got %v", provider.GetTimeout(true))
	}
}

func TestBaseProvider_Timeout_Default(t *testing.T) {
	provider := &BaseProvider{
		name:    "test",
		apiBase: "https://api.test.com",
	}

	if provider.GetTimeout(false) != DefaultTimeout {
		t.Errorf("Expected default normal timeout, got %v", provider.GetTimeout(false))
	}

	if provider.GetTimeout(true) != DefaultStreamTimeout {
		t.Errorf("Expected default stream timeout, got %v", provider.GetTimeout(true))
	}
}

func TestProvider_SetAPIBase(t *testing.T) {
	provider := NewOpenAIProvider("test-key", nil, nil)
	provider.SetAPIBase("https://custom.api.com/v1")

	if provider.APIBase() != "https://custom.api.com/v1" {
		t.Errorf("Expected custom API base, got %s", provider.APIBase())
	}
}

func TestProvider_SetTimeout(t *testing.T) {
	provider := NewOpenAIProvider("test-key", nil, nil)
	provider.SetTimeout(60*time.Second, 10*time.Minute)

	if provider.GetTimeout(false) != 60*time.Second {
		t.Errorf("Expected 60s timeout, got %v", provider.GetTimeout(false))
	}

	if provider.GetTimeout(true) != 10*time.Minute {
		t.Errorf("Expected 10m stream timeout, got %v", provider.GetTimeout(true))
	}
}

func TestIntegration_FullRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Expected Bearer token, got %s", r.Header.Get("Authorization"))
		}

		resp := openai.ChatCompletionResponse{
			ID:      "test-id",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4",
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: openai.ChatMessage{
						Role:    "assistant",
						Content: "Hello, how can I help you?",
					},
					FinishReason: "stop",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test-key", nil, nil)
	provider.SetAPIBase(server.URL)

	req := &openai.ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []openai.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	httpReq, err := provider.TransformRequest(req, "test-key")
	if err != nil {
		t.Fatalf("TransformRequest failed: %v", err)
	}

	client := &http.Client{}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer httpResp.Body.Close()

	resp, err := provider.TransformResponse(httpResp)
	if err != nil {
		t.Fatalf("TransformResponse failed: %v", err)
	}

	if resp.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got %s", resp.ID)
	}

	if resp.Choices[0].Message.Content != "Hello, how can I help you?" {
		t.Errorf("Unexpected response content: %s", resp.Choices[0].Message.Content)
	}
}
