package proxy

import (
	"context"
	"net/http"
	"time"
)

type Transport struct {
	*http.Transport
	config *Config
}

func NewTransport(cfg *Config) *Transport {
	if cfg == nil {
		cfg = &Config{
			Timeout:         30 * time.Second,
			StreamTimeout:   5 * time.Minute,
			MaxIdleConns:    100,
			IdleConnTimeout: 90 * time.Second,
		}
	}

	transport := &http.Transport{
		MaxIdleConns:        cfg.MaxIdleConns,
		IdleConnTimeout:     cfg.IdleConnTimeout,
		DisableCompression:  false,
		MaxIdleConnsPerHost: cfg.MaxIdleConns / 10,
		MaxConnsPerHost:     cfg.MaxIdleConns,
	}

	return &Transport{
		Transport: transport,
		config:    cfg,
	}
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	timeout := t.config.Timeout

	streamingVal := ctx.Value("streaming")
	if streamingVal != nil {
		if isStreaming, ok := streamingVal.(bool); ok && isStreaming {
			timeout = t.config.StreamTimeout
		}
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}

	resp, err := t.Transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (t *Transport) CloseIdleConnections() {
	t.Transport.CloseIdleConnections()
}

func (t *Transport) GetConfig() *Config {
	return t.config
}
