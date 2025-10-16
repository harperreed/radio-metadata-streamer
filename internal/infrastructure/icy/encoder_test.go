// ABOUTME: Tests for ICY metadata block encoding
// ABOUTME: Verifies padding, length byte calculation, and truncation
package icy

import (
	"testing"
)

func TestBuildBlock_Empty(t *testing.T) {
	result := BuildBlock("")
	if len(result) != 1 || result[0] != 0x00 {
		t.Errorf("empty string should produce single zero byte, got %v", result)
	}
}

func TestBuildBlock_ShortString(t *testing.T) {
	result := BuildBlock("StreamTitle='Test';")

	// Should be: length byte + 19 chars + 13 padding = 33 bytes
	if len(result) != 33 {
		t.Errorf("expected 33 bytes, got %d", len(result))
	}

	// Length byte should be 2 (2 * 16 = 32 bytes of data+padding)
	if result[0] != 2 {
		t.Errorf("expected length byte 2, got %d", result[0])
	}

	// Verify content
	content := string(result[1:20])
	if content != "StreamTitle='Test';" {
		t.Errorf("expected 'StreamTitle='Test';', got %q", content)
	}

	// Verify padding is zeros
	for i := 20; i < 33; i++ {
		if result[i] != 0x00 {
			t.Errorf("byte %d should be 0x00, got 0x%02x", i, result[i])
		}
	}
}

func TestBuildBlock_Truncation(t *testing.T) {
	// Create string larger than max (255 * 16 = 4080 bytes)
	longStr := string(make([]byte, 5000))
	result := BuildBlock(longStr)

	// Length byte should be 255
	if result[0] != 255 {
		t.Errorf("expected length byte 255, got %d", result[0])
	}

	// Total length should be 1 + 255*16 = 4081 bytes
	expected := 1 + 255*16
	if len(result) != expected {
		t.Errorf("expected %d bytes, got %d", expected, len(result))
	}
}
