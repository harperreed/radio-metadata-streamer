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
