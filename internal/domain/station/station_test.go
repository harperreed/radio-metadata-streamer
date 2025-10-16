// ABOUTME: Tests for station domain model
// ABOUTME: Verifies lifecycle, client management, and metadata updates
package station

import (
	"testing"
)

func TestNew(t *testing.T) {
	cfg := Config{
		ID:      "test",
		MetaInt: 16384,
	}

	s := New(cfg, nil, nil, nil)

	if s.ID() != "test" {
		t.Errorf("expected ID 'test', got %q", s.ID())
	}
}

func TestStation_CurrentMetadata(t *testing.T) {
	cfg := Config{
		ID:      "test",
		MetaInt: 16384,
	}

	s := New(cfg, nil, nil, nil)

	// Initially empty
	if meta := s.CurrentMetadata(); meta != "" {
		t.Errorf("expected empty metadata, got %q", meta)
	}

	// Set metadata
	s.UpdateMetadata("StreamTitle='Test';")

	if meta := s.CurrentMetadata(); meta != "StreamTitle='Test';" {
		t.Errorf("expected 'StreamTitle='Test';', got %q", meta)
	}
}

func TestStation_ClientManagement(t *testing.T) {
	cfg := Config{
		ID:      "test",
		MetaInt: 16384,
	}

	s := New(cfg, nil, nil, nil)

	if count := s.ClientCount(); count != 0 {
		t.Errorf("expected 0 clients, got %d", count)
	}

	client := &Client{ID: "c1"}
	s.AddClient(client)

	if count := s.ClientCount(); count != 1 {
		t.Errorf("expected 1 client, got %d", count)
	}

	s.RemoveClient(client)

	if count := s.ClientCount(); count != 0 {
		t.Errorf("expected 0 clients after removal, got %d", count)
	}
}

func TestStation_Properties(t *testing.T) {
	cfg := Config{
		ID:          "fip",
		ICYName:     "FIP Radio",
		MetaInt:     16384,
		BitrateHint: 128,
	}

	s := New(cfg, nil, nil, nil)

	if s.ICYName() != "FIP Radio" {
		t.Errorf("expected ICYName 'FIP Radio', got %q", s.ICYName())
	}

	if s.MetaInt() != 16384 {
		t.Errorf("expected MetaInt 16384, got %d", s.MetaInt())
	}

	if s.BitrateHint() != 128 {
		t.Errorf("expected BitrateHint 128, got %d", s.BitrateHint())
	}

	if s.SourceHealthy() {
		t.Error("expected SourceHealthy false initially")
	}

	s.SetSourceHealthy(true)

	if !s.SourceHealthy() {
		t.Error("expected SourceHealthy true after set")
	}
}
