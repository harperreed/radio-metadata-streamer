// ABOUTME: Tests for station manager lifecycle
// ABOUTME: Verifies station creation and lookup
package manager

import (
	"testing"

	"github.com/harper/radio-metadata-proxy/internal/application/config"
)

func TestManager_NewFromConfig(t *testing.T) {
	cfg := &config.Config{
		Stations: []config.StationConfig{
			{
				ID: "test1",
				ICY: config.ICYConfig{
					Name:            "Test 1",
					MetaInt:         16384,
					BitrateHintKbps: 128,
				},
				Source: config.SourceConfig{
					URL:              "http://example.com/test1.mp3",
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
		},
	}

	mgr, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	if len(mgr.List()) != 1 {
		t.Errorf("expected 1 station, got %d", len(mgr.List()))
	}

	st := mgr.Get("test1")
	if st == nil {
		t.Fatal("expected to find test1 station")
	}

	if st.ID() != "test1" {
		t.Errorf("expected ID test1, got %s", st.ID())
	}
}
