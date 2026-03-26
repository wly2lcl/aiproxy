package provider

import (
	"net/http"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/pkg/openai"
)

type Provider interface {
	Name() string
	APIBase() string
	TransformRequest(req *openai.ChatCompletionRequest, apiKey string) (*http.Request, error)
	TransformResponse(resp *http.Response) (*openai.ChatCompletionResponse, error)
	SupportsModel(model string) bool
	GetHeaders(apiKey string) map[string]string
	GetTimeout(isStreaming bool) time.Duration
}

type BaseProvider struct {
	name     string
	apiBase  string
	models   []string
	config   *domain.ProviderConfig
	headers  map[string]string
	timeout  time.Duration
	timeoutS time.Duration
}

func (p *BaseProvider) Name() string {
	return p.name
}

func (p *BaseProvider) APIBase() string {
	return p.apiBase
}

func (p *BaseProvider) SupportsModel(model string) bool {
	if len(p.models) == 0 {
		return true
	}
	for _, m := range p.models {
		if m == model {
			return true
		}
	}
	return false
}

func (p *BaseProvider) GetTimeout(isStreaming bool) time.Duration {
	if isStreaming && p.timeoutS > 0 {
		return p.timeoutS
	}
	if p.timeout > 0 {
		return p.timeout
	}
	if isStreaming {
		return 5 * time.Minute
	}
	return 30 * time.Second
}

func (p *BaseProvider) SetName(name string) {
	p.name = name
}

const (
	DefaultTimeout       = 30 * time.Second
	DefaultStreamTimeout = 5 * time.Minute
)
