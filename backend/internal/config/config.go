package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// Database
	DatabaseURL string

	// Redis
	RedisURL string

	// Security
	JWTSecret     string
	EncryptionKey string

	// Logging
	LogLevel string

	// Server
	APIPort     string
	ScannerPort string
	RelayPort   string

	// Scanner
	ScannerTimeout     int
	ScannerConcurrency int

	// Stream
	StreamIdleTimeout  int
	StreamMaxSessions  int
	HLSSegmentDuration int
}

// Load reads configuration from environment variables with sensible defaults.
// Auto-generates secrets if they are not set.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:        getEnv("CAMGUID_DB_URL", "postgres://camguid:camguid@localhost:5432/camguid?sslmode=disable"),
		RedisURL:           getEnv("CAMGUID_REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:          getEnv("CAMGUID_JWT_SECRET", ""),
		EncryptionKey:      getEnv("CAMGUID_ENCRYPTION_KEY", ""),
		LogLevel:           getEnv("CAMGUID_LOG_LEVEL", "info"),
		APIPort:            getEnv("CAMGUID_API_PORT", "8080"),
		ScannerPort:        getEnv("CAMGUID_SCANNER_PORT", "8081"),
		RelayPort:          getEnv("CAMGUID_RELAY_PORT", "8082"),
		ScannerTimeout:     getEnvInt("CAMGUID_SCANNER_TIMEOUT", 60),
		ScannerConcurrency: getEnvInt("CAMGUID_SCANNER_CONCURRENCY", 50),
		StreamIdleTimeout:  getEnvInt("CAMGUID_STREAM_IDLE_TIMEOUT", 60),
		StreamMaxSessions:  getEnvInt("CAMGUID_STREAM_MAX_SESSIONS", 20),
		HLSSegmentDuration: getEnvInt("CAMGUID_HLS_SEGMENT_DURATION", 2),
	}

	// Auto-generate JWT secret if not provided
	if cfg.JWTSecret == "" {
		secret, err := generateRandomHex(32)
		if err != nil {
			return nil, fmt.Errorf("failed to generate JWT secret: %w", err)
		}
		cfg.JWTSecret = secret
		fmt.Println("[CONFIG] Auto-generated JWT_SECRET (set CAMGUID_JWT_SECRET to persist)")
	}

	// Auto-generate encryption key if not provided
	if cfg.EncryptionKey == "" {
		key, err := generateRandomHex(32)
		if err != nil {
			return nil, fmt.Errorf("failed to generate encryption key: %w", err)
		}
		cfg.EncryptionKey = key
		fmt.Println("[CONFIG] Auto-generated ENCRYPTION_KEY (set CAMGUID_ENCRYPTION_KEY to persist)")
	}

	// Validate encryption key length (must be 32 bytes = 64 hex chars)
	if len(cfg.EncryptionKey) != 64 {
		return nil, fmt.Errorf("CAMGUID_ENCRYPTION_KEY must be 64 hex characters (32 bytes), got %d", len(cfg.EncryptionKey))
	}

	return cfg, nil
}

// Validate performs runtime validation on the config.
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("CAMGUID_DB_URL is required")
	}
	if c.RedisURL == "" {
		return fmt.Errorf("CAMGUID_REDIS_URL is required")
	}
	if c.ScannerTimeout < 5 || c.ScannerTimeout > 300 {
		return fmt.Errorf("CAMGUID_SCANNER_TIMEOUT must be between 5 and 300 seconds")
	}
	if c.ScannerConcurrency < 1 || c.ScannerConcurrency > 500 {
		return fmt.Errorf("CAMGUID_SCANNER_CONCURRENCY must be between 1 and 500")
	}
	if c.StreamMaxSessions < 1 || c.StreamMaxSessions > 100 {
		return fmt.Errorf("CAMGUID_STREAM_MAX_SESSIONS must be between 1 and 100")
	}
	return nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return strings.TrimSpace(val)
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return i
		}
	}
	return defaultVal
}

func generateRandomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
