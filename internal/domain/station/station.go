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
