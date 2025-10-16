# Multi-Station ICY Metadata Proxy Implementation Plan

> **For Claude:** Use `${SUPERPOWERS_SKILLS_ROOT}/skills/collaboration/executing-plans/SKILL.md` to implement this plan task-by-task.

**Goal:** Build a multi-station ICY metadata proxy that streams MP3 audio with injected metadata for FIP and NTS radio.

**Architecture:** Domain-driven hexagonal architecture with three layers: domain (Station, interfaces), application (StationManager, config), and infrastructure (HTTP implementations, ICY encoding, ring buffer). Each station runs 3 goroutines: source reader, metadata poller, and per-client handlers. Uses channels for fan-out and atomic pointers for metadata.

**Tech Stack:** Go 1.23+, stdlib only, YAML config (gopkg.in/yaml.v3), Docker multi-stage build

---

## Task 1: Project Initialization

**Files:**
- Create: `go.mod`
- Create: `.gitignore`

**Step 1: Initialize Go module**

Run:
```bash
go mod init github.com/harper/radio-metadata-proxy
```

Expected: Creates `go.mod` with module declaration

**Step 2: Add YAML dependency**

Run:
```bash
go get gopkg.in/yaml.v3
```

Expected: Adds yaml.v3 to go.mod

**Step 3: Create .gitignore**

Create `.gitignore`:
```
# Binaries
icyproxy
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test artifacts
*.test
*.out

# Go workspace file
go.work

# IDE
.vscode/
.idea/
*.swp
*.swo
```

**Step 4: Commit**

Run:
```bash
git add go.mod go.sum .gitignore
git commit -m "feat: initialize Go project with yaml dependency"
```

---

## Task 2: ICY Block Encoder

**Files:**
- Create: `internal/infrastructure/icy/encoder.go`
- Create: `internal/infrastructure/icy/encoder_test.go`

**Step 1: Write failing test for empty metadata**

Create `internal/infrastructure/icy/encoder_test.go`:
```go
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
```

**Step 2: Run test to verify failure**

Run:
```bash
go test ./internal/infrastructure/icy/...
```

Expected: FAIL with "no Go files in internal/infrastructure/icy"

**Step 3: Write minimal implementation**

Create `internal/infrastructure/icy/encoder.go`:
```go
// ABOUTME: ICY metadata block encoding for Shoutcast/Icecast protocol
// ABOUTME: Handles 16-byte padding and length byte calculation per ICY spec
package icy

func BuildBlock(text string) []byte {
	if text == "" {
		return []byte{0x00}
	}
	return []byte{0x00}
}
```

**Step 4: Run test to verify pass**

Run:
```bash
go test ./internal/infrastructure/icy/...
```

Expected: PASS

**Step 5: Write failing test for non-empty metadata**

Add to `internal/infrastructure/icy/encoder_test.go`:
```go
func TestBuildBlock_ShortString(t *testing.T) {
	result := BuildBlock("StreamTitle='Test';")

	// Should be: length byte + 20 chars + 12 padding = 33 bytes
	if len(result) != 33 {
		t.Errorf("expected 33 bytes, got %d", len(result))
	}

	// Length byte should be 2 (2 * 16 = 32 bytes of data+padding)
	if result[0] != 2 {
		t.Errorf("expected length byte 2, got %d", result[0])
	}

	// Verify content
	content := string(result[1:21])
	if content != "StreamTitle='Test';" {
		t.Errorf("expected 'StreamTitle='Test';', got %q", content)
	}

	// Verify padding is zeros
	for i := 21; i < 33; i++ {
		if result[i] != 0x00 {
			t.Errorf("byte %d should be 0x00, got 0x%02x", i, result[i])
		}
	}
}
```

**Step 6: Run test to verify failure**

Run:
```bash
go test ./internal/infrastructure/icy/... -v
```

Expected: FAIL on length check

**Step 7: Implement full encoding logic**

Update `internal/infrastructure/icy/encoder.go`:
```go
// ABOUTME: ICY metadata block encoding for Shoutcast/Icecast protocol
// ABOUTME: Handles 16-byte padding and length byte calculation per ICY spec
package icy

import "bytes"

// BuildBlock encodes text as ICY metadata block with 16-byte padding.
// Returns length byte (count of 16-byte chunks) followed by padded payload.
// Max size: 255 * 16 = 4080 bytes.
func BuildBlock(text string) []byte {
	if text == "" {
		return []byte{0x00}
	}

	payload := []byte(text)

	// Truncate if exceeds max (255 blocks * 16 bytes)
	if len(payload) > 255*16 {
		payload = payload[:255*16]
	}

	// Calculate blocks (round up)
	blocks := (len(payload) + 15) / 16
	if blocks > 255 {
		blocks = 255
	}

	// Calculate padding
	pad := blocks*16 - len(payload)

	// Build result: length byte + payload + padding
	var buf bytes.Buffer
	buf.WriteByte(byte(blocks))
	buf.Write(payload)
	if pad > 0 {
		buf.Write(bytes.Repeat([]byte{0x00}, pad))
	}

	return buf.Bytes()
}
```

**Step 8: Run tests to verify pass**

Run:
```bash
go test ./internal/infrastructure/icy/... -v
```

Expected: PASS (2 tests)

**Step 9: Write test for truncation**

Add to `internal/infrastructure/icy/encoder_test.go`:
```go
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
```

**Step 10: Run tests to verify pass**

Run:
```bash
go test ./internal/infrastructure/icy/... -v
```

Expected: PASS (3 tests)

**Step 11: Commit**

Run:
```bash
git add internal/infrastructure/icy/
git commit -m "feat: implement ICY metadata block encoding with tests"
```

---

## Task 3: Ring Buffer

**Files:**
- Create: `internal/infrastructure/ring/buffer.go`
- Create: `internal/infrastructure/ring/buffer_test.go`

**Step 1: Write failing test for ring buffer creation**

Create `internal/infrastructure/ring/buffer_test.go`:
```go
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
```

**Step 2: Run test to verify failure**

Run:
```bash
go test ./internal/infrastructure/ring/...
```

Expected: FAIL with "no Go files"

**Step 3: Write minimal implementation**

Create `internal/infrastructure/ring/buffer.go`:
```go
// ABOUTME: Circular ring buffer for audio stream smoothing
// ABOUTME: Drops oldest data on overflow to maintain fixed size
package ring

import "sync"

type Buffer struct {
	buf []byte
	w   int  // write position
	n   int  // bytes stored
	mu  sync.Mutex
}

func New(size int) *Buffer {
	return &Buffer{buf: make([]byte, size)}
}

func (b *Buffer) Snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return make([]byte, b.n)
}
```

**Step 4: Run test to verify pass**

Run:
```bash
go test ./internal/infrastructure/ring/... -v
```

Expected: PASS

**Step 5: Write failing test for simple write**

Add to `internal/infrastructure/ring/buffer_test.go`:
```go
func TestWrite_Simple(t *testing.T) {
	buf := New(1024)
	data := []byte("hello")

	buf.Write(data)

	snap := buf.Snapshot()
	if string(snap) != "hello" {
		t.Errorf("expected 'hello', got %q", snap)
	}
}
```

**Step 6: Run test to verify failure**

Run:
```bash
go test ./internal/infrastructure/ring/... -v
```

Expected: FAIL on snapshot comparison

**Step 7: Implement Write method**

Add to `internal/infrastructure/ring/buffer.go`:
```go
func (b *Buffer) Write(p []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for len(p) > 0 {
		space := len(b.buf) - b.n
		if space == 0 {
			// Drop oldest 25%
			drop := len(b.buf) / 4
			b.w = (b.w + drop) % len(b.buf)
			b.n -= drop
			if b.n < 0 {
				b.n = 0
			}
			space = len(b.buf) - b.n
		}

		chunk := len(p)
		if chunk > space {
			chunk = space
		}

		end := (b.w + b.n) % len(b.buf)
		right := len(b.buf) - end
		if right > chunk {
			right = chunk
		}

		copy(b.buf[end:end+right], p[:right])
		if right < chunk {
			copy(b.buf[0:chunk-right], p[right:chunk])
		}

		b.n += chunk
		p = p[chunk:]
	}
}
```

**Step 8: Update Snapshot to return actual data**

Update `Snapshot` in `internal/infrastructure/ring/buffer.go`:
```go
func (b *Buffer) Snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]byte, b.n)
	if b.n == 0 {
		return out
	}

	head := b.w
	tail := (b.w + b.n) % len(b.buf)

	if head < tail {
		copy(out, b.buf[head:tail])
	} else {
		copy(out, b.buf[head:])
		copy(out[len(b.buf)-head:], b.buf[:tail])
	}

	return out
}
```

**Step 9: Run tests to verify pass**

Run:
```bash
go test ./internal/infrastructure/ring/... -v
```

Expected: PASS (2 tests)

**Step 10: Write test for overflow behavior**

Add to `internal/infrastructure/ring/buffer_test.go`:
```go
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
```

**Step 11: Run tests to verify pass**

Run:
```bash
go test ./internal/infrastructure/ring/... -v
```

Expected: PASS (3 tests)

**Step 12: Commit**

Run:
```bash
git add internal/infrastructure/ring/
git commit -m "feat: implement ring buffer with overflow handling"
```

---

## Task 4: Domain Interfaces

**Files:**
- Create: `internal/domain/interfaces.go`

**Step 1: Create domain interfaces**

Create `internal/domain/interfaces.go`:
```go
// ABOUTME: Domain interfaces for dependency inversion
// ABOUTME: Allows station to depend on abstractions, not concrete implementations
package domain

import (
	"context"
	"io"
)

// StreamSource provides MP3 audio stream bytes
type StreamSource interface {
	Connect(ctx context.Context) (io.ReadCloser, error)
}

// MetadataProvider fetches current track metadata
type MetadataProvider interface {
	Fetch(ctx context.Context) (string, error)
}
```

**Step 2: Commit**

Run:
```bash
git add internal/domain/interfaces.go
git commit -m "feat: add domain interfaces for stream source and metadata provider"
```

---

## Task 5: Station Domain Model (Part 1 - Structure)

**Files:**
- Create: `internal/domain/station/station.go`
- Create: `internal/domain/station/station_test.go`

**Step 1: Write failing test for station creation**

Create `internal/domain/station/station_test.go`:
```go
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
```

**Step 2: Run test to verify failure**

Run:
```bash
go test ./internal/domain/station/...
```

Expected: FAIL with "no Go files"

**Step 3: Write minimal station structure**

Create `internal/domain/station/station.go`:
```go
// ABOUTME: Station domain model coordinating streaming, metadata, and clients
// ABOUTME: Manages lifecycle of goroutines for source reading and metadata polling
package station

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/harper/radio-metadata-proxy/internal/domain"
	"github.com/harper/radio-metadata-proxy/internal/infrastructure/ring"
)

type Config struct {
	ID             string
	ICYName        string
	MetaInt        int
	BitrateHint    int
	PollInterval   time.Duration
	RingBufferSize int
	ChunkBusCap    int
}

type Station struct {
	id          string
	icyName     string
	metaInt     int
	bitrateHint int

	source   domain.StreamSource
	metadata domain.MetadataProvider
	buffer   *ring.Buffer

	pollInterval time.Duration

	currentMeta   atomic.Pointer[string]
	lastMetaAt    atomic.Pointer[time.Time]
	sourceHealthy atomic.Bool

	clients   map[*Client]struct{}
	clientsMu sync.Mutex

	chunkBus chan []byte

	ctx    context.Context
	cancel context.CancelFunc
}

type Client struct {
	ID string
}

func New(cfg Config, source domain.StreamSource, metadata domain.MetadataProvider, buffer *ring.Buffer) *Station {
	return &Station{
		id:           cfg.ID,
		icyName:      cfg.ICYName,
		metaInt:      cfg.MetaInt,
		bitrateHint:  cfg.BitrateHint,
		source:       source,
		metadata:     metadata,
		buffer:       buffer,
		pollInterval: cfg.PollInterval,
		clients:      make(map[*Client]struct{}),
		chunkBus:     make(chan []byte, cfg.ChunkBusCap),
	}
}

func (s *Station) ID() string {
	return s.id
}
```

**Step 4: Run test to verify pass**

Run:
```bash
go test ./internal/domain/station/... -v
```

Expected: PASS

**Step 5: Commit**

Run:
```bash
git add internal/domain/station/
git commit -m "feat: add station domain model structure"
```

---

## Task 6: Station Domain Model (Part 2 - Metadata)

**Files:**
- Modify: `internal/domain/station/station.go`
- Modify: `internal/domain/station/station_test.go`

**Step 1: Write failing test for metadata access**

Add to `internal/domain/station/station_test.go`:
```go
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
```

**Step 2: Run test to verify failure**

Run:
```bash
go test ./internal/domain/station/... -v
```

Expected: FAIL with "undefined: Station.CurrentMetadata"

**Step 3: Implement metadata methods**

Add to `internal/domain/station/station.go`:
```go
func (s *Station) CurrentMetadata() string {
	p := s.currentMeta.Load()
	if p == nil {
		return ""
	}
	return *p
}

func (s *Station) UpdateMetadata(meta string) {
	s.currentMeta.Store(&meta)
	now := time.Now()
	s.lastMetaAt.Store(&now)
}

func (s *Station) LastMetadataUpdate() *time.Time {
	return s.lastMetaAt.Load()
}
```

**Step 4: Run tests to verify pass**

Run:
```bash
go test ./internal/domain/station/... -v
```

Expected: PASS (2 tests)

**Step 5: Commit**

Run:
```bash
git add internal/domain/station/
git commit -m "feat: add metadata management to station"
```

---

## Task 7: Station Domain Model (Part 3 - Client Management)

**Files:**
- Modify: `internal/domain/station/station.go`
- Modify: `internal/domain/station/station_test.go`

**Step 1: Write failing test for client management**

Add to `internal/domain/station/station_test.go`:
```go
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
```

**Step 2: Run test to verify failure**

Run:
```bash
go test ./internal/domain/station/... -v
```

Expected: FAIL with "undefined: Station.ClientCount"

**Step 3: Implement client management methods**

Add to `internal/domain/station/station.go`:
```go
func (s *Station) AddClient(c *Client) {
	s.clientsMu.Lock()
	s.clients[c] = struct{}{}
	s.clientsMu.Unlock()
}

func (s *Station) RemoveClient(c *Client) {
	s.clientsMu.Lock()
	delete(s.clients, c)
	s.clientsMu.Unlock()
}

func (s *Station) ClientCount() int {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	return len(s.clients)
}
```

**Step 4: Run tests to verify pass**

Run:
```bash
go test ./internal/domain/station/... -v
```

Expected: PASS (3 tests)

**Step 5: Commit**

Run:
```bash
git add internal/domain/station/
git commit -m "feat: add client management to station"
```

---

## Task 8: Station Domain Model (Part 4 - Health & Properties)

**Files:**
- Modify: `internal/domain/station/station.go`
- Modify: `internal/domain/station/station_test.go`

**Step 1: Write failing test for health and properties**

Add to `internal/domain/station/station_test.go`:
```go
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
```

**Step 2: Run test to verify failure**

Run:
```bash
go test ./internal/domain/station/... -v
```

Expected: FAIL with "undefined: Station.ICYName"

**Step 3: Implement property accessors**

Add to `internal/domain/station/station.go`:
```go
func (s *Station) ICYName() string {
	return s.icyName
}

func (s *Station) MetaInt() int {
	return s.metaInt
}

func (s *Station) BitrateHint() int {
	return s.bitrateHint
}

func (s *Station) SourceHealthy() bool {
	return s.sourceHealthy.Load()
}

func (s *Station) SetSourceHealthy(healthy bool) {
	s.sourceHealthy.Store(healthy)
}
```

**Step 4: Run tests to verify pass**

Run:
```bash
go test ./internal/domain/station/... -v
```

Expected: PASS (4 tests)

**Step 5: Commit**

Run:
```bash
git add internal/domain/station/
git commit -m "feat: add health status and property accessors to station"
```

---

## Task 9: HTTP Stream Source Implementation

**Files:**
- Create: `internal/infrastructure/source/http.go`
- Create: `internal/infrastructure/source/http_test.go`

**Step 1: Write failing test for HTTP stream source**

Create `internal/infrastructure/source/http_test.go`:
```go
// ABOUTME: Tests for HTTP stream source implementation
// ABOUTME: Verifies connection handling and error cases
package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPSource_Connect(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check headers
		if r.Header.Get("Icy-MetaData") != "0" {
			t.Errorf("expected Icy-MetaData: 0 header")
		}

		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("audio data"))
	}))
	defer server.Close()

	cfg := HTTPConfig{
		URL:            server.URL,
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    10 * time.Second,
	}

	src := NewHTTP(cfg)

	ctx := context.Background()
	reader, err := src.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer reader.Close()

	buf := make([]byte, 10)
	n, _ := reader.Read(buf)

	if string(buf[:n]) != "audio data" {
		t.Errorf("expected 'audio data', got %q", buf[:n])
	}
}
```

**Step 2: Run test to verify failure**

Run:
```bash
go test ./internal/infrastructure/source/...
```

Expected: FAIL with "no Go files"

**Step 3: Write HTTP stream source implementation**

Create `internal/infrastructure/source/http.go`:
```go
// ABOUTME: HTTP stream source implementation for MP3 audio
// ABOUTME: Handles upstream connection with timeouts and proper headers
package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HTTPConfig struct {
	URL            string
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	Headers        map[string]string
}

type HTTPSource struct {
	cfg    HTTPConfig
	client *http.Client
}

func NewHTTP(cfg HTTPConfig) *HTTPSource {
	transport := &http.Transport{
		DisableCompression:    true,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   0, // No total timeout for streaming
	}

	return &HTTPSource{
		cfg:    cfg,
		client: client,
	}
}

func (h *HTTPSource) Connect(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", h.cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set ICY headers
	req.Header.Set("Icy-MetaData", "0")

	// Set custom headers
	for k, v := range h.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return resp.Body, nil
}
```

**Step 4: Run tests to verify pass**

Run:
```bash
go test ./internal/infrastructure/source/... -v
```

Expected: PASS

**Step 5: Commit**

Run:
```bash
git add internal/infrastructure/source/
git commit -m "feat: implement HTTP stream source"
```

---

## Task 10: HTTP Metadata Provider Implementation

**Files:**
- Create: `internal/infrastructure/metadata/http.go`
- Create: `internal/infrastructure/metadata/http_test.go`

**Step 1: Write failing test for HTTP metadata provider**

Create `internal/infrastructure/metadata/http_test.go`:
```go
// ABOUTME: Tests for HTTP metadata provider implementation
// ABOUTME: Verifies JSON parsing and ICY formatting
package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPProvider_Fetch_JSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"artist":"Test Artist","title":"Test Song"}`))
	}))
	defer server.Close()

	cfg := HTTPConfig{
		URL:     server.URL,
		Timeout: 5 * time.Second,
		Build: BuildConfig{
			Format: "StreamTitle='{artist} - {title}';",
		},
	}

	provider := NewHTTP(cfg)

	ctx := context.Background()
	result, err := provider.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	expected := "StreamTitle='Test Artist - Test Song';"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
```

**Step 2: Run test to verify failure**

Run:
```bash
go test ./internal/infrastructure/metadata/...
```

Expected: FAIL with "no Go files"

**Step 3: Write HTTP metadata provider implementation**

Create `internal/infrastructure/metadata/http.go`:
```go
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

	// Build ICY string from format template
	result := h.cfg.Build.Format

	// Simple placeholder replacement
	result = strings.ReplaceAll(result, "{artist}", getString(data, "artist"))
	result = strings.ReplaceAll(result, "{title}", getString(data, "title"))

	// Apply transformations
	if h.cfg.Build.StripSingleQuotes {
		result = strings.ReplaceAll(result, "'", "")
	}

	if h.cfg.Build.NormalizeWhitespace {
		result = strings.Join(strings.Fields(result), " ")
	}

	return result, nil
}

func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}
```

**Step 4: Run tests to verify pass**

Run:
```bash
go test ./internal/infrastructure/metadata/... -v
```

Expected: PASS

**Step 5: Commit**

Run:
```bash
git add internal/infrastructure/metadata/
git commit -m "feat: implement HTTP metadata provider"
```

---

## Task 11: Config Parsing

**Files:**
- Create: `internal/application/config/config.go`
- Create: `internal/application/config/config_test.go`

**Step 1: Write failing test for config parsing**

Create `internal/application/config/config_test.go`:
```go
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
```

**Step 2: Run test to verify failure**

Run:
```bash
go test ./internal/application/config/...
```

Expected: FAIL with "no Go files"

**Step 3: Write config implementation**

Create `internal/application/config/config.go`:
```go
// ABOUTME: YAML configuration parsing and validation
// ABOUTME: Defines structure for multi-station proxy configuration
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen   ListenConfig     `yaml:"listen"`
	Stations []StationConfig  `yaml:"stations"`
	Logging  LoggingConfig    `yaml:"logging"`
}

type ListenConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type StationConfig struct {
	ID        string          `yaml:"id"`
	ICY       ICYConfig       `yaml:"icy"`
	Source    SourceConfig    `yaml:"source"`
	Metadata  MetadataConfig  `yaml:"metadata"`
	Buffering BufferingConfig `yaml:"buffering"`
}

type ICYConfig struct {
	Name          string `yaml:"name"`
	MetaInt       int    `yaml:"metaint"`
	BitrateHintKbps int  `yaml:"bitrate_hint_kbps"`
}

type SourceConfig struct {
	URL              string            `yaml:"url"`
	RequestHeaders   map[string]string `yaml:"request_headers"`
	ConnectTimeoutMs int               `yaml:"connect_timeout_ms"`
	ReadTimeoutMs    int               `yaml:"read_timeout_ms"`
}

type MetadataConfig struct {
	URL    string      `yaml:"url"`
	PollMs int         `yaml:"poll_ms"`
	Build  BuildConfig `yaml:"build"`
}

type BuildConfig struct {
	Format              string   `yaml:"format"`
	StripSingleQuotes   bool     `yaml:"strip_single_quotes"`
	Encoding            string   `yaml:"encoding"`
	NormalizeWhitespace bool     `yaml:"normalize_whitespace"`
	FallbackKeyOrder    []string `yaml:"fallback_key_order"`
}

type BufferingConfig struct {
	RingBytes            int `yaml:"ring_bytes"`
	ClientPendingMaxBytes int `yaml:"client_pending_max_bytes"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
	JSON  bool   `yaml:"json"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	return &cfg, nil
}
```

**Step 4: Run tests to verify pass**

Run:
```bash
go test ./internal/application/config/... -v
```

Expected: PASS

**Step 5: Commit**

Run:
```bash
git add internal/application/config/
git commit -m "feat: implement YAML config parsing"
```

---

## Task 12: Station Manager

**Files:**
- Create: `internal/application/manager/manager.go`
- Create: `internal/application/manager/manager_test.go`

**Step 1: Write failing test for station manager**

Create `internal/application/manager/manager_test.go`:
```go
// ABOUTME: Tests for station manager lifecycle
// ABOUTME: Verifies station creation and lookup
package manager

import (
	"testing"
	"time"

	"github.com/harper/radio-metadata-proxy/internal/application/config"
	"github.com/harper/radio-metadata-proxy/internal/domain/station"
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
```

**Step 2: Run test to verify failure**

Run:
```bash
go test ./internal/application/manager/...
```

Expected: FAIL with "no Go files"

**Step 3: Write station manager implementation**

Create `internal/application/manager/manager.go`:
```go
// ABOUTME: Station manager for lifecycle and lookup
// ABOUTME: Creates stations from config and manages their goroutines
package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/harper/radio-metadata-proxy/internal/application/config"
	"github.com/harper/radio-metadata-proxy/internal/domain/station"
	"github.com/harper/radio-metadata-proxy/internal/infrastructure/metadata"
	"github.com/harper/radio-metadata-proxy/internal/infrastructure/ring"
	"github.com/harper/radio-metadata-proxy/internal/infrastructure/source"
)

type Manager struct {
	stations map[string]*station.Station
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewFromConfig(cfg *config.Config) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	mgr := &Manager{
		stations: make(map[string]*station.Station),
		ctx:      ctx,
		cancel:   cancel,
	}

	for _, stCfg := range cfg.Stations {
		// Create dependencies
		srcCfg := source.HTTPConfig{
			URL:            stCfg.Source.URL,
			ConnectTimeout: time.Duration(stCfg.Source.ConnectTimeoutMs) * time.Millisecond,
			ReadTimeout:    time.Duration(stCfg.Source.ReadTimeoutMs) * time.Millisecond,
			Headers:        stCfg.Source.RequestHeaders,
		}
		src := source.NewHTTP(srcCfg)

		metaCfg := metadata.HTTPConfig{
			URL:     stCfg.Metadata.URL,
			Timeout: time.Duration(stCfg.Metadata.PollMs) * time.Millisecond,
			Build: metadata.BuildConfig{
				Format:              stCfg.Metadata.Build.Format,
				StripSingleQuotes:   stCfg.Metadata.Build.StripSingleQuotes,
				NormalizeWhitespace: stCfg.Metadata.Build.NormalizeWhitespace,
				FallbackKeyOrder:    stCfg.Metadata.Build.FallbackKeyOrder,
			},
		}
		metaProv := metadata.NewHTTP(metaCfg)

		buffer := ring.New(stCfg.Buffering.RingBytes)

		// Create station
		stationCfg := station.Config{
			ID:             stCfg.ID,
			ICYName:        stCfg.ICY.Name,
			MetaInt:        stCfg.ICY.MetaInt,
			BitrateHint:    stCfg.ICY.BitrateHintKbps,
			PollInterval:   time.Duration(stCfg.Metadata.PollMs) * time.Millisecond,
			RingBufferSize: stCfg.Buffering.RingBytes,
			ChunkBusCap:    32,
		}

		st := station.New(stationCfg, src, metaProv, buffer)

		mgr.stations[stCfg.ID] = st
	}

	return mgr, nil
}

func (m *Manager) Get(id string) *station.Station {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stations[id]
}

func (m *Manager) List() []*station.Station {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*station.Station, 0, len(m.stations))
	for _, st := range m.stations {
		result = append(result, st)
	}
	return result
}

func (m *Manager) Start() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, st := range m.stations {
		// TODO: Start station goroutines when implemented
		_ = st
	}

	return nil
}

func (m *Manager) Shutdown() error {
	m.cancel()
	m.wg.Wait()
	return nil
}
```

**Step 4: Run tests to verify pass**

Run:
```bash
go test ./internal/application/manager/... -v
```

Expected: PASS

**Step 5: Commit**

Run:
```bash
git add internal/application/manager/
git commit -m "feat: implement station manager"
```

---

## Task 13: HTTP Handlers (Part 1 - Stream Handler)

**Files:**
- Create: `internal/infrastructure/http/handlers.go`
- Create: `internal/infrastructure/http/handlers_test.go`

**Step 1: Write basic test for stream handler structure**

Create `internal/infrastructure/http/handlers_test.go`:
```go
// ABOUTME: Tests for HTTP handlers
// ABOUTME: Verifies routing, headers, and response formats
package http

import (
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
```

**Step 2: Run test to verify failure**

Run:
```bash
go test ./internal/infrastructure/http/...
```

Expected: FAIL with "no Go files"

**Step 3: Write stream handler implementation**

Create `internal/infrastructure/http/handlers.go`:
```go
// ABOUTME: HTTP handlers for station endpoints
// ABOUTME: Implements stream, metadata, and health check routes
package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/harper/radio-metadata-proxy/internal/application/manager"
)

type StreamHandler struct {
	mgr *manager.Manager
}

func NewStreamHandler(mgr *manager.Manager) *StreamHandler {
	return &StreamHandler{mgr: mgr}
}

func (h *StreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract station ID from path: /{station}/stream
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 2 || parts[1] != "stream" {
		http.NotFound(w, r)
		return
	}

	stationID := parts[0]
	st := h.mgr.Get(stationID)
	if st == nil {
		http.NotFound(w, r)
		return
	}

	// Set ICY headers
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("icy-name", st.ICYName())
	w.Header().Set("icy-br", fmt.Sprintf("%d", st.BitrateHint()))
	w.Header().Set("icy-metaint", fmt.Sprintf("%d", st.MetaInt()))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "close")

	// TODO: Implement actual streaming when station goroutines are ready
	w.WriteHeader(http.StatusOK)
}

type MetaHandler struct {
	mgr *manager.Manager
}

func NewMetaHandler(mgr *manager.Manager) *MetaHandler {
	return &MetaHandler{mgr: mgr}
}

func (h *MetaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 2 || parts[1] != "meta" {
		http.NotFound(w, r)
		return
	}

	stationID := parts[0]
	st := h.mgr.Get(stationID)
	if st == nil {
		http.NotFound(w, r)
		return
	}

	type response struct {
		Current      string  `json:"current"`
		UpdatedAt    *string `json:"updated_at,omitempty"`
		SourceHealthy bool   `json:"sourceHealthy"`
	}

	var updatedAt *string
	if t := st.LastMetadataUpdate(); t != nil {
		s := t.Format("2006-01-02T15:04:05Z07:00")
		updatedAt = &s
	}

	resp := response{
		Current:      st.CurrentMetadata(),
		UpdatedAt:    updatedAt,
		SourceHealthy: st.SourceHealthy(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type StationsHandler struct {
	mgr *manager.Manager
}

func NewStationsHandler(mgr *manager.Manager) *StationsHandler {
	return &StationsHandler{mgr: mgr}
}

func (h *StationsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	type stationInfo struct {
		ID            string `json:"id"`
		StreamURL     string `json:"stream_url"`
		MetaURL       string `json:"meta_url"`
		Clients       int    `json:"clients"`
		SourceHealthy bool   `json:"sourceHealthy"`
	}

	stations := h.mgr.List()
	result := make([]stationInfo, 0, len(stations))

	for _, st := range stations {
		result = append(result, stationInfo{
			ID:            st.ID(),
			StreamURL:     fmt.Sprintf("/%s/stream", st.ID()),
			MetaURL:       fmt.Sprintf("/%s/meta", st.ID()),
			Clients:       st.ClientCount(),
			SourceHealthy: st.SourceHealthy(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func HealthzHandler(w http.ResponseWriter, r *http.Request) {
	type response struct {
		OK bool `json:"ok"`
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response{OK: true})
}
```

**Step 4: Run tests to verify pass**

Run:
```bash
go test ./internal/infrastructure/http/... -v
```

Expected: PASS

**Step 5: Commit**

Run:
```bash
git add internal/infrastructure/http/
git commit -m "feat: implement HTTP handlers for streaming and metadata"
```

---

## Task 14: Main Entry Point

**Files:**
- Create: `cmd/icyproxy/main.go`

**Step 1: Write main.go**

Create `cmd/icyproxy/main.go`:
```go
// ABOUTME: Main entry point for ICY metadata proxy
// ABOUTME: Loads config, starts stations, runs HTTP server
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/harper/radio-metadata-proxy/internal/application/config"
	"github.com/harper/radio-metadata-proxy/internal/application/manager"
	"github.com/harper/radio-metadata-proxy/internal/infrastructure/http"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	// Load config
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Create station manager
	mgr, err := manager.NewFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("create manager: %w", err)
	}

	// Start stations
	if err := mgr.Start(); err != nil {
		return fmt.Errorf("start stations: %w", err)
	}

	// Setup HTTP routes
	mux := nethttp.NewServeMux()
	mux.Handle("/stations", http.NewStationsHandler(mgr))
	mux.HandleFunc("/healthz", http.HealthzHandler)

	// Station-specific routes
	streamHandler := http.NewStreamHandler(mgr)
	metaHandler := http.NewMetaHandler(mgr)

	mux.HandleFunc("/", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if len(r.URL.Path) > 1 && r.URL.Path[len(r.URL.Path)-7:] == "/stream" {
			streamHandler.ServeHTTP(w, r)
			return
		}
		if len(r.URL.Path) > 1 && r.URL.Path[len(r.URL.Path)-5:] == "/meta" {
			metaHandler.ServeHTTP(w, r)
			return
		}
		nethttp.NotFound(w, r)
	})

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Listen.Host, cfg.Listen.Port)
	srv := &nethttp.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // Streaming
		IdleTimeout:  0, // Streaming
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	// Graceful shutdown
	shutdown := make(chan error, 1)
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		log.Println("shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		shutdown <- srv.Shutdown(ctx)
	}()

	// Start server
	log.Printf("listening on http://%s (try /stations)", addr)
	if err := srv.ListenAndServe(); err != nil && err != nethttp.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}

	// Wait for shutdown
	if err := <-shutdown; err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	// Shutdown stations
	if err := mgr.Shutdown(); err != nil {
		return fmt.Errorf("shutdown stations: %w", err)
	}

	log.Println("shutdown complete")
	return nil
}
```

**Step 2: Commit**

Run:
```bash
git add cmd/icyproxy/
git commit -m "feat: add main entry point with HTTP server"
```

---

## Task 15: Example Config

**Files:**
- Create: `configs/example.yaml`

**Step 1: Write example config**

Create `configs/example.yaml`:
```yaml
listen:
  host: 0.0.0.0
  port: 8000

stations:
  - id: "fip"
    icy:
      name: "FIP (proxy)"
      metaint: 16384
      bitrate_hint_kbps: 128
    source:
      url: "https://icecast.radiofrance.fr/fip-hifi.aac"
      request_headers:
        Icy-MetaData: "0"
      connect_timeout_ms: 5000
      read_timeout_ms: 15000
    metadata:
      url: "https://fip-metadata.fly.dev/"
      poll_ms: 3000
      build:
        format: "StreamTitle='{artist} - {title}';"
        strip_single_quotes: true
        normalize_whitespace: true
    buffering:
      ring_bytes: 262144

  - id: "nts"
    icy:
      name: "NTS Radio 1 (proxy)"
      metaint: 16384
      bitrate_hint_kbps: 128
    source:
      url: "https://stream-mixtape-geo.ntslive.net/stream"
      request_headers:
        Icy-MetaData: "0"
      connect_timeout_ms: 5000
      read_timeout_ms: 15000
    metadata:
      url: "https://www.nts.live/api/v2/live"
      poll_ms: 5000
      build:
        format: "StreamTitle='{artist} - {title}';"
        strip_single_quotes: true
        normalize_whitespace: true
    buffering:
      ring_bytes: 262144

logging:
  level: info
  json: false
```

**Step 2: Commit**

Run:
```bash
git add configs/
git commit -m "feat: add example configuration for FIP and NTS"
```

---

## Task 16: Dockerfile

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

**Step 1: Write Dockerfile**

Create `Dockerfile`:
```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o icyproxy ./cmd/icyproxy

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/icyproxy .
COPY configs/example.yaml /etc/icyproxy/config.yaml

EXPOSE 8000

CMD ["./icyproxy", "/etc/icyproxy/config.yaml"]
```

**Step 2: Write .dockerignore**

Create `.dockerignore`:
```
.git
.gitignore
README.md
docs/
*.md
.vscode
.idea
```

**Step 3: Commit**

Run:
```bash
git add Dockerfile .dockerignore
git commit -m "feat: add Dockerfile for multi-stage build"
```

---

## Task 17: README

**Files:**
- Modify: `README.md`

**Step 1: Write README**

Update `README.md`:
```markdown
# Radio Metadata Proxy

Multi-station ICY metadata proxy for streaming radio with custom metadata injection.

## Features

- Multiple stations from single daemon
- ICY metadata injection (Shoutcast/Icecast compatible)
- Ring buffer for stream smoothing
- Automatic reconnection with backoff
- Clean hexagonal architecture

## Quick Start

### Binary

```bash
go build -o icyproxy ./cmd/icyproxy
./icyproxy configs/example.yaml
```

### Docker

```bash
docker build -t icyproxy .
docker run -p 8000:8000 -v ./myconfig.yaml:/etc/icyproxy/config.yaml icyproxy
```

## Usage

### Endpoints

- `GET /{station}/stream` - ICY stream
- `GET /{station}/meta` - JSON metadata
- `GET /stations` - List all stations
- `GET /healthz` - Health check

### Example

```bash
# List stations
curl http://localhost:8000/stations

# Stream audio (VLC, mpv, etc)
mpv http://localhost:8000/fip/stream

# Get metadata
curl http://localhost:8000/fip/meta
```

## Configuration

See `configs/example.yaml` for full configuration options.

## Architecture

- **Domain Layer**: Station model, interfaces
- **Application Layer**: Manager, config
- **Infrastructure Layer**: HTTP implementations, ICY encoding, ring buffer

## License

MIT
```

**Step 2: Commit**

Run:
```bash
git add README.md
git commit -m "docs: add README with usage instructions"
```

---

## Next Steps

**All v1 minimal features are now implemented!**

What's NOT yet implemented (but designed for):
- Station goroutines (source reader, metadata poller) - need to add Start/Stop methods
- Actual streaming with ICY injection in client handlers
- Client backpressure management

**To complete v1, you'll need to:**
1. Implement `Station.Start()` method with 3 goroutines
2. Implement actual streaming in `StreamHandler`
3. Add client writer with ICY injection logic
4. Test with real audio streams

These are the core runtime pieces that tie everything together. The architecture is ready!
