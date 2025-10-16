// ABOUTME: Tests for station domain model
// ABOUTME: Verifies lifecycle, client management, and metadata updates
package station

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/harper/radio-metadata-proxy/internal/infrastructure/ring"
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

// Mock implementations for testing
type mockSource struct {
	data []byte
}

func (m *mockSource) Connect(ctx context.Context) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m.data)), nil
}

type mockMetadataProvider struct {
	meta string
}

func (m *mockMetadataProvider) Fetch(ctx context.Context) (string, error) {
	return m.meta, nil
}

func TestStation_Start(t *testing.T) {
	// Create test data
	testData := bytes.Repeat([]byte("test audio data "), 100)

	src := &mockSource{data: testData}
	meta := &mockMetadataProvider{meta: "StreamTitle='Test Song';"}
	buffer := ring.New(1024)

	cfg := Config{
		ID:             "test",
		MetaInt:        16384,
		PollInterval:   100 * time.Millisecond,
		RingBufferSize: 1024,
		ChunkBusCap:    32,
	}

	s := New(cfg, src, meta, buffer)

	// Start station goroutines
	err := s.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for data to be read and buffered
	time.Sleep(200 * time.Millisecond)

	// Check that ring buffer has data
	snapshot := buffer.Snapshot()
	if len(snapshot) == 0 {
		t.Error("expected ring buffer to contain data after Start")
	}

	// Check that metadata was polled
	if s.CurrentMetadata() != "StreamTitle='Test Song';" {
		t.Errorf("expected metadata to be polled, got %q", s.CurrentMetadata())
	}

	// Check that source is marked healthy
	if !s.SourceHealthy() {
		t.Error("expected source to be marked healthy after successful connection")
	}

	// Shutdown
	err = s.Shutdown()
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

func TestStation_Subscribe(t *testing.T) {
	testData := bytes.Repeat([]byte("test"), 50)

	src := &mockSource{data: testData}
	meta := &mockMetadataProvider{meta: "StreamTitle='Test';"}
	buffer := ring.New(1024)

	cfg := Config{
		ID:             "test",
		MetaInt:        16384,
		PollInterval:   100 * time.Millisecond,
		RingBufferSize: 1024,
		ChunkBusCap:    32,
	}

	s := New(cfg, src, meta, buffer)
	s.Start()
	defer s.Shutdown()

	// Subscribe to chunks
	client := &Client{ID: "test-client"}
	chunks := s.Subscribe(client)

	// Should receive chunks
	select {
	case chunk := <-chunks:
		if len(chunk) == 0 {
			t.Error("expected non-empty chunk")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for chunk")
	}

	// Unsubscribe
	s.Unsubscribe(client)

	// Should not receive more chunks after unsubscribe
	// (drain any buffered chunks first)
	for len(chunks) > 0 {
		<-chunks
	}
}
