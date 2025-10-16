// ABOUTME: HTTP metadata provider implementation with JSON parsing
// ABOUTME: Formats metadata into ICY-compatible strings
package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type BuildConfig struct {
	Format              string
	StripSingleQuotes   bool
	NormalizeWhitespace bool
	FallbackKeyOrder    []string
}

type HTTPConfig struct {
	URL     string
	Timeout time.Duration
	Build   BuildConfig
}

type HTTPProvider struct {
	cfg    HTTPConfig
	client *http.Client
}

func NewHTTP(cfg HTTPConfig) *HTTPProvider {
	client := &http.Client{
		Timeout: cfg.Timeout,
	}

	return &HTTPProvider{
		cfg:    cfg,
		client: client,
	}
}

func (h *HTTPProvider) Fetch(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", h.cfg.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Cache-Control", "no-store")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	// Parse JSON
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("parse json: %w", err)
	}

	// Build ICY string from format template
	result := h.cfg.Build.Format

	// Simple placeholder replacement
	result = strings.ReplaceAll(result, "{artist}", getString(data, "artist"))
	result = strings.ReplaceAll(result, "{title}", getString(data, "title"))

	// Apply transformations
	if h.cfg.Build.StripSingleQuotes {
		result = strings.ReplaceAll(result, "'", "")
	}

	if h.cfg.Build.NormalizeWhitespace {
		result = strings.Join(strings.Fields(result), " ")
	}

	return result, nil
}

func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}
