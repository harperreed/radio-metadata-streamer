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
