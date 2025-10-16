// ABOUTME: YAML configuration parsing and validation
// ABOUTME: Defines structure for multi-station proxy configuration
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen   ListenConfig    `yaml:"listen"`
	Stations []StationConfig `yaml:"stations"`
	Logging  LoggingConfig   `yaml:"logging"`
}

type ListenConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type StationConfig struct {
	ID        string          `yaml:"id"`
	ICY       ICYConfig       `yaml:"icy"`
	Source    SourceConfig    `yaml:"source"`
	Metadata  MetadataConfig  `yaml:"metadata"`
	Buffering BufferingConfig `yaml:"buffering"`
}

type ICYConfig struct {
	Name            string `yaml:"name"`
	MetaInt         int    `yaml:"metaint"`
	BitrateHintKbps int    `yaml:"bitrate_hint_kbps"`
}

type SourceConfig struct {
	URL              string            `yaml:"url"`
	RequestHeaders   map[string]string `yaml:"request_headers"`
	ConnectTimeoutMs int               `yaml:"connect_timeout_ms"`
	ReadTimeoutMs    int               `yaml:"read_timeout_ms"`
}

type MetadataConfig struct {
	URL    string      `yaml:"url"`
	PollMs int         `yaml:"poll_ms"`
	Build  BuildConfig `yaml:"build"`
}

type BuildConfig struct {
	Format              string   `yaml:"format"`
	StripSingleQuotes   bool     `yaml:"strip_single_quotes"`
	Encoding            string   `yaml:"encoding"`
	NormalizeWhitespace bool     `yaml:"normalize_whitespace"`
	FallbackKeyOrder    []string `yaml:"fallback_key_order"`
}

type BufferingConfig struct {
	RingBytes             int `yaml:"ring_bytes"`
	ClientPendingMaxBytes int `yaml:"client_pending_max_bytes"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
	JSON  bool   `yaml:"json"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	return &cfg, nil
}
