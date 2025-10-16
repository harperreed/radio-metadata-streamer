// ABOUTME: Station domain model coordinating streaming, metadata, and clients
// ABOUTME: Manages lifecycle of goroutines for source reading and metadata polling
package station

import (
	"context"
	"io"
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
	ch chan []byte
}

func New(cfg Config, source domain.StreamSource, metadata domain.MetadataProvider, buffer *ring.Buffer) *Station {
	ctx, cancel := context.WithCancel(context.Background())
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
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (s *Station) ID() string {
	return s.id
}

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

func (s *Station) Subscribe(c *Client) <-chan []byte {
	c.ch = make(chan []byte, 64)
	s.AddClient(c)
	return c.ch
}

func (s *Station) Unsubscribe(c *Client) {
	s.RemoveClient(c)
	if c.ch != nil {
		close(c.ch)
		c.ch = nil
	}
}

func (s *Station) Start() error {
	// Start source reader goroutine
	go s.runSourceReader()

	// Start metadata poller goroutine
	go s.runMetadataPoller()

	// Start fan-out goroutine
	go s.runFanOut()

	return nil
}

func (s *Station) Shutdown() error {
	s.cancel()
	return nil
}

func (s *Station) runSourceReader() {
	stream, err := s.source.Connect(s.ctx)
	if err != nil {
		s.SetSourceHealthy(false)
		return
	}
	defer stream.Close()

	s.SetSourceHealthy(true)

	buf := make([]byte, 8192)
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		n, err := stream.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			// Write to ring buffer
			s.buffer.Write(chunk)

			// Send to fan-out
			select {
			case s.chunkBus <- chunk:
			case <-s.ctx.Done():
				return
			}
		}

		if err != nil {
			if err != io.EOF {
				s.SetSourceHealthy(false)
			}
			return
		}
	}
}

func (s *Station) runMetadataPoller() {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	// Poll immediately on start
	if meta, err := s.metadata.Fetch(s.ctx); err == nil {
		s.UpdateMetadata(meta)
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if meta, err := s.metadata.Fetch(s.ctx); err == nil {
				s.UpdateMetadata(meta)
			}
		}
	}
}

func (s *Station) runFanOut() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case chunk := <-s.chunkBus:
			// Distribute chunk to all subscribed clients
			s.clientsMu.Lock()
			for client := range s.clients {
				if client.ch != nil {
					select {
					case client.ch <- chunk:
					default:
						// Client buffer full, skip this chunk
					}
				}
			}
			s.clientsMu.Unlock()
		}
	}
}
