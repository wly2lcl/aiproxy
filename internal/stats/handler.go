package stats

import (
	"encoding/json"
	"net/http"
)

type Handler struct {
	reporter *Reporter
}

func NewHandler(reporter *Reporter) *Handler {
	return &Handler{
		reporter: reporter,
	}
}

func (h *Handler) ServePrometheus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(h.reporter.Prometheus()))
}

func (h *Handler) ServeJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	data := h.reporter.JSON()
	var raw interface{}
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		w.Write([]byte("{}"))
		return
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(raw)
}
