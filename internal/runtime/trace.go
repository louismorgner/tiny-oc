package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// traceEntry is one JSONL line written to trace.jsonl when tracing is enabled.
type traceEntry struct {
	Turn      int          `json:"turn"`
	Timestamp time.Time    `json:"timestamp"`
	Request   chatRequest  `json:"request"`
	Response  *chatResponse `json:"response"`
	Tokens    traceTokens  `json:"tokens"`
}

type traceTokens struct {
	Prompt      int64 `json:"prompt"`
	Completion  int64 `json:"completion"`
	Total       int64 `json:"total"`
	CacheRead   int64 `json:"cache_read,omitempty"`
	CacheCreate int64 `json:"cache_create,omitempty"`
}

// traceWriter appends trace entries to a JSONL file. A nil traceWriter is safe
// to use — all methods no-op, so callers don't need to guard against nil.
type traceWriter struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

// newTraceWriter opens (or creates) trace.jsonl inside metadataDir for appending.
func newTraceWriter(metadataDir string) (*traceWriter, error) {
	if err := os.MkdirAll(metadataDir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(metadataDir, "trace.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	enc := json.NewEncoder(f)
	return &traceWriter{f: f, enc: enc}, nil
}

// WriteTurn appends one trace entry. Safe to call on a nil receiver.
func (tw *traceWriter) WriteTurn(turn int, req chatRequest, resp *chatResponse) {
	if tw == nil {
		return
	}
	entry := traceEntry{
		Turn:      turn,
		Timestamp: time.Now().UTC(),
		Request:   req,
		Response:  resp,
	}
	if resp != nil {
		entry.Tokens = traceTokens{
			Prompt:     resp.Usage.PromptTokens,
			Completion: resp.Usage.CompletionTokens,
			Total:      resp.Usage.TotalTokens,
		}
		if resp.Usage.PromptTokensDetails != nil {
			entry.Tokens.CacheRead = resp.Usage.PromptTokensDetails.CachedTokens
			entry.Tokens.CacheCreate = resp.Usage.PromptTokensDetails.CacheWriteTokens
		}
	}
	tw.mu.Lock()
	defer tw.mu.Unlock()
	// Encoding errors are best-effort; trace failures must never break the session.
	_ = tw.enc.Encode(entry)
}

// Close releases the underlying file handle. Safe to call on a nil receiver.
func (tw *traceWriter) Close() {
	if tw == nil {
		return
	}
	tw.mu.Lock()
	defer tw.mu.Unlock()
	_ = tw.f.Close()
}
