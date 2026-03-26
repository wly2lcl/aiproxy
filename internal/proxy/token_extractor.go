package proxy

import (
	"sync"

	"github.com/wangluyao/aiproxy/pkg/openai"
)

type TokenExtractor struct {
	charsPerToken int
	hybridMode    bool
	mu            sync.RWMutex
	lastUsage     struct {
		prompt     int
		completion int
		found      bool
	}
}

func NewTokenExtractor(charsPerToken int) *TokenExtractor {
	if charsPerToken <= 0 {
		charsPerToken = 4
	}
	return &TokenExtractor{
		charsPerToken: charsPerToken,
		hybridMode:    true,
	}
}

func (e *TokenExtractor) ExtractFromStream(chunk *openai.StreamChunk) (prompt, completion int, found bool) {
	if chunk == nil {
		e.mu.RLock()
		defer e.mu.RUnlock()
		return e.lastUsage.prompt, e.lastUsage.completion, e.lastUsage.found
	}

	if chunk.Usage == nil {
		return 0, 0, false
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastUsage.prompt = chunk.Usage.PromptTokens
	e.lastUsage.completion = chunk.Usage.CompletionTokens
	e.lastUsage.found = true

	return chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens, true
}

func (e *TokenExtractor) EstimateFromText(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + e.charsPerToken - 1) / e.charsPerToken
}

func (e *TokenExtractor) IsHybridMode() bool {
	return e.hybridMode
}

func (e *TokenExtractor) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastUsage.prompt = 0
	e.lastUsage.completion = 0
	e.lastUsage.found = false
}
