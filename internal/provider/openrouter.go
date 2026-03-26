package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/pkg/openai"
)

const (
	OpenRouterBaseURL = "https://openrouter.ai/api/v1"
)

type OpenRouterProvider struct {
	BaseProvider
	apiKey       string
	extraHeaders map[string]string
}

func NewOpenRouterProvider(apiKey string, config *domain.ProviderConfig, models []string, extraHeaders map[string]string) *OpenRouterProvider {
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	for k, v := range extraHeaders {
		headers[k] = v
	}

	return &OpenRouterProvider{
		BaseProvider: BaseProvider{
			name:     "openrouter",
			apiBase:  OpenRouterBaseURL,
			models:   models,
			config:   config,
			headers:  headers,
			timeout:  DefaultTimeout,
			timeoutS: DefaultStreamTimeout,
		},
		apiKey:       apiKey,
		extraHeaders: extraHeaders,
	}
}

func (p *OpenRouterProvider) GetHeaders(apiKey string) map[string]string {
	headers := make(map[string]string)
	for k, v := range p.headers {
		headers[k] = v
	}
	headers["Authorization"] = fmt.Sprintf("Bearer %s", apiKey)

	if p.extraHeaders != nil {
		if referer, ok := p.extraHeaders["HTTP-Referer"]; ok && referer != "" {
			headers["HTTP-Referer"] = referer
		}
		if title, ok := p.extraHeaders["X-Title"]; ok && title != "" {
			headers["X-Title"] = title
		}
	}

	return headers
}

func (p *OpenRouterProvider) TransformRequest(req *openai.ChatCompletionRequest, apiKey string) (*http.Request, error) {
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(p.apiBase, "/"))
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range p.GetHeaders(apiKey) {
		httpReq.Header.Set(k, v)
	}

	return httpReq, nil
}

func (p *OpenRouterProvider) TransformResponse(resp *http.Response) (*openai.ChatCompletionResponse, error) {
	if resp == nil {
		return nil, errors.New("response cannot be nil")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result openai.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

func (p *OpenRouterProvider) SupportsModel(model string) bool {
	if len(p.models) == 0 {
		return strings.Contains(model, "/") || !strings.Contains(model, "/")
	}
	for _, m := range p.models {
		if m == model {
			return true
		}
		if strings.HasSuffix(m, "/*") {
			prefix := strings.TrimSuffix(m, "*")
			if strings.HasPrefix(model, prefix) {
				return true
			}
		}
	}
	return false
}

func (p *OpenRouterProvider) SetAPIBase(base string) {
	p.apiBase = base
}

func (p *OpenRouterProvider) SetTimeout(timeout, streamTimeout time.Duration) {
	if timeout > 0 {
		p.timeout = timeout
	}
	if streamTimeout > 0 {
		p.timeoutS = streamTimeout
	}
}
