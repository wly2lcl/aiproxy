package proxy

import (
	"bufio"
	"io"
	"net/http"

	"github.com/wangluyao/aiproxy/pkg/openai"
)

type StreamHandler struct {
	proxy          *Proxy
	tokenExtractor *TokenExtractor
}

func NewStreamHandler(proxy *Proxy) *StreamHandler {
	return &StreamHandler{
		proxy:          proxy,
		tokenExtractor: NewTokenExtractor(4),
	}
}

type flushWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (fw *flushWriter) Write(p []byte) (n int, err error) {
	n, err = fw.w.Write(p)
	if fw.flusher != nil {
		fw.flusher.Flush()
	}
	return
}

func (h *StreamHandler) ServeStream(w http.ResponseWriter, r *http.Request, upstreamResp *http.Response) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return nil
	}

	fw := &flushWriter{
		w:       w,
		flusher: flusher,
	}

	reader := bufio.NewReader(upstreamResp.Body)
	defer upstreamResp.Body.Close()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if _, err := fw.Write([]byte(line)); err != nil {
			return err
		}

		if openai.IsDoneChunk(line) {
			break
		}

		chunk, err := openai.ParseStreamLine(line)
		if err != nil {
			continue
		}

		if chunk != nil && chunk.Usage != nil {
			prompt, completion, found := h.tokenExtractor.ExtractFromStream(chunk)
			if found {
				_ = prompt
				_ = completion
			}
		}
	}

	return nil
}

func (h *StreamHandler) GetTokenExtractor() *TokenExtractor {
	return h.tokenExtractor
}
