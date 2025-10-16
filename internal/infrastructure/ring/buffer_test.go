// ABOUTME: Tests for circular ring buffer implementation
// ABOUTME: Verifies write, wrap-around, overflow, and snapshot operations
package ring

import (
	"testing"
)

func TestNew(t *testing.T) {
	buf := New(1024)
	if buf == nil {
		t.Fatal("New should return non-nil buffer")
	}

	snap := buf.Snapshot()
	if len(snap) != 0 {
		t.Errorf("new buffer should be empty, got %d bytes", len(snap))
	}
}

func TestWrite_Simple(t *testing.T) {
	buf := New(1024)
	data := []byte("hello")

	buf.Write(data)

	snap := buf.Snapshot()
	if string(snap) != "hello" {
		t.Errorf("expected 'hello', got %q", snap)
	}
}

func TestWrite_Overflow(t *testing.T) {
	buf := New(100)

	// Write 150 bytes - should trigger overflow
	data := make([]byte, 150)
	for i := range data {
		data[i] = byte(i)
	}

	buf.Write(data)

	snap := buf.Snapshot()

	// Should have dropped oldest 25 bytes, kept newest 100
	if len(snap) != 100 {
		t.Errorf("expected 100 bytes, got %d", len(snap))
	}

	// Should contain bytes 50-149 (dropped 0-49)
	for i, v := range snap {
		expected := byte(50 + i)
		if v != expected {
			t.Errorf("byte %d: expected %d, got %d", i, expected, v)
			break
		}
	}
}
