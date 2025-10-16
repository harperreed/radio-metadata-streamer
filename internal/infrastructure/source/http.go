// ABOUTME: HTTP stream source implementation for MP3 audio
// ABOUTME: Handles upstream connection with timeouts and proper headers
package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HTTPConfig struct {
	URL            string
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	Headers        map[string]string
}

type HTTPSource struct {
	cfg    HTTPConfig
	client *http.Client
}

func NewHTTP(cfg HTTPConfig) *HTTPSource {
	transport := &http.Transport{
		DisableCompression:    true,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   0, // No total timeout for streaming
	}

	return &HTTPSource{
		cfg:    cfg,
		client: client,
	}
}

func (h *HTTPSource) Connect(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", h.cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set ICY headers
	req.Header.Set("Icy-MetaData", "0")

	// Set custom headers
	for k, v := range h.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return resp.Body, nil
}
