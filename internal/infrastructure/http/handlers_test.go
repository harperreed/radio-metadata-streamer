// ABOUTME: Tests for HTTP handlers
// ABOUTME: Verifies routing, headers, and response formats
package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/harper/radio-metadata-proxy/internal/application/manager"
	"github.com/harper/radio-metadata-proxy/internal/application/config"
)

func TestStreamHandler_404(t *testing.T) {
	cfg := &config.Config{
		Stations: []config.StationConfig{},
	}

	mgr, _ := manager.NewFromConfig(cfg)

	handler := NewStreamHandler(mgr)

	req := httptest.NewRequest("GET", "/nonexistent/stream", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestStreamHandler_Success(t *testing.T) {
	cfg := &config.Config{
		Stations: []config.StationConfig{
			{
				ID: "test_station",
				ICY: config.ICYConfig{
					Name:            "Test Station",
					MetaInt:         16384,
					BitrateHintKbps: 128,
				},
				Source: config.SourceConfig{
					URL:              "http://example.com/stream.mp3",
					ConnectTimeoutMs: 5000,
				},
				Metadata: config.MetadataConfig{
					URL:    "http://example.com/meta",
					PollMs: 3000,
				},
				Buffering: config.BufferingConfig{
					RingBytes: 262144,
				},
			},
		},
	}

	mgr, _ := manager.NewFromConfig(cfg)

	handler := NewStreamHandler(mgr)

	req := httptest.NewRequest("GET", "/test_station/stream", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "audio/mpeg" {
		t.Errorf("expected Content-Type audio/mpeg, got %s", ct)
	}

	if name := rec.Header().Get("icy-name"); name != "Test Station" {
		t.Errorf("expected icy-name 'Test Station', got %s", name)
	}

	if metaint := rec.Header().Get("icy-metaint"); metaint != "16384" {
		t.Errorf("expected icy-metaint 16384, got %s", metaint)
	}

	if br := rec.Header().Get("icy-br"); br != "128" {
		t.Errorf("expected icy-br 128, got %s", br)
	}
}

func TestMetaHandler_404(t *testing.T) {
	cfg := &config.Config{
		Stations: []config.StationConfig{},
	}

	mgr, _ := manager.NewFromConfig(cfg)

	handler := NewMetaHandler(mgr)

	req := httptest.NewRequest("GET", "/nonexistent/meta", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestMetaHandler_Success(t *testing.T) {
	cfg := &config.Config{
		Stations: []config.StationConfig{
			{
				ID: "test_station",
				ICY: config.ICYConfig{
					Name:            "Test Station",
					MetaInt:         16384,
					BitrateHintKbps: 128,
				},
				Source: config.SourceConfig{
					URL:              "http://example.com/stream.mp3",
					ConnectTimeoutMs: 5000,
				},
				Metadata: config.MetadataConfig{
					URL:    "http://example.com/meta",
					PollMs: 3000,
				},
				Buffering: config.BufferingConfig{
					RingBytes: 262144,
				},
			},
		},
	}

	mgr, _ := manager.NewFromConfig(cfg)

	handler := NewMetaHandler(mgr)

	req := httptest.NewRequest("GET", "/test_station/meta", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var resp struct {
		Current       string  `json:"current"`
		UpdatedAt     *string `json:"updated_at,omitempty"`
		SourceHealthy bool    `json:"sourceHealthy"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Current != "" {
		t.Errorf("expected empty current metadata, got %s", resp.Current)
	}

	if resp.SourceHealthy {
		t.Error("expected sourceHealthy false initially")
	}
}

func TestStationsHandler(t *testing.T) {
	cfg := &config.Config{
		Stations: []config.StationConfig{
			{
				ID: "station1",
				ICY: config.ICYConfig{
					Name:            "Station 1",
					MetaInt:         16384,
					BitrateHintKbps: 128,
				},
				Source: config.SourceConfig{
					URL:              "http://example.com/stream1.mp3",
					ConnectTimeoutMs: 5000,
				},
				Metadata: config.MetadataConfig{
					URL:    "http://example.com/meta1",
					PollMs: 3000,
				},
				Buffering: config.BufferingConfig{
					RingBytes: 262144,
				},
			},
			{
				ID: "station2",
				ICY: config.ICYConfig{
					Name:            "Station 2",
					MetaInt:         16384,
					BitrateHintKbps: 128,
				},
				Source: config.SourceConfig{
					URL:              "http://example.com/stream2.mp3",
					ConnectTimeoutMs: 5000,
				},
				Metadata: config.MetadataConfig{
					URL:    "http://example.com/meta2",
					PollMs: 3000,
				},
				Buffering: config.BufferingConfig{
					RingBytes: 262144,
				},
			},
		},
	}

	mgr, _ := manager.NewFromConfig(cfg)

	handler := NewStationsHandler(mgr)

	req := httptest.NewRequest("GET", "/stations", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var stations []struct {
		ID            string `json:"id"`
		StreamURL     string `json:"stream_url"`
		MetaURL       string `json:"meta_url"`
		Clients       int    `json:"clients"`
		SourceHealthy bool   `json:"sourceHealthy"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&stations); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(stations) != 2 {
		t.Errorf("expected 2 stations, got %d", len(stations))
	}
}

func TestHealthzHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	HealthzHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var resp struct {
		OK bool `json:"ok"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.OK {
		t.Error("expected ok: true")
	}
}
