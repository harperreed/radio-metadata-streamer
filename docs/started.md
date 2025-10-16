Go. For this kind of long-running, low-latency byte shoveling with per-client state, Go’s goroutines + channels + zero-dependency static binary beat Node’s event loop and package bloat. You’ll get steadier memory, simpler deploy (single binary), and easier concurrency (one goroutine per station + per client). Node only wins if you want to hack fast in JS or reuse existing web code.

Here’s a tight Go starter that matches your multi-station spec. It’s a single file so you can feel the shape; expand into packages later.

```go
// cmd/icyproxy/main.go
// go 1.22+
// Minimal multi-station ICY proxy: MP3 passthrough + ICY metadata injection.
// - Path routing: /{station}/stream, /{station}/meta, /stations, /healthz
// - One SourceReader + MetadataPoller per station
// - Very small ring buffer to smooth hiccups
//
// Build: go build -o icyproxy ./cmd/icyproxy
// Run:   ./icyproxy
//
// NOTE: This is a compact starter; productionize by:
// - moving to packages, YAML config, better logging, timeouts, auth, metrics.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultMetaInt     = 16384
	ringBytes          = 256 * 1024
	clientPendingLimit = 256 * 1024
	metadataPollEvery  = 3 * time.Second
	staleTTL           = 5 * time.Minute
	listenAddr         = "127.0.0.1:8000" // change to 0.0.0.0 for LAN
)

// ---- Station model ----

type Station struct {
	ID            string
	ICYName       string
	MetaInt       int
	BitrateHint   int
	SourceURL     string
	MetadataURL   string
	srcClient     *http.Client
	metaClient    *http.Client
	ring          *Ring
	clients       map[*Client]struct{}
	muClients     sync.Mutex
	chunkBus      chan []byte
	sourceOK      atomic.Bool
	curMeta       atomic.Pointer[string]
	lastMetaAt    atomic.Pointer[time.Time]
	stop          context.CancelFunc
	reconnectBack time.Duration
}

type Client struct {
	w          http.ResponseWriter
	flusher    http.Flusher
	bytesToMet int
	pending    int
	closed     atomic.Bool
	station    *Station
}

// ---- Minimal ring buffer for smoothing hiccups ----

type Ring struct {
	buf []byte
	w   int
	n   int
	mu  sync.Mutex
}

func NewRing(sz int) *Ring { return &Ring{buf: make([]byte, sz)} }

// Write appends bytes (drop oldest on overflow).
func (r *Ring) Write(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(p) > 0 {
		space := len(r.buf) - r.n
		if space == 0 {
			// drop 1/4 oldest to make room
			drop := len(r.buf) / 4
			r.w = (r.w + drop) % len(r.buf)
			r.n -= drop
			if r.n < 0 {
				r.n = 0
			}
			space = len(r.buf) - r.n
		}
		chunk := min(space, len(p))
		end := (r.w + r.n) % len(r.buf)
		right := min(chunk, len(r.buf)-end)
		copy(r.buf[end:end+right], p[:right])
		if right < chunk {
			copy(r.buf[0:chunk-right], p[right:chunk])
		}
		r.n += chunk
		p = p[chunk:]
	}
}

// Snapshot returns a contiguous copy of current buffer head..tail.
func (r *Ring) Snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]byte, r.n)
	if r.n == 0 {
		return out
	}
	head := r.w
	tail := (r.w + r.n) % len(r.buf)
	if head < tail {
		copy(out, r.buf[head:tail])
	} else {
		copy(out, r.buf[head:])
		copy(out[len(r.buf)-head:], r.buf[:tail])
	}
	return out
}

// ---- Station runtime ----

func (s *Station) Start(ctx context.Context, wg *sync.WaitGroup) {
	ctx, cancel := context.WithCancel(ctx)
	s.stop = cancel
	wg.Add(2)
	go func() { defer wg.Done(); s.runSource(ctx) }()
	go func() { defer wg.Done(); s.runMetadata(ctx) }()
}

func (s *Station) runSource(ctx context.Context) {
	backoff := 1 * time.Second
	for {
		req, _ := http.NewRequestWithContext(ctx, "GET", s.SourceURL, nil)
		req.Header.Set("Icy-MetaData", "0")
		resp, err := s.srcClient.Do(req)
		if err != nil || resp.StatusCode != 200 {
			log.Printf("[%s] source connect failed: %v (code=%v)", s.ID, err, statusCode(resp))
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			backoff = minDur(backoff*2, 30*time.Second)
			continue
		}
		log.Printf("[%s] source connected", s.ID)
		s.sourceOK.Store(true)
		backoff = 1 * time.Second
		r := resp.Body
		buf := make([]byte, 32*1024)
		for {
			n, e := r.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				// write to ring (for smoothing) and broadcast
				s.ring.Write(chunk)
				select {
				case s.chunkBus <- chunk:
				default:
					// if bus is full, drop chunk (protect producer)
				}
			}
			if e != nil {
				if !errors.Is(e, io.EOF) {
					log.Printf("[%s] source read error: %v", s.ID, e)
				}
				break
			}
			if ctx.Err() != nil {
				break
			}
		}
		s.sourceOK.Store(false)
		resp.Body.Close()
		log.Printf("[%s] source disconnected", s.ID)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		backoff = minDur(backoff*2, 30*time.Second)
	}
}

func (s *Station) runMetadata(ctx context.Context) {
	var last string
	ticker := time.NewTicker(metadataPollEvery)
	defer ticker.Stop()
	for {
		_ = s.fetchAndSetMeta(&last)
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func (s *Station) fetchAndSetMeta(last *string) error {
	req, _ := http.NewRequest("GET", s.MetadataURL, nil)
	req.Header.Set("Cache-Control", "no-store")
	resp, err := s.metaClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	title := parseTitle(body) // flexible JSON/text parse
	title = normalizeTitle(title)
	icy := fmt.Sprintf("StreamTitle='%s';", sanitizeQuotes(title))
	if icy != *last && icy != "StreamTitle='';" {
		s.curMeta.Store(&icy)
		now := time.Now()
		s.lastMetaAt.Store(&now)
		*last = icy
		log.Printf("[%s] meta: %s", s.ID, title)
	}
	return nil
}

// ---- HTTP handlers ----

func (s *Station) handleStream(w http.ResponseWriter, r *http.Request) {
	// headers: ICY-compatible
	h := w.Header()
	h.Set("Content-Type", "audio/mpeg")
	h.Set("icy-name", s.ICYName)
	h.Set("icy-br", fmt.Sprintf("%d", s.BitrateHint))
	h.Set("icy-metaint", fmt.Sprintf("%d", s.MetaInt))
	// Some clients care less about status line; Go will send HTTP/1.1 200 OK.
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	c := &Client{
		w:          w,
		flusher:    w.(http.Flusher),
		bytesToMet: s.MetaInt,
		station:    s,
	}
	s.addClient(c)
	defer s.removeClient(c)

	notify := r.Context().Done()
	// Start by sending nothing from ring (live only, simpler).
	// If you want, you can write s.ring.Snapshot() here.

	for {
		select {
		case chunk := <-s.chunkBus:
			if c.closed.Load() {
				return
			}
			if err := c.writeWithICY(chunk); err != nil {
				return
			}
		case <-notify:
			return
		}
	}
}

func (s *Station) handleMeta(w http.ResponseWriter, _ *http.Request) {
	type resp struct {
		Current   string     `json:"current"`
		UpdatedAt *time.Time `json:"updated_at,omitempty"`
		SourceOK  bool       `json:"sourceConnected"`
	}
	cur := ""
	if p := s.curMeta.Load(); p != nil {
		cur = *p
	}
	var at *time.Time
	if t := s.lastMetaAt.Load(); t != nil {
		at = t
	}
	j, _ := json.Marshal(resp{Current: cur, UpdatedAt: at, SourceOK: s.sourceOK.Load()})
	w.Header().Set("Content-Type", "application/json")
	w.Write(j)
}

func (s *Station) addClient(c *Client) {
	s.muClients.Lock()
	if s.clients == nil {
		s.clients = map[*Client]struct{}{}
	}
	s.clients[c] = struct{}{}
	s.muClients.Unlock()
	log.Printf("[%s] client +1 (now %d)", s.ID, s.clientCount())
}

func (s *Station) removeClient(c *Client) {
	c.closed.Store(true)
	s.muClients.Lock()
	delete(s.clients, c)
	s.muClients.Unlock()
	log.Printf("[%s] client -1 (now %d)", s.ID, s.clientCount())
}

func (s *Station) clientCount() int {
	s.muClients.Lock()
	defer s.muClients.Unlock()
	return len(s.clients)
}

// ---- Client write with ICY injection ----

func (c *Client) writeWithICY(chunk []byte) error {
	for len(chunk) > 0 {
		if c.bytesToMet > len(chunk) {
			// Write all and decrement.
			if _, err := c.w.Write(chunk); err != nil {
				return err
			}
			c.bytesToMet -= len(chunk)
			c.flusher.Flush()
			return nil
		}
		// Write up to boundary.
		n := c.bytesToMet
		if n > 0 {
			if _, err := c.w.Write(chunk[:n]); err != nil {
				return err
			}
			c.flusher.Flush()
			chunk = chunk[n:]
		}
		// Inject metadata block.
		meta := ""
		if p := c.station.curMeta.Load(); p != nil {
			meta = *p
		}
		block := buildICYBlock(meta)
		if _, err := c.w.Write(block); err != nil {
			return err
		}
		c.flusher.Flush()
		c.bytesToMet = c.station.MetaInt
	}
	return nil
}

// ---- ICY helpers ----

func buildICYBlock(text string) []byte {
	if text == "" {
		return []byte{0x00}
	}
	// ICY wants 16-byte padded, length byte is count of 16-byte chunks (max 255).
	payload := []byte(text)
	if len(payload) > 255*16 {
		payload = payload[:255*16]
	}
	blocks := (len(payload) + 15) / 16
	if blocks > 255 {
		blocks = 255
	}
	pad := blocks*16 - len(payload)
	var b bytes.Buffer
	b.WriteByte(byte(blocks))
	b.Write(payload)
	if pad > 0 {
		b.Write(bytes.Repeat([]byte{0x00}, pad))
	}
	return b.Bytes()
}

func parseTitle(body []byte) string {
	trim := strings.TrimSpace(string(body))
	if strings.HasPrefix(trim, "{") {
		var m map[string]any
		if json.Unmarshal(body, &m) == nil {
			// try common shapes
			if t, ok := m["title"].(string); ok && t != "" {
				return t
			}
			if track, ok := m["track"].(map[string]any); ok {
				a, _ := track["artist"].(string)
				t, _ := track["title"].(string)
				return strings.TrimSpace(strings.TrimSpace(a) + " - " + strings.TrimSpace(t))
			}
			// fallback: flatten
			j, _ := json.Marshal(m)
			return string(j)
		}
	}
	return trim
}

func normalizeTitle(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func sanitizeQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "") // minimal; extend if you need ISO-8859-1 mapping
}

// ---- main / routing ----

func main() {
	// Define your stations here (swap URLs for real ones)
	stations := []*Station{
		newStation("fip", "FIP (proxy)", defaultMetaInt, 128,
			"https://example.com/fip.mp3",
			"https://fip-metadata.fly.dev/"),
		newStation("fip_hiphop", "FIP Hip-Hop (proxy)", defaultMetaInt, 128,
			"https://example.com/fip_hiphop.mp3",
			"https://fip-metadata.fly.dev/hiphop"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stations", func(w http.ResponseWriter, _ *http.Request) {
		type s struct {
			ID           string `json:"id"`
			Clients      int    `json:"clients"`
			SourceOK     bool   `json:"sourceConnected"`
			Stream       string `json:"stream"`
			MetaEndpoint string `json:"meta"`
		}
		var out []s
		for _, st := range stations {
			out = append(out, s{
				ID:           st.ID,
				Clients:      st.clientCount(),
				SourceOK:     st.sourceOK.Load(),
				Stream:       fmt.Sprintf("/%s/stream", st.ID),
				MetaEndpoint: fmt.Sprintf("/%s/meta", st.ID),
			})
		}
		j, _ := json.Marshal(out)
		w.Header().Set("Content-Type", "application/json")
		w.Write(j)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		type h struct{ OK bool `json:"ok"` }
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(h{OK: true})
	})
	// Per-station routes
	for _, st := range stations {
		s := st
		mux.HandleFunc("/"+s.ID+"/stream", func(w http.ResponseWriter, r *http.Request) {
			// ensure writer is not proxied through buffering reverse proxies if you expose it
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Connection", "close")
			s.handleStream(w, r)
		})
		mux.HandleFunc("/"+s.ID+"/meta", s.handleMeta)
	}

	// Start stations
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for _, s := range stations {
		s.Start(ctx, &wg)
	}

	// HTTP server with sane timeouts; disable read timeout for streaming if needed.
	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // streaming
		IdleTimeout:  0, // streaming
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}
	log.Printf("listening on http://%s  (try /stations)", listenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	wg.Wait()
}

func newStation(id, name string, metaint, br int, srcURL, metaURL string) *Station {
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConns:        100,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &Station{
		ID:          id,
		ICYName:     name,
		MetaInt:     metaint,
		BitrateHint: br,
		SourceURL:   srcURL,
		MetadataURL: metaURL,
		srcClient:   &http.Client{Transport: tr, Timeout: 0},           // streaming: no total timeout
		metaClient:  &http.Client{Transport: tr, Timeout: 10 * time.Second},
		ring:        NewRing(ringBytes),
		chunkBus:    make(chan []byte, 32), // small buffer; drops on pressure
	}
}

// ---- helpers ----

func statusCode(r *http.Response) int {
	if r == nil {
		return 0
	}
	return r.StatusCode
}
func min(a, b int) int { if a < b { return a }; return b }
func minDur(a, b time.Duration) time.Duration { if a < b { return a }; return b }
```

How it works (brief):

* One goroutine reads each upstream MP3 and feeds a small ring + a chunk channel.
* Another polls metadata and updates an atomic string like `StreamTitle='Artist - Title';`.
* Each client connection counts bytes until `metaInt`, then injects a correctly padded ICY block.
* Routes are path-based: `/fip/stream`, `/fip/meta`, `/fip_hiphop/…`.
* It’s all stdlib; no deps, no CGO, single binary.

Alt path (Node): if you want a super-fast hack and don’t care about single-binary deploys, the Node version you saw earlier is fine; just mind long-lived connections and per-client backpressure.

Next logical extension: YAML config + systemd unit + optional basic auth + Prometheus metrics.

