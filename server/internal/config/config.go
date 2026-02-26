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
	ShareSubdomainBase  string
	ShareSubdomainOn    bool
	AllowedCORSOrigin   string
	DefaultShareTTL     time.Duration
	ForceFRP            bool
	FRPUpstreamURL      string
	FRPPortMin          int
	FRPPortMax          int
	FRPClientBin        string
	FRPServerBin        string
	EmbedFRPServer      bool
	FRPServerBindAddr   string
	FRPServerAddr       string
	FRPServerPort       int
	FRPAuthToken        string
	FRPRecoverOnStart   bool
	FRPExposeAPI        bool
	FRPAPIRemotePort    int
	LogLevel            string
	LogRouteTrace       bool
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
		ShareSubdomainBase:  getEnv("NONAV_SHARE_SUBDOMAIN_BASE", ""),
		ShareSubdomainOn:    getEnvBool("NONAV_SHARE_SUBDOMAIN_ENABLED", false),
		AllowedCORSOrigin:   getEnv("NONAV_CORS_ORIGIN", "http://localhost:8080"),
		DefaultShareTTL:     time.Duration(defaultTTL) * time.Hour,
		ForceFRP:            getEnvBool("NONAV_FORCE_FRP", false),
		FRPUpstreamURL:      getEnv("NONAV_FRP_UPSTREAM_URL", "http://127.0.0.1:13000"),
		FRPPortMin:          getEnvInt("NONAV_FRP_PORT_MIN", 13000),
		FRPPortMax:          getEnvInt("NONAV_FRP_PORT_MAX", 13000),
		FRPClientBin:        getEnv("NONAV_FRP_CLIENT_BIN", "frpc"),
		FRPServerBin:        getEnv("NONAV_FRP_SERVER_BIN", "frps"),
		EmbedFRPServer:      getEnvBool("NONAV_EMBED_FRPS", false),
		FRPServerBindAddr:   getEnv("NONAV_FRP_SERVER_BIND_ADDR", "0.0.0.0"),
		FRPServerAddr:       getEnv("NONAV_FRP_SERVER_ADDR", "127.0.0.1"),
		FRPServerPort:       getEnvInt("NONAV_FRP_SERVER_PORT", 7000),
		FRPAuthToken:        getEnv("NONAV_FRP_AUTH_TOKEN", "nonav-local-dev"),
		FRPRecoverOnStart:   getEnvBool("NONAV_FRP_RECOVER_ON_START", true),
		FRPExposeAPI:        getEnvBool("NONAV_FRP_EXPOSE_API", true),
		FRPAPIRemotePort:    getEnvInt("NONAV_FRP_API_REMOTE_PORT", 18081),
		LogLevel:            strings.ToLower(strings.TrimSpace(getEnv("NONAV_LOG_LEVEL", "info"))),
		LogRouteTrace:       getEnvBool("NONAV_LOG_ROUTE_TRACE", true),
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
