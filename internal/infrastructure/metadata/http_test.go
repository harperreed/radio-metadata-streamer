// ABOUTME: Tests for HTTP metadata provider implementation
// ABOUTME: Verifies JSON parsing and ICY formatting
package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPProvider_Fetch_JSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"artist":"Test Artist","title":"Test Song"}`))
	}))
	defer server.Close()

	cfg := HTTPConfig{
		URL:     server.URL,
		Timeout: 5 * time.Second,
		Build: BuildConfig{
			Format: "StreamTitle='{artist} - {title}';",
		},
	}

	provider := NewHTTP(cfg)

	ctx := context.Background()
	result, err := provider.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	expected := "StreamTitle='Test Artist - Test Song';"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestHTTPProvider_Fetch_NestedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"now": {
				"firstLine": {"title": "Test Song"},
				"secondLine": {"title": "Test Artist"}
			}
		}`))
	}))
	defer server.Close()

	cfg := HTTPConfig{
		URL:     server.URL,
		Timeout: 5 * time.Second,
		Build: BuildConfig{
			Format: "StreamTitle='{artist} - {title}';",
			FallbackKeyOrder: []string{
				"now.secondLine.title",
				"now.firstLine.title",
			},
		},
	}

	provider := NewHTTP(cfg)

	ctx := context.Background()
	result, err := provider.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	expected := "StreamTitle='Test Artist - Test Song';"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
