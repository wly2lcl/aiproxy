package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangluyao/aiproxy/pkg/openai"
)

func TestTokenExtractor_ExtractFromStream(t *testing.T) {
	extractor := NewTokenExtractor(4)

	t.Run("nil chunk", func(t *testing.T) {
		prompt, completion, found := extractor.ExtractFromStream(nil)
		if found {
			t.Error("Expected found to be false for nil chunk")
		}
		if prompt != 0 || completion != 0 {
			t.Error("Expected zero values for nil chunk")
		}
	})

	t.Run("chunk without usage", func(t *testing.T) {
		chunk := &openai.StreamChunk{
			ID: "test-id",
		}
		prompt, completion, found := extractor.ExtractFromStream(chunk)
		if found {
			t.Error("Expected found to be false for chunk without usage")
		}
		if prompt != 0 || completion != 0 {
			t.Error("Expected zero values for chunk without usage")
		}
	})

	t.Run("chunk with usage", func(t *testing.T) {
		chunk := &openai.StreamChunk{
			ID: "test-id",
			Usage: &openai.Usage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		}
		prompt, completion, found := extractor.ExtractFromStream(chunk)
		if !found {
			t.Error("Expected found to be true for chunk with usage")
		}
		if prompt != 100 {
			t.Errorf("Expected prompt tokens 100, got %d", prompt)
		}
		if completion != 50 {
			t.Errorf("Expected completion tokens 50, got %d", completion)
		}
	})
}

func TestTokenExtractor_EstimateFromText(t *testing.T) {
	tests := []struct {
		name          string
		charsPerToken int
		text          string
		expected      int
	}{
		{"default chars per token", 4, "hello world", 3},
		{"empty text", 4, "", 0},
		{"exact multiple", 4, "abcd", 1},
		{"not exact multiple", 4, "abcde", 2},
		{"custom chars per token", 2, "abcd", 2},
		{"single char", 4, "a", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewTokenExtractor(tt.charsPerToken)
			result := extractor.EstimateFromText(tt.text)
			if result != tt.expected {
				t.Errorf("EstimateFromText(%q) = %d, expected %d", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTokenExtractor_IsHybridMode(t *testing.T) {
	extractor := NewTokenExtractor(4)
	if !extractor.IsHybridMode() {
		t.Error("Expected hybrid mode to be true by default")
	}
}

func TestTokenExtractor_DefaultCharsPerToken(t *testing.T) {
	extractor := NewTokenExtractor(0)
	result := extractor.EstimateFromText("abcdefgh")
	if result != 2 {
		t.Errorf("Expected 2 tokens for 8 chars with default 4 chars/token, got %d", result)
	}
}

func TestTokenExtractor_NegativeCharsPerToken(t *testing.T) {
	extractor := NewTokenExtractor(-1)
	result := extractor.EstimateFromText("abcdefgh")
	if result != 2 {
		t.Errorf("Expected 2 tokens for 8 chars with default 4 chars/token, got %d", result)
	}
}

func TestFlushWriter(t *testing.T) {
	recorder := httptest.NewRecorder()

	fw := &flushWriter{
		w:       recorder,
		flusher: recorder,
	}

	data := []byte("test data")
	n, err := fw.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Expected %d bytes written, got %d", len(data), n)
	}

	if recorder.Body.String() != "test data" {
		t.Errorf("Expected body 'test data', got %q", recorder.Body.String())
	}
}

func TestFlushWriter_NilFlusher(t *testing.T) {
	w := &mockResponseWriter{body: &bytes.Buffer{}}
	fw := &flushWriter{
		w:       w,
		flusher: nil,
	}

	data := []byte("test data")
	n, err := fw.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Expected %d bytes written, got %d", len(data), n)
	}
}

type mockResponseWriter struct {
	body   *bytes.Buffer
	header http.Header
}

func (m *mockResponseWriter) Header() http.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return m.header
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	return m.body.Write(b)
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {}

func TestStreamHandler_ServeStream(t *testing.T) {
	t.Run("basic streaming", func(t *testing.T) {
		chunks := []string{
			"data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":123,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":123,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"},\"finish_reason\":null}]}\n\n",
			"data: [DONE]\n\n",
		}

		upstreamBody := strings.Join(chunks, "")
		upstreamResp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(upstreamBody)),
			Header:     make(http.Header),
		}

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		handler := NewStreamHandler(nil)
		err := handler.ServeStream(recorder, req, upstreamResp)
		if err != nil {
			t.Fatalf("ServeStream failed: %v", err)
		}

		if recorder.Header().Get("Content-Type") != "text/event-stream" {
			t.Errorf("Expected Content-Type 'text/event-stream', got %q", recorder.Header().Get("Content-Type"))
		}

		if recorder.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("Expected Cache-Control 'no-cache', got %q", recorder.Header().Get("Cache-Control"))
		}

		body := recorder.Body.String()
		if !strings.Contains(body, "Hello") {
			t.Error("Expected body to contain 'Hello'")
		}
		if !strings.Contains(body, "[DONE]") {
			t.Error("Expected body to contain '[DONE]'")
		}
	})

	t.Run("with usage stats", func(t *testing.T) {
		chunks := []string{
			"data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":123,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"},\"finish_reason\":null}]}\n\n",
			"data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":123,\"model\":\"gpt-4\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n",
			"data: [DONE]\n\n",
		}

		upstreamBody := strings.Join(chunks, "")
		upstreamResp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(upstreamBody)),
			Header:     make(http.Header),
		}

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		handler := NewStreamHandler(nil)
		err := handler.ServeStream(recorder, req, upstreamResp)
		if err != nil {
			t.Fatalf("ServeStream failed: %v", err)
		}

		body := recorder.Body.String()
		if !strings.Contains(body, "prompt_tokens") {
			t.Error("Expected body to contain usage stats")
		}
	})
}

func TestStreamHandler_ChunkPassthrough(t *testing.T) {
	inputChunks := []string{
		"data: {\"id\":\"1\",\"choices\":[{\"delta\":{\"content\":\"A\"}}]}\n\n",
		"data: {\"id\":\"2\",\"choices\":[{\"delta\":{\"content\":\"B\"}}]}\n\n",
		"data: {\"id\":\"3\",\"choices\":[{\"delta\":{\"content\":\"C\"}}]}\n\n",
		"data: [DONE]\n\n",
	}

	upstreamBody := strings.Join(inputChunks, "")
	upstreamResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
		Header:     make(http.Header),
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	handler := NewStreamHandler(nil)
	err := handler.ServeStream(recorder, req, upstreamResp)
	if err != nil {
		t.Fatalf("ServeStream failed: %v", err)
	}

	body := recorder.Body.String()
	for _, chunk := range inputChunks {
		if !strings.Contains(body, strings.TrimSuffix(chunk, "\n\n")) {
			t.Errorf("Expected body to contain chunk: %q", chunk)
		}
	}
}

func TestStreamHandler_DoneMarker(t *testing.T) {
	chunks := []string{
		"data: {\"id\":\"test\",\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n",
		"data: [DONE]\n\n",
	}

	upstreamBody := strings.Join(chunks, "")
	upstreamResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
		Header:     make(http.Header),
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	handler := NewStreamHandler(nil)
	err := handler.ServeStream(recorder, req, upstreamResp)
	if err != nil {
		t.Fatalf("ServeStream failed: %v", err)
	}

	if !strings.Contains(recorder.Body.String(), "[DONE]") {
		t.Error("Expected [DONE] marker in response")
	}
}

func TestStreamHandler_GetTokenExtractor(t *testing.T) {
	handler := NewStreamHandler(nil)
	extractor := handler.GetTokenExtractor()
	if extractor == nil {
		t.Error("Expected non-nil token extractor")
	}
}
