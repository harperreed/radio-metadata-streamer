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
