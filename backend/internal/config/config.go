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
	JWTSecret           string
	EncryptionKey       string
	StreamSigningSecret string // shared HMAC secret for signing stream access tokens

	// Logging
	LogLevel string

	// Server
	APIPort     string
	ScannerPort string
	RelayPort   string

	// Relay identity (for per-replica logs/metrics)
	RelayInstanceID string

	// Scanner
	ScannerTimeout     int
	ScannerConcurrency int

	// Scanner abuse prevention
	ScanMaxConcurrent int // max concurrent scan jobs across the service
	ScanMinInterval   int // minimum seconds between scans by the same user

	// Login abuse prevention (used by the API service)
	LoginRateLimit  int // max failed attempts per window per IP+username
	LoginRateWindow int // window in seconds

	// Audit retention (used by the API service)
	AuditRetentionDays int // delete audit logs older than this

	// Stream
	StreamIdleTimeout  int
	StreamMaxSessions  int
	HLSSegmentDuration int

	// Snapshots
	SnapshotInterval int // seconds between periodic snapshot captures
	SnapshotWidth    int // max width of captured snapshot frames
}

// Load reads configuration from environment variables with sensible defaults.
// Secrets (JWT secret, encryption key, stream signing secret) are never
// auto-generated: the service fails fast if they are missing or set to a
// known placeholder value. Generate them once with scripts/generate-secrets.sh.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:        getEnv("CAMGUID_DB_URL", "postgres://camguid:camguid@localhost:5432/camguid?sslmode=disable"),
		RedisURL:           getEnv("CAMGUID_REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:          getEnv("CAMGUID_JWT_SECRET", ""),
		EncryptionKey:      getEnv("CAMGUID_ENCRYPTION_KEY", ""),
		StreamSigningSecret: getEnv("CAMGUID_STREAM_SIGNING_SECRET", ""),
		LogLevel:           getEnv("CAMGUID_LOG_LEVEL", "info"),
		APIPort:            getEnv("CAMGUID_API_PORT", "8080"),
		ScannerPort:        getEnv("CAMGUID_SCANNER_PORT", "8081"),
		RelayPort:          getEnv("CAMGUID_RELAY_PORT", "8082"),
		RelayInstanceID:    resolveInstanceID(),
		ScannerTimeout:     getEnvInt("CAMGUID_SCANNER_TIMEOUT", 60),
		ScannerConcurrency: getEnvInt("CAMGUID_SCANNER_CONCURRENCY", 50),
		StreamIdleTimeout:  getEnvInt("CAMGUID_STREAM_IDLE_TIMEOUT", 60),
		StreamMaxSessions:  getEnvInt("CAMGUID_STREAM_MAX_SESSIONS", 20),
		HLSSegmentDuration: getEnvInt("CAMGUID_HLS_SEGMENT_DURATION", 2),
		SnapshotInterval:   getEnvInt("CAMGUID_SNAPSHOT_INTERVAL", 30),
		SnapshotWidth:      getEnvInt("CAMGUID_SNAPSHOT_WIDTH", 640),
		ScanMaxConcurrent:  getEnvInt("CAMGUID_SCAN_MAX_CONCURRENT", 1),
		ScanMinInterval:    getEnvInt("CAMGUID_SCAN_MIN_INTERVAL", 60),
		LoginRateLimit:     getEnvInt("CAMGUID_LOGIN_RATE_LIMIT", 5),
		LoginRateWindow:    getEnvInt("CAMGUID_LOGIN_RATE_WINDOW", 60),
		AuditRetentionDays: getEnvInt("CAMGUID_AUDIT_RETENTION_DAYS", 90),
	}

	// Secrets must be explicitly provided and non-placeholder. Failing fast
	// here is intentional — it prevents a silently insecure deployment.
	if isInsecureSecret(cfg.JWTSecret) {
		return nil, fmt.Errorf("CAMGUID_JWT_SECRET is missing or set to a known placeholder. Run scripts/generate-secrets.sh and set it to a strong random value.")
	}
	if isInsecureSecret(cfg.EncryptionKey) {
		return nil, fmt.Errorf("CAMGUID_ENCRYPTION_KEY is missing or set to a known placeholder. Run scripts/generate-secrets.sh and set it to a random 64-hex value.")
	}
	if isInsecureSecret(cfg.StreamSigningSecret) {
		return nil, fmt.Errorf("CAMGUID_STREAM_SIGNING_SECRET is missing or set to a known placeholder. Run scripts/generate-secrets.sh and set it to a strong random value.")
	}

	// Encryption key must be 32 bytes = 64 hex chars (AES-256-GCM)
	if len(cfg.EncryptionKey) != 64 {
		return nil, fmt.Errorf("CAMGUID_ENCRYPTION_KEY must be 64 hex characters (32 bytes), got %d", len(cfg.EncryptionKey))
	}
	if _, err := hex.DecodeString(cfg.EncryptionKey); err != nil {
		return nil, fmt.Errorf("CAMGUID_ENCRYPTION_KEY must be hex-encoded: %w", err)
	}

	return cfg, nil
}

// isInsecureSecret reports whether a configured secret should be rejected:
// empty, or matching any known placeholder/example value shipped in docs.
func isInsecureSecret(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "change_me", "change-me", "changeme":
		return true
	}
	lower := strings.ToLower(v)
	for _, prefix := range []string{"change_me", "change-me", "ch4ng3me"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	switch lower {
	case "super_secret_jwt_key_12345",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"camguid-nvr-stream-signing-secret-2026",
		"change-me-in-production",
		"change-me-to-a-long-random-string":
		return true
	}
	return false
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
	if c.StreamIdleTimeout < 5 {
		return fmt.Errorf("CAMGUID_STREAM_IDLE_TIMEOUT must be at least 5 seconds")
	}
	if c.SnapshotInterval < 5 {
		return fmt.Errorf("CAMGUID_SNAPSHOT_INTERVAL must be at least 5 seconds")
	}
	if c.ScanMaxConcurrent < 1 || c.ScanMaxConcurrent > 10 {
		return fmt.Errorf("CAMGUID_SCAN_MAX_CONCURRENT must be between 1 and 10")
	}
	if c.ScanMinInterval < 10 || c.ScanMinInterval > 86400 {
		return fmt.Errorf("CAMGUID_SCAN_MIN_INTERVAL must be between 10 and 86400 seconds")
	}
	if c.LoginRateLimit < 1 || c.LoginRateLimit > 1000 {
		return fmt.Errorf("CAMGUID_LOGIN_RATE_LIMIT must be between 1 and 1000")
	}
	if c.LoginRateWindow < 10 || c.LoginRateWindow > 86400 {
		return fmt.Errorf("CAMGUID_LOGIN_RATE_WINDOW must be between 10 and 86400 seconds")
	}
	if c.AuditRetentionDays < 7 || c.AuditRetentionDays > 3650 {
		return fmt.Errorf("CAMGUID_AUDIT_RETENTION_DAYS must be between 7 and 3650")
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

// resolveInstanceID returns the CAMGUID_RELAY_ID override, falling back to the
// container hostname so multi-replica deployments get distinct identities.
func resolveInstanceID() string {
	if id := os.Getenv("CAMGUID_RELAY_ID"); id != "" {
		return strings.TrimSpace(id)
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "relay"
	}
	return host
}

func generateRandomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
