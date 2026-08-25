package webapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxRecordedBodyBytes = 16 << 20

// HTTPRecorder captures the browser-facing API contract as credential-safe
// JSON Lines. It records request and response bodies, including completed SSE
// streams, so a successful workflow can be inspected or converted to fixtures.
type HTTPRecorder struct {
	mu       sync.Mutex
	file     *os.File
	sequence atomic.Uint64
}

type httpTraceEntry struct {
	Sequence   uint64            `json:"sequence"`
	StartedAt  time.Time         `json:"startedAt"`
	DurationMS int64             `json:"durationMs"`
	Request    httpTraceRequest  `json:"request"`
	Response   httpTraceResponse `json:"response"`
}

type httpTraceRequest struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	ContentType string `json:"contentType,omitempty"`
	Body        string `json:"body,omitempty"`
}

type httpTraceResponse struct {
	Status      int    `json:"status"`
	ContentType string `json:"contentType,omitempty"`
	Body        string `json:"body,omitempty"`
}

func NewHTTPRecorder(path string) (*HTTPRecorder, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("API recording path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create API recording directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open API recording: %w", err)
	}
	return &HTTPRecorder{file: file}, nil
}

func (recorder *HTTPRecorder) Close() error {
	if recorder == nil || recorder.file == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return recorder.file.Close()
}

func (recorder *HTTPRecorder) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now().UTC()
		sequence := recorder.sequence.Add(1)
		requestBody := []byte(nil)
		if r.Body != nil {
			requestBody, _ = io.ReadAll(io.LimitReader(r.Body, maxRecordedBodyBytes))
		}
		if r.Body != nil {
			_ = r.Body.Close()
		}
		r.Body = io.NopCloser(bytes.NewReader(requestBody))
		captured := &traceResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(captured, r)
		entry := httpTraceEntry{
			Sequence: sequence, StartedAt: startedAt, DurationMS: time.Since(startedAt).Milliseconds(),
			Request: httpTraceRequest{
				Method: r.Method, Path: r.URL.Path, ContentType: r.Header.Get("Content-Type"),
				Body: redactRecordedBody(requestBody, r.Header.Get("Content-Type")),
			},
			Response: httpTraceResponse{
				Status: captured.status, ContentType: captured.Header().Get("Content-Type"),
				Body: redactRecordedBody(captured.body.Bytes(), captured.Header().Get("Content-Type")),
			},
		}
		recorder.write(entry)
	})
}

func (recorder *HTTPRecorder) write(entry httpTraceEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.file == nil {
		return
	}
	_, _ = recorder.file.Write(append(data, '\n'))
	_ = recorder.file.Sync()
}

type traceResponseWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (writer *traceResponseWriter) WriteHeader(status int) {
	if writer.status != http.StatusOK {
		return
	}
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *traceResponseWriter) Write(data []byte) (int, error) {
	if writer.body.Len() < maxRecordedBodyBytes {
		remaining := maxRecordedBodyBytes - writer.body.Len()
		_, _ = writer.body.Write(data[:min(len(data), remaining)])
	}
	return writer.ResponseWriter.Write(data)
}

func (writer *traceResponseWriter) Flush() {
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

var recordedSecretKeys = map[string]bool{
	"accesstoken": true, "refreshtoken": true, "clientsecret": true,
	"code": true, "codeverifier": true, "authorizeurl": true,
}

func redactRecordedBody(data []byte, contentType string) string {
	if len(data) == 0 {
		return ""
	}
	if !strings.Contains(strings.ToLower(contentType), "json") {
		return string(data)
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return string(data)
	}
	redactRecordedValue(value)
	redacted, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(redacted)
}

func redactRecordedValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			if recordedSecretKeys[normalized] {
				typed[key] = "[REDACTED]"
				continue
			}
			redactRecordedValue(child)
		}
	case []any:
		for _, child := range typed {
			redactRecordedValue(child)
		}
	}
}
