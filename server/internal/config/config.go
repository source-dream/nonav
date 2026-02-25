package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	APIListenAddr       string
	GatewayListenAddr   string
	APIBaseURL          string
	FrontendDevProxyURL string
	DBPath              string
	WebDistDir          string
	PublicBaseURL       string
	AllowedCORSOrigin   string
	DefaultShareTTL     time.Duration
	ForceFRP            bool
	FRPUpstreamURL      string
}

func Load() Config {
	defaultTTL := getEnvInt("NONAV_DEFAULT_SHARE_TTL_HOURS", 24)

	return Config{
		APIListenAddr:       getEnv("NONAV_API_LISTEN_ADDR", ":8081"),
		GatewayListenAddr:   getEnv("NONAV_GATEWAY_LISTEN_ADDR", ":8080"),
		APIBaseURL:          getEnv("NONAV_API_BASE_URL", "http://127.0.0.1:8081"),
		FrontendDevProxyURL: getEnv("NONAV_FRONTEND_DEV_PROXY_URL", ""),
		DBPath:              getEnv("NONAV_DB_PATH", "./data/nonav.db"),
		WebDistDir:          getEnv("NONAV_WEB_DIST_DIR", "./web-dist"),
		PublicBaseURL:       getEnv("NONAV_PUBLIC_BASE_URL", "http://localhost:8080"),
		AllowedCORSOrigin:   getEnv("NONAV_CORS_ORIGIN", "http://localhost:8080"),
		DefaultShareTTL:     time.Duration(defaultTTL) * time.Hour,
		ForceFRP:            getEnvBool("NONAV_FORCE_FRP", false),
		FRPUpstreamURL:      getEnv("NONAV_FRP_UPSTREAM_URL", "http://127.0.0.1:13000"),
	}
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}

	if value == "1" || value == "true" || value == "yes" || value == "on" {
		return true
	}

	if value == "0" || value == "false" || value == "no" || value == "off" {
		return false
	}

	return fallback
}
