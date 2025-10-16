# Local ICY Metadata Proxy — Multi-Station Spec (v1.1)

## Core idea

* Run one daemon that hosts multiple stations.
* Each station has its own upstream audio URL, metadata URL, buffer, and state.
* HTTP path routes select the station:

  * `/fip/stream`, `/fip/meta`
  * `/fip_hiphop/stream`, `/fip_hiphop/meta`
  * Add as many as you like.

## Station model

For each `station`:

* `id` (URL-safe, used in routes)
* `source.url` (MP3 stream)
* `metadata.url` (JSON/whatever you fetch)
* `icy.metaint` (default per station; can differ)
* `buffering` + `reconnect` policy (per station)
* live state: `currentMeta`, `updatedAt`, `ringBuffer`, `clients[]`, `sourceConnected`

All stations share the same binary/process but **do not** share readers/buffers.

## Routing

* `GET /:station/stream` → ICY stream for that station.
* `GET /:station/meta` → JSON now-playing for that station.
* `GET /:station/healthz` → station health JSON.
* `GET /stations` → list of station IDs + minimal status.
* `GET /healthz` → aggregate (all stations).

Return `404` for unknown `:station`.

## Concurrency model

* One `SourceReader` goroutine/task per station.
* One `MetadataPoller` per station.
* Fan-out per station: station-scoped event bus + ring buffer.
* Client sessions attach to the selected station only.

## Config (YAML)

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
      url: "https://audio.example/fip.mp3"
      request_headers: { Icy-MetaData: "0" }
      connect_timeout_ms: 5000
      read_timeout_ms: 15000
      reconnect: { backoff_initial_ms: 1000, backoff_max_ms: 30000 }
    metadata:
      url: "https://fip-metadata.fly.dev/"
      poll_ms: 3000
      stale_ttl_ms: 300000
      build:
        format: "StreamTitle='{artist} - {title}';"
        strip_single_quotes: true
        encoding: "iso-8859-1"
        normalize_whitespace: true
        fallback_key_order: ["title", "track.title"]
    buffering:
      ring_bytes: 262144
      client_pending_max_bytes: 262144

  - id: "fip_hiphop"
    icy:
      name: "FIP Hip-Hop (proxy)"
      metaint: 16384
      bitrate_hint_kbps: 128
    source:
      url: "https://audio.example/fip_hiphop.mp3"
      request_headers: { Icy-MetaData: "0" }
      connect_timeout_ms: 5000
      read_timeout_ms: 15000
      reconnect: { backoff_initial_ms: 1000, backoff_max_ms: 30000 }
    metadata:
      url: "https://fip-metadata.fly.dev/hiphop"   # if different; else reuse
      poll_ms: 3000
      stale_ttl_ms: 300000
      build:
        format: "StreamTitle='{artist} - {title}';"
        strip_single_quotes: true
        encoding: "iso-8859-1"
        normalize_whitespace: true
        fallback_key_order: ["title", "track.title"]
    buffering:
      ring_bytes: 262144
      client_pending_max_bytes: 262144

logging:
  level: info
  json: false

http:
  headers: {}
  # optional basic auth (if exposing beyond localhost)
  # auth:
  #   type: basic
  #   users:
  #     - username: "harper"
  #       password: "hunter2"
```

## Endpoints (per station)

* `/fip/stream` → `Content-Type: audio/mpeg`, `icy-metaint: 16384`, ICY blocks injected
* `/fip/meta` → `{current, updated_at, raw}`
* `/fip/healthz` → `{sourceConnected, clients, ring_fill_bytes, lastMetaAt}`
* `/stations` → `[{id, sourceConnected, clients}]`
* `/healthz` → `{ok: bool, stations: {...}}`

## ICY injection (unchanged)

* After each `metaInt` audio bytes, write 1-byte length + padded metadata payload (16-byte blocks).
* Max payload 4080 bytes; truncate earlier (e.g., 512B) in practice.

## Metadata logic (per station)

* Poll on interval; normalize; only update when value changes.
* Debounce 1 cycle to avoid flaps.
* On endpoint failure, keep last value until `stale_ttl_ms`, then send empty metadata blocks.

## Resilience (per station)

* Reconnect with exponential backoff on source failure; keep clients attached.
* No silence synthesis (v1.1); clients stall until bytes resume.
* Ring buffer (~256 KB) smooths hiccups; new clients start at live head.

## Backpressure

* Each client has a bounded pending queue; if over limit or slow, drop the client.
* Station fan-out is non-blocking; slow clients don’t stall others.

## Security

* Default bind `127.0.0.1`.
* If binding LAN, consider basic auth or a simple HMAC token on `/stream`.

## Observability

* Logs include `station_id` for every event.
* Per-station counters: `clients_current`, `source_disconnects`, `metadata_failures`.
* Gauges: `ring_fill_bytes`, `client_pending_max`.

## Performance targets (multi-station)

* With 2–4 stations, ~10 clients total @128kbps: <10% CPU, <128MB RSS on a modest box.

## Testing matrix (multi-station)

* Parallel source disconnects: one station drops, others unaffected.
* Metadata update skew: one station flaps; others stable.
* Slow client on `/fip_hiphop` doesn’t impact `/fip`.
* Unknown route `/nope/stream` → 404.

## Directory layout

```
/cmd/metadata-proxy/main.(js|go)
/internal/config/...
/internal/station/manager.(js|go)      # creates N stations from config
/internal/station/instance.(js|go)     # SourceReader, Poller, Fanout
/internal/icy/block.(js|go)
/internal/http/router.(js|go)
/configs/example.yaml
```

## Minimal route map (what you asked for)

* `/fip/stream` → fip
* `/fip_hiphop/stream` → fip_hiphop
* same for `/meta`, `/healthz`.

