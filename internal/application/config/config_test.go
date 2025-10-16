// ABOUTME: Tests for YAML configuration parsing
// ABOUTME: Verifies config structure and validation
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	yamlContent := `
listen:
  host: 0.0.0.0
  port: 8000

stations:
  - id: test_station
    icy:
      name: "Test Station"
      metaint: 16384
      bitrate_hint_kbps: 128
    source:
      url: "http://example.com/stream.mp3"
      connect_timeout_ms: 5000
      read_timeout_ms: 15000
    metadata:
      url: "http://example.com/meta"
      poll_ms: 3000
      build:
        format: "StreamTitle='{artist} - {title}';"
    buffering:
      ring_bytes: 262144
`

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Listen.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Listen.Host)
	}

	if cfg.Listen.Port != 8000 {
		t.Errorf("expected port 8000, got %d", cfg.Listen.Port)
	}

	if len(cfg.Stations) != 1 {
		t.Fatalf("expected 1 station, got %d", len(cfg.Stations))
	}

	st := cfg.Stations[0]
	if st.ID != "test_station" {
		t.Errorf("expected ID test_station, got %s", st.ID)
	}
}
