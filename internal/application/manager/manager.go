// ABOUTME: Station manager for lifecycle and lookup
// ABOUTME: Creates stations from config and manages their goroutines
package manager

import (
	"context"
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
		if err := st.Start(); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) Shutdown() error {
	m.cancel()
	m.wg.Wait()

	// Shutdown all stations
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, st := range m.stations {
		if err := st.Shutdown(); err != nil {
			return err
		}
	}

	return nil
}
