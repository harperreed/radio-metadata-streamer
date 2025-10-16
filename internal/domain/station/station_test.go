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
