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

type OpenAIProvider struct {
	BaseProvider
	apiKey       string
	extraHeaders map[string]string
}

func NewOpenAIProvider(apiKey string, config *domain.ProviderConfig, models []string, extraHeaders ...map[string]string) *OpenAIProvider {
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	var extra map[string]string
	if len(extraHeaders) > 0 {
		extra = extraHeaders[0]
		for k, v := range extra {
			headers[k] = v
		}
	}

	return &OpenAIProvider{
		BaseProvider: BaseProvider{
			name:     "openai",
			apiBase:  "https://api.openai.com/v1",
			models:   models,
			config:   config,
			headers:  headers,
			timeout:  DefaultTimeout,
			timeoutS: DefaultStreamTimeout,
		},
		apiKey:       apiKey,
		extraHeaders: extra,
	}
}

func (p *OpenAIProvider) GetHeaders(apiKey string) map[string]string {
	headers := make(map[string]string)
	for k, v := range p.headers {
		headers[k] = v
	}
	headers["Authorization"] = fmt.Sprintf("Bearer %s", apiKey)
	return headers
}

func (p *OpenAIProvider) TransformRequest(req *openai.ChatCompletionRequest, apiKey string) (*http.Request, error) {
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

func (p *OpenAIProvider) TransformResponse(resp *http.Response) (*openai.ChatCompletionResponse, error) {
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

func (p *OpenAIProvider) SetAPIBase(base string) {
	p.apiBase = base
}

func (p *OpenAIProvider) SetTimeout(timeout, streamTimeout time.Duration) {
	if timeout > 0 {
		p.timeout = timeout
	}
	if streamTimeout > 0 {
		p.timeoutS = streamTimeout
	}
}
