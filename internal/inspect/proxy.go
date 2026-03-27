package inspect

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	HelperCommandName    = "__inspect-proxy"
	DefaultClaudeBaseURL = "https://api.anthropic.com"
)

type CaptureEntry struct {
	Timestamp  time.Time        `json:"timestamp"`
	Method     string           `json:"method"`
	Path       string           `json:"path"`
	Upstream   string           `json:"upstream_url"`
	DurationMS int64            `json:"duration_ms"`
	Request    CaptureMessage   `json:"request"`
	Response   *CaptureResponse `json:"response,omitempty"`
	Error      string           `json:"error,omitempty"`
}

type CaptureMessage struct {
	Headers map[string][]string `json:"headers,omitempty"`
	Body    string              `json:"body,omitempty"`
}

type CaptureResponse struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       string              `json:"body,omitempty"`
}

type HelperOptions struct {
	Executable      string
	UpstreamBaseURL string
	CapturePath     string
	ReadyPath       string
	StderrPath      string
}

type HelperProcess struct {
	URL string

	cmd *exec.Cmd
}

type captureWriter struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

func StartHelper(opts HelperOptions) (*HelperProcess, error) {
	if opts.Executable == "" {
		return nil, fmt.Errorf("helper executable is required")
	}
	if strings.TrimSpace(opts.UpstreamBaseURL) == "" {
		return nil, fmt.Errorf("upstream base URL is required")
	}
	if opts.CapturePath == "" || opts.ReadyPath == "" {
		return nil, fmt.Errorf("capture and ready paths are required")
	}
	if err := os.MkdirAll(filepath.Dir(opts.CapturePath), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(opts.ReadyPath), 0700); err != nil {
		return nil, err
	}
	if opts.StderrPath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.StderrPath), 0700); err != nil {
			return nil, err
		}
	}
	_ = os.Remove(opts.ReadyPath)

	args := []string{
		HelperCommandName,
		"--listen-addr", "127.0.0.1:0",
		"--upstream", opts.UpstreamBaseURL,
		"--capture-file", opts.CapturePath,
		"--ready-file", opts.ReadyPath,
	}
	cmd := exec.Command(opts.Executable, args...)
	cmd.Stdout = nil
	if opts.StderrPath != "" {
		stderrFile, err := os.OpenFile(opts.StderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return nil, err
		}
		defer stderrFile.Close()
		cmd.Stderr = stderrFile
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			break
		}
		data, err := os.ReadFile(opts.ReadyPath)
		if err == nil {
			baseURL := strings.TrimSpace(string(data))
			if baseURL != "" {
				return &HelperProcess{URL: baseURL, cmd: cmd}, nil
			}
		}
		time.Sleep(25 * time.Millisecond)
	}

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return nil, fmt.Errorf("inspect proxy did not become ready")
}

func (hp *HelperProcess) Close() {
	if hp == nil || hp.cmd == nil || hp.cmd.Process == nil {
		return
	}
	_ = hp.cmd.Process.Kill()
	_ = hp.cmd.Wait()
}

func RunProxy(ctx context.Context, listenAddr, upstreamBaseURL, capturePath, readyPath string) error {
	upstream, err := url.Parse(strings.TrimSpace(upstreamBaseURL))
	if err != nil {
		return fmt.Errorf("parse upstream URL: %w", err)
	}
	if upstream.Scheme == "" || upstream.Host == "" {
		return fmt.Errorf("upstream URL must include scheme and host")
	}

	cw, err := newCaptureWriter(capturePath)
	if err != nil {
		return err
	}
	defer cw.Close()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			proxyRequest(ctx, transport, cw, upstream, w, r)
		}),
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	defer ln.Close()

	if err := os.MkdirAll(filepath.Dir(readyPath), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(readyPath, []byte("http://"+ln.Addr().String()+"\n"), 0600); err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		err := <-errCh
		if err == nil || err == http.ErrServerClosed {
			return nil
		}
		return err
	case err := <-errCh:
		if err == nil || err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func proxyRequest(parent context.Context, transport http.RoundTripper, cw *captureWriter, upstream *url.URL, w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	entry := CaptureEntry{
		Timestamp: started.UTC(),
		Method:    r.Method,
		Path:      r.URL.RequestURI(),
	}

	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		entry.Error = fmt.Sprintf("read request body: %v", err)
		cw.Write(entry)
		return
	}
	entry.Request = CaptureMessage{
		Headers: redactHeaders(r.Header),
		Body:    string(reqBody),
	}

	target := joinURL(upstream, r.URL)
	entry.Upstream = target.String()

	upstreamReq, err := http.NewRequestWithContext(parent, r.Method, target.String(), strings.NewReader(string(reqBody)))
	if err != nil {
		http.Error(w, "failed to create upstream request", http.StatusBadGateway)
		entry.Error = fmt.Sprintf("create upstream request: %v", err)
		entry.DurationMS = time.Since(started).Milliseconds()
		cw.Write(entry)
		return
	}
	upstreamReq.Header = cloneHeader(r.Header)
	upstreamReq.Host = upstream.Host

	resp, err := transport.RoundTrip(upstreamReq)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		entry.Error = err.Error()
		entry.DurationMS = time.Since(started).Milliseconds()
		cw.Write(entry)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	respBuf := &strings.Builder{}
	reader := bufio.NewReader(resp.Body)
	for {
		chunk := make([]byte, 32*1024)
		n, readErr := reader.Read(chunk)
		if n > 0 {
			part := chunk[:n]
			_, _ = w.Write(part)
			respBuf.Write(part)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			entry.Error = fmt.Sprintf("read upstream response: %v", readErr)
			break
		}
	}

	entry.DurationMS = time.Since(started).Milliseconds()
	entry.Response = &CaptureResponse{
		StatusCode: resp.StatusCode,
		Headers:    redactHeaders(resp.Header),
		Body:       respBuf.String(),
	}
	cw.Write(entry)
}

func joinURL(base *url.URL, reqURL *url.URL) *url.URL {
	clone := *base
	basePath := strings.TrimSuffix(clone.Path, "/")
	reqPath := "/" + strings.TrimPrefix(reqURL.Path, "/")
	if basePath == "" {
		clone.Path = reqPath
	} else {
		clone.Path = basePath + reqPath
	}
	clone.RawQuery = reqURL.RawQuery
	return &clone
}

func newCaptureWriter(path string) (*captureWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	return &captureWriter{
		f:   f,
		enc: json.NewEncoder(f),
	}, nil
}

func (cw *captureWriter) Write(entry CaptureEntry) {
	if cw == nil {
		return
	}
	cw.mu.Lock()
	defer cw.mu.Unlock()
	_ = cw.enc.Encode(entry)
}

func (cw *captureWriter) Close() {
	if cw == nil || cw.f == nil {
		return
	}
	cw.mu.Lock()
	defer cw.mu.Unlock()
	_ = cw.f.Close()
}

func redactHeaders(h http.Header) map[string][]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string][]string, len(h))
	for k, v := range h {
		lower := strings.ToLower(k)
		switch lower {
		case "authorization", "x-api-key", "cookie", "set-cookie", "anthropic-api-key":
			out[k] = []string{"[redacted]"}
		default:
			copied := make([]string, len(v))
			copy(copied, v)
			out[k] = copied
		}
	}
	return out
}

func cloneHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, v := range h {
		copied := make([]string, len(v))
		copy(copied, v)
		out[k] = copied
	}
	return out
}

func copyResponseHeaders(dst, src http.Header) {
	for k, values := range src {
		if strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, value := range values {
			dst.Add(k, value)
		}
	}
}
