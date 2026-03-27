package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"

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

type StreamError struct {
	Err       error
	Phase     string
	BytesRead int64
}

func (e *StreamError) Error() string {
	return fmt.Sprintf("stream error during %s: %v (bytes read: %d)", e.Phase, e.Err, e.BytesRead)
}

func (e *StreamError) Unwrap() error {
	return e.Err
}

func isUnexpectedEOF(err error) bool {
	return err != nil && (err == io.ErrUnexpectedEOF || strings.Contains(err.Error(), "unexpected EOF"))
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

	var bytesRead int64

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}

			if isUnexpectedEOF(err) {
				h.sendStreamError(fw, "upstream_closed", "upstream connection closed unexpectedly")
				return &StreamError{
					Err:       err,
					Phase:     "read_line",
					BytesRead: bytesRead,
				}
			}

			h.sendStreamError(fw, "read_error", fmt.Sprintf("failed to read stream: %v", err))
			return &StreamError{
				Err:       err,
				Phase:     "read_line",
				BytesRead: bytesRead,
			}
		}

		bytesRead += int64(len(line))

		if _, err := fw.Write([]byte(line)); err != nil {
			return &StreamError{
				Err:       err,
				Phase:     "write_client",
				BytesRead: bytesRead,
			}
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

func (h *StreamHandler) sendStreamError(fw *flushWriter, code, message string) {
	errorEvent := fmt.Sprintf("data: {\"error\":{\"message\":\"%s\",\"type\":\"stream_error\",\"code\":\"%s\"}}\n\n", message, code)
	fw.Write([]byte(errorEvent))
}

func (h *StreamHandler) GetTokenExtractor() *TokenExtractor {
	return h.tokenExtractor
}
