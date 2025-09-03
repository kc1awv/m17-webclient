package config

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/kc1awv/m17-webclient/internal/cors"
	log "github.com/kc1awv/m17-webclient/internal/logger"
)

type Config struct {
	ServerName     string
	AllowedOrigins []cors.OriginRule
	AllowedHeaders []string
	AllowedMethods []string
	ListenAddr     string
	ListenPort     int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	MaxSessions    int
	WSPingInterval time.Duration
	WSPongWait     time.Duration
}

func (c Config) Address() string {
	addr := ":8090"
	switch {
	case c.ListenAddr != "" && c.ListenPort != 0:
		addr = net.JoinHostPort(c.ListenAddr, strconv.Itoa(c.ListenPort))
	case c.ListenAddr != "":
		addr = c.ListenAddr
	case c.ListenPort != 0:
		addr = ":" + strconv.Itoa(c.ListenPort)
	}
	return addr
}

var loadEnvOnce sync.Once

func loadEnv() {
	if os.Getenv("SERVER_NAME") == "" {
		if err := godotenv.Load(); err != nil {
			log.Info("No .env file found", "err", err)
		}
	}
}

func Load() (Config, error) {
	loadEnvOnce.Do(loadEnv)

	cfg := Config{}
	var errs []error

	cfg.ServerName = os.Getenv("SERVER_NAME")
	cfg.AllowedOrigins = cors.ParseOriginRules(os.Getenv("ALLOWED_ORIGINS"))

	cfg.AllowedHeaders = append([]string{"Content-Type"}, splitAndTrim(os.Getenv("ALLOWED_HEADERS"))...)

	cfg.AllowedMethods = append([]string{http.MethodGet, http.MethodPost, http.MethodOptions}, splitAndTrim(os.Getenv("ALLOWED_METHODS"))...)

	cfg.ListenAddr = os.Getenv("LISTEN_ADDR")
	if v := os.Getenv("LISTEN_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p <= 0 || p > 65535 {
			errs = append(errs, fmt.Errorf("invalid LISTEN_PORT %q: %w", v, err))
		} else {
			cfg.ListenPort = p
		}
	}

	if v := os.Getenv("MAX_SESSIONS"); v != "" {
		m, err := strconv.Atoi(v)
		if err != nil || m <= 0 {
			errs = append(errs, fmt.Errorf("invalid MAX_SESSIONS %q: %w", v, err))
		} else {
			cfg.MaxSessions = m
		}
	}

	var err error
	cfg.ReadTimeout, err = parseDurationEnv("SERVER_READ_TIMEOUT", 15*time.Second)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.WriteTimeout, err = parseDurationEnv("SERVER_WRITE_TIMEOUT", 15*time.Second)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.IdleTimeout, err = parseDurationEnv("SERVER_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		errs = append(errs, err)
	}

	cfg.WSPingInterval, err = parseDurationEnv("WS_PING_INTERVAL", 30*time.Second)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.WSPongWait, err = parseDurationEnv("WS_PONG_WAIT", 60*time.Second)
	if err != nil {
		errs = append(errs, err)
	}

	return cfg, errors.Join(errs...)
}

func parseDurationEnv(key string, def time.Duration) (time.Duration, error) {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return def, fmt.Errorf("invalid duration format for %s: %w", key, err)
		}
		if d <= 0 {
			return def, fmt.Errorf("non-positive duration for %s: %s", key, v)
		}
		return d, nil
	}
	return def, nil
}

func splitAndTrim(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			res = append(res, p)
		}
	}
	return res
}
