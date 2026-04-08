package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wangluyao/aiproxy/internal/domain"
	"github.com/wangluyao/aiproxy/internal/provider"
	"github.com/wangluyao/aiproxy/pkg/openai"
)

type Config struct {
	Timeout         time.Duration
	StreamTimeout   time.Duration
	MaxIdleConns    int
	IdleConnTimeout time.Duration
}

type Proxy struct {
	transport *Transport
	config    *Config
}

func NewProxy(cfg *Config) *Proxy {
	if cfg == nil {
		cfg = &Config{
			Timeout:         30 * time.Second,
			StreamTimeout:   5 * time.Minute,
			MaxIdleConns:    100,
			IdleConnTimeout: 90 * time.Second,
		}
	}

	return &Proxy{
		transport: NewTransport(cfg),
		config:    cfg,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	accountVal := ctx.Value("account")
	providerVal := ctx.Value("provider")

	if accountVal == nil || providerVal == nil {
		p.HandleError(w, fmt.Errorf("account or provider not found in context"))
		return
	}

	account, ok := accountVal.(*domain.Account)
	if !ok {
		p.HandleError(w, fmt.Errorf("invalid account type in context"))
		return
	}

	prov, ok := providerVal.(provider.Provider)
	if !ok {
		p.HandleError(w, fmt.Errorf("invalid provider type in context"))
		return
	}

	resp, err := p.Do(ctx, r)
	if err != nil {
		p.HandleError(w, err)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		p.HandleError(w, err)
		return
	}

	_ = account
	_ = prov
}

func (p *Proxy) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	accountVal := ctx.Value("account")
	providerVal := ctx.Value("provider")

	var account *domain.Account
	var prov provider.Provider

	if accountVal != nil {
		if a, ok := accountVal.(*domain.Account); ok {
			account = a
		}
	}

	if providerVal != nil {
		if pr, ok := providerVal.(provider.Provider); ok {
			prov = pr
		}
	}

	outReq := req.Clone(ctx)

	if prov != nil && account != nil {
		if err := p.ModifyRequest(outReq, account, prov); err != nil {
			return nil, err
		}
	}

	return p.transport.RoundTrip(outReq)
}

func (p *Proxy) ModifyRequest(r *http.Request, account *domain.Account, prov provider.Provider) error {
	if prov == nil {
		return fmt.Errorf("provider is nil")
	}

	apiKey := ""
	if account != nil {
		apiKey = account.APIKey
	}

	headers := prov.GetHeaders(apiKey)
	for key, value := range headers {
		r.Header.Set(key, value)
	}

	return nil
}

func (p *Proxy) HandleError(w http.ResponseWriter, err error) {
	errResp := openai.ErrorResponse{
		Error: openai.ErrorDetail{
			Message: err.Error(),
			Type:    "proxy_error",
			Code:    "internal_error",
		},
	}

	if domainErr, ok := err.(*domain.DomainError); ok {
		errResp.Error.Code = domainErr.Code
		errResp.Error.Type = "domain_error"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)

	json.NewEncoder(w).Encode(errResp)
}

func (p *Proxy) GetTransport() *Transport {
	return p.transport
}
