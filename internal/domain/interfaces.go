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
