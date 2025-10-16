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
