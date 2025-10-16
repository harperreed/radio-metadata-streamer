// ABOUTME: Tests for HTTP stream source implementation
// ABOUTME: Verifies connection handling and error cases
package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPSource_Connect(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check headers
		if r.Header.Get("Icy-MetaData") != "0" {
			t.Errorf("expected Icy-MetaData: 0 header")
		}

		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("audio data"))
	}))
	defer server.Close()

	cfg := HTTPConfig{
		URL:            server.URL,
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    10 * time.Second,
	}

	src := NewHTTP(cfg)

	ctx := context.Background()
	reader, err := src.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer reader.Close()

	buf := make([]byte, 10)
	n, _ := reader.Read(buf)

	if string(buf[:n]) != "audio data" {
		t.Errorf("expected 'audio data', got %q", buf[:n])
	}
}
