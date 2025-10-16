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

	// Build ICY string from format template with all placeholders
	result := h.cfg.Build.Format

	// Replace all placeholders: {artist}, {title}, {album}, {year}, etc.
	placeholders := []string{"artist", "title", "album", "year", "label"}
	for _, placeholder := range placeholders {
		value := h.extractValue(data, placeholder)
		result = strings.ReplaceAll(result, "{"+placeholder+"}", value)
	}

	// Apply transformations
	if h.cfg.Build.StripSingleQuotes {
		result = strings.ReplaceAll(result, "'", "")
	}

	if h.cfg.Build.NormalizeWhitespace {
		result = strings.Join(strings.Fields(result), " ")
	}

	return result, nil
}

// extractValue tries to extract a value using fallback paths or simple key lookup
func (h *HTTPProvider) extractValue(data map[string]interface{}, placeholder string) string {
	// If FallbackKeyOrder is configured, use it
	if len(h.cfg.Build.FallbackKeyOrder) > 0 {
		// Map placeholder to fallback path index
		// Order: artist, title, album, year, label, ...
		placeholderMap := map[string]int{
			"artist": 0,
			"title":  1,
			"album":  2,
			"year":   3,
			"label":  4,
		}

		if idx, ok := placeholderMap[placeholder]; ok && idx < len(h.cfg.Build.FallbackKeyOrder) {
			path := h.cfg.Build.FallbackKeyOrder[idx]
			if val := getNestedString(data, path); val != "" {
				return val
			}
		}
	}

	// Fallback to simple key lookup
	return getString(data, placeholder)
}

func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

// getNestedString traverses a nested JSON path using dot notation
// e.g., "now.secondLine.title" → data["now"]["secondLine"]["title"]
// Handles strings and numbers (converts numbers to strings)
func getNestedString(data map[string]interface{}, path string) string {
	parts := strings.Split(path, ".")
	var current interface{} = data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		default:
			return ""
		}
	}

	// Handle different types
	switch v := current.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case bool:
		return fmt.Sprintf("%t", v)
	}
	return ""
}
