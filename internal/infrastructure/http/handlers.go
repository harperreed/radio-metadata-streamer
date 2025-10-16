// ABOUTME: HTTP handlers for station endpoints
// ABOUTME: Implements stream, metadata, and health check routes
package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/harper/radio-metadata-proxy/internal/application/manager"
	"github.com/harper/radio-metadata-proxy/internal/domain/station"
	"github.com/harper/radio-metadata-proxy/internal/infrastructure/icy"
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

	// Check if client wants ICY metadata
	wantsMetadata := r.Header.Get("Icy-MetaData") == "1"

	// Set ICY headers
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("icy-name", st.ICYName())
	w.Header().Set("icy-br", fmt.Sprintf("%d", st.BitrateHint()))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "close")

	// Only send metaint if client wants metadata
	if wantsMetadata {
		w.Header().Set("icy-metaint", fmt.Sprintf("%d", st.MetaInt()))
	}

	w.WriteHeader(http.StatusOK)

	// Subscribe to station chunks
	client := &station.Client{ID: fmt.Sprintf("http-%p", r)}
	chunks := st.Subscribe(client)
	defer st.Unsubscribe(client)

	// Stream with ICY metadata injection
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	var metaInt int
	var bytesUntilMeta int

	if wantsMetadata {
		metaInt = st.MetaInt()
		bytesUntilMeta = metaInt
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case chunk, ok := <-chunks:
			if !ok {
				return
			}

			// Write chunk in pieces, injecting metadata at intervals
			for len(chunk) > 0 {
				if wantsMetadata {
					// Write up to next metadata point
					toWrite := len(chunk)
					if toWrite > bytesUntilMeta {
						toWrite = bytesUntilMeta
					}

					n, err := w.Write(chunk[:toWrite])
					if err != nil {
						return
					}

					chunk = chunk[n:]
					bytesUntilMeta -= n

					// Inject metadata if needed
					if bytesUntilMeta == 0 {
						meta := st.CurrentMetadata()
						if meta == "" {
							meta = "StreamTitle='';"
						}

						// Always send metadata at intervals (ICY spec requires it)
						metaBlock := icy.BuildBlock(meta)
						if _, err := w.Write(metaBlock); err != nil {
							return
						}

						bytesUntilMeta = metaInt
					}
				} else {
					// No metadata - just stream audio directly
					n, err := w.Write(chunk)
					if err != nil {
						return
					}
					chunk = chunk[n:]
				}
			}

			flusher.Flush()
		}
	}
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

// CoverHandler redirects to (or serves) the current artwork URL for a station.
type CoverHandler struct {
	mgr *manager.Manager
}

func NewCoverHandler(mgr *manager.Manager) *CoverHandler {
	return &CoverHandler{mgr: mgr}
}

func (h *CoverHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 2 || parts[1] != "cover" {
		http.NotFound(w, r)
		return
	}

	stationID := parts[0]
	st := h.mgr.Get(stationID)
	if st == nil {
		http.NotFound(w, r)
		return
	}

	meta := st.CurrentMetadata()
	// Parse Artwork='...'; from the ICY string
	art := extractKV(meta, "Artwork")
	if art == "" {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, art, http.StatusFound)
}

// extractKV finds Key='value'; in a semicolon-separated ICY string.
func extractKV(icy string, key string) string {
	keyEq := key + "='"
	if i := strings.Index(icy, keyEq); i >= 0 {
		rest := icy[i+len(keyEq):]
		if j := strings.Index(rest, "';"); j >= 0 {
			return rest[:j]
		}
	}
	return ""
}
