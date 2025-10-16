// ABOUTME: Main entry point for ICY metadata proxy
// ABOUTME: Loads config, starts stations, runs HTTP server
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/harper/radio-metadata-proxy/internal/application/config"
	"github.com/harper/radio-metadata-proxy/internal/application/manager"
	"github.com/harper/radio-metadata-proxy/internal/infrastructure/http"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	// Load config
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Create station manager
	mgr, err := manager.NewFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("create manager: %w", err)
	}

	// Start stations
	if err := mgr.Start(); err != nil {
		return fmt.Errorf("start stations: %w", err)
	}

	// Setup HTTP routes
	mux := nethttp.NewServeMux()
	mux.Handle("/stations", http.NewStationsHandler(mgr))
	mux.HandleFunc("/healthz", http.HealthzHandler)

	// Station-specific routes
	streamHandler := http.NewStreamHandler(mgr)
	metaHandler := http.NewMetaHandler(mgr)

	mux.HandleFunc("/", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if len(r.URL.Path) > 1 && r.URL.Path[len(r.URL.Path)-7:] == "/stream" {
			streamHandler.ServeHTTP(w, r)
			return
		}
		if len(r.URL.Path) > 1 && r.URL.Path[len(r.URL.Path)-5:] == "/meta" {
			metaHandler.ServeHTTP(w, r)
			return
		}
		nethttp.NotFound(w, r)
	})

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Listen.Host, cfg.Listen.Port)
	srv := &nethttp.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // Streaming
		IdleTimeout:  0, // Streaming
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	// Graceful shutdown
	shutdown := make(chan error, 1)
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		log.Println("shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		shutdown <- srv.Shutdown(ctx)
	}()

	// Start server
	log.Printf("listening on http://%s (try /stations)", addr)
	if err := srv.ListenAndServe(); err != nil && err != nethttp.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}

	// Wait for shutdown
	if err := <-shutdown; err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	// Shutdown stations
	if err := mgr.Shutdown(); err != nil {
		return fmt.Errorf("shutdown stations: %w", err)
	}

	log.Println("shutdown complete")
	return nil
}
