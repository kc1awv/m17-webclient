package config

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/kc1awv/m17-webclient/internal/cors"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("SERVER_NAME", "")
	t.Setenv("ALLOWED_ORIGINS", "")
	t.Setenv("ALLOWED_HEADERS", "")
	t.Setenv("ALLOWED_METHODS", "")
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("LISTEN_PORT", "")
	t.Setenv("MAX_SESSIONS", "")
	t.Setenv("SERVER_READ_TIMEOUT", "")
	t.Setenv("SERVER_WRITE_TIMEOUT", "")
	t.Setenv("SERVER_IDLE_TIMEOUT", "")
	t.Setenv("WS_PING_INTERVAL", "")
	t.Setenv("WS_PONG_WAIT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ServerName != "" {
		t.Fatalf("ServerName = %q; want empty", cfg.ServerName)
	}
	if len(cfg.AllowedOrigins) != 0 {
		t.Fatalf("AllowedOrigins = %v; want empty", cfg.AllowedOrigins)
	}
	if !reflect.DeepEqual(cfg.AllowedHeaders, []string{"Content-Type"}) {
		t.Fatalf("AllowedHeaders = %v; want [Content-Type]", cfg.AllowedHeaders)
	}
	expectedMethods := []string{http.MethodGet, http.MethodPost, http.MethodOptions}
	if !reflect.DeepEqual(cfg.AllowedMethods, expectedMethods) {
		t.Fatalf("AllowedMethods = %v; want %v", cfg.AllowedMethods, expectedMethods)
	}
	if cfg.ListenPort != 0 {
		t.Fatalf("ListenPort = %d; want 0", cfg.ListenPort)
	}
	if cfg.Address() != ":8090" {
		t.Fatalf("Address() = %q; want :8090", cfg.Address())
	}
	if cfg.ReadTimeout != 15*time.Second {
		t.Fatalf("ReadTimeout = %v; want 15s", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 15*time.Second {
		t.Fatalf("WriteTimeout = %v; want 15s", cfg.WriteTimeout)
	}
	if cfg.IdleTimeout != 60*time.Second {
		t.Fatalf("IdleTimeout = %v; want 60s", cfg.IdleTimeout)
	}
	if cfg.MaxSessions != 0 {
		t.Fatalf("MaxSessions = %d; want 0", cfg.MaxSessions)
	}
	if cfg.WSPingInterval != 30*time.Second {
		t.Fatalf("WSPingInterval = %v; want 30s", cfg.WSPingInterval)
	}
	if cfg.WSPongWait != 60*time.Second {
		t.Fatalf("WSPongWait = %v; want 60s", cfg.WSPongWait)
	}
}

func TestLoadParsing(t *testing.T) {
	t.Setenv("SERVER_NAME", "srv")
	t.Setenv("ALLOWED_ORIGINS", "https://example.com")
	t.Setenv("ALLOWED_HEADERS", "X-Test, Authorization")
	t.Setenv("ALLOWED_METHODS", "PUT")
	t.Setenv("LISTEN_ADDR", "127.0.0.1")
	t.Setenv("LISTEN_PORT", "9000")
	t.Setenv("MAX_SESSIONS", "10")
	t.Setenv("SERVER_READ_TIMEOUT", "20s")
	t.Setenv("SERVER_WRITE_TIMEOUT", "25s")
	t.Setenv("SERVER_IDLE_TIMEOUT", "90s")
	t.Setenv("WS_PING_INTERVAL", "5s")
	t.Setenv("WS_PONG_WAIT", "10s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ServerName != "srv" {
		t.Fatalf("ServerName = %q; want srv", cfg.ServerName)
	}
	expectedRules := cors.ParseOriginRules("https://example.com")
	if !reflect.DeepEqual(cfg.AllowedOrigins, expectedRules) {
		t.Fatalf("AllowedOrigins = %v; want %v", cfg.AllowedOrigins, expectedRules)
	}
	expectedHeaders := []string{"Content-Type", "X-Test", "Authorization"}
	if !reflect.DeepEqual(cfg.AllowedHeaders, expectedHeaders) {
		t.Fatalf("AllowedHeaders = %v; want %v", cfg.AllowedHeaders, expectedHeaders)
	}
	expectedMethods := []string{http.MethodGet, http.MethodPost, http.MethodOptions, "PUT"}
	if !reflect.DeepEqual(cfg.AllowedMethods, expectedMethods) {
		t.Fatalf("AllowedMethods = %v; want %v", cfg.AllowedMethods, expectedMethods)
	}
	if cfg.ListenPort != 9000 {
		t.Fatalf("ListenPort = %d; want 9000", cfg.ListenPort)
	}
	if cfg.Address() != "127.0.0.1:9000" {
		t.Fatalf("Address() = %q; want 127.0.0.1:9000", cfg.Address())
	}
	if cfg.MaxSessions != 10 {
		t.Fatalf("MaxSessions = %d; want 10", cfg.MaxSessions)
	}
	if cfg.ReadTimeout != 20*time.Second {
		t.Fatalf("ReadTimeout = %v; want 20s", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 25*time.Second {
		t.Fatalf("WriteTimeout = %v; want 25s", cfg.WriteTimeout)
	}
	if cfg.IdleTimeout != 90*time.Second {
		t.Fatalf("IdleTimeout = %v; want 90s", cfg.IdleTimeout)
	}
	if cfg.WSPingInterval != 5*time.Second {
		t.Fatalf("WSPingInterval = %v; want 5s", cfg.WSPingInterval)
	}
	if cfg.WSPongWait != 10*time.Second {
		t.Fatalf("WSPongWait = %v; want 10s", cfg.WSPongWait)
	}
}

func TestLoadInvalidPort(t *testing.T) {
	t.Setenv("LISTEN_PORT", "abc")
	t.Setenv("SERVER_NAME", "srv")
	cfg, err := Load()
	if err == nil {
		t.Fatalf("Load() error = nil; want error")
	}
	if cfg.ListenPort != 0 {
		t.Fatalf("ListenPort = %d; want 0", cfg.ListenPort)
	}
}

func TestParseDurationEnvNonPositive(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"negative", "-1s"},
		{"zero", "0s"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_DURATION", tc.value)
			got, err := parseDurationEnv("TEST_DURATION", time.Second)
			if err == nil {
				t.Fatalf("parseDurationEnv(%q) error = nil; want error", tc.value)
			}
			if got != time.Second {
				t.Fatalf("parseDurationEnv(%q) = %v; want 1s", tc.value, got)
			}
		})
	}
}
