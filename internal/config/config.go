package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr      string
	StorageDir    string
	PublicBaseURL string
	MaxFileSize   int64
	Auth          AuthConfig
}

type AuthConfig struct {
	JWKSUrl      string
	Issuer       string
	Audience     string
	JWKSCacheTTL int // Cache TTL in seconds
}

func Load() (*Config, error) {
	httpAddr := getEnv("MEDIA_HTTP_ADDR", ":8080")
	storageDir := getEnv("MEDIA_STORAGE_DIR", "/var/media")
	publicBaseURL := getEnv("MEDIA_PUBLIC_BASE_URL", "http://localhost:8080")
	maxFileSizeStr := getEnv("MEDIA_MAX_FILE_SIZE", "10485760")

	maxFileSize, err := strconv.ParseInt(maxFileSizeStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid MEDIA_MAX_FILE_SIZE: %w", err)
	}

	jwksCacheTTL := 900 // 15 minutes default
	if ttlStr := getEnv("AUTH_JWKS_CACHE_TTL", ""); ttlStr != "" {
		if ttl, err := strconv.Atoi(ttlStr); err == nil {
			jwksCacheTTL = ttl
		}
	}

	return &Config{
		HTTPAddr:      httpAddr,
		StorageDir:    storageDir,
		PublicBaseURL: publicBaseURL,
		MaxFileSize:   maxFileSize,
		Auth: AuthConfig{
			JWKSUrl:      getEnv("AUTH_JWKS_URL", "http://user-service:3000/.well-known/jwks.json"),
			Issuer:       getEnv("AUTH_ISSUER", "http://user-service:3000"),
			Audience:     getEnv("AUTH_AUDIENCE", "backboard"),
			JWKSCacheTTL: jwksCacheTTL,
		},
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
