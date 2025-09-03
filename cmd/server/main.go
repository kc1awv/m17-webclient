package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kc1awv/m17-webclient/internal/config"
	"github.com/kc1awv/m17-webclient/internal/cors"
	log "github.com/kc1awv/m17-webclient/internal/logger"
	"github.com/kc1awv/m17-webclient/internal/reflector"
	"github.com/kc1awv/m17-webclient/internal/status"
	"github.com/kc1awv/m17-webclient/internal/transport"
	promhttp "github.com/prometheus/client_golang/prometheus/promhttp"
)

func corsMiddleware(validateOrigin func(string) bool, allowedMethods, allowedHeaders []string, next http.Handler) http.Handler {
	headers := strings.Join(allowedHeaders, ", ")
	methods := strings.Join(allowedMethods, ", ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); origin != "" {
			if validateOrigin(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", methods)
		w.Header().Set("Access-Control-Allow-Headers", headers)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load configuration", "err", err)
	}

	store := reflector.NewListStore()
	store.Init()

	originValidator := cors.NewOriginValidator(cfg.AllowedOrigins)
	wsCfg := transport.WebSocketConfig{
		OriginValidator: originValidator,
		PingInterval:    cfg.WSPingInterval,
		PongWait:        cfg.WSPongWait,
		ServerName:      cfg.ServerName,
		NewReflectorClient: func(ctx context.Context, addr, callsign string, module byte) (*reflector.ReflectorClient, error) {
			rc, err := reflector.NewReflectorClient(ctx, addr, callsign, module)
			if err != nil {
				return nil, err
			}
			rc.Designator = store.LookupDesignator(addr)
			return rc, nil
		},
	}

	addr := cfg.Address()

	manager := transport.NewSessionManager()
	if cfg.MaxSessions > 0 {
		manager.MaxSessions = cfg.MaxSessions
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			status.RecordHeartbeat(manager.Count())
		}
	}()

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store.StartReflectorUpdater(rootCtx)

	mux := http.NewServeMux()

	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := writeJSONResponse(w, map[string]string{"status": "ok"}); err != nil {
			log.Error("failed to encode health response", "err", err)
		}
	})

	mux.HandleFunc("/api/reflectors", func(w http.ResponseWriter, r *http.Request) {
		if err := writeJSONResponse(w, store.GetReflectors()); err != nil {
			log.Error("failed to encode reflector list", "err", err)
		}
	})

	mux.HandleFunc("/api/reflectors/modules", func(w http.ResponseWriter, r *http.Request) {
		slug := r.URL.Query().Get("slug")
		if slug == "" {
			http.Error(w, "missing slug", http.StatusBadRequest)
			return
		}
		modules := store.FetchModules(slug)
		if err := writeJSONResponse(w, modules); err != nil {
			log.Error("failed to encode reflector modules", "err", err)
		}
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		transport.HandleWebSocket(manager, wsCfg, w, r)
	})

	server := &http.Server{
		Addr:         addr,
		Handler:      corsMiddleware(originValidator, cfg.AllowedMethods, cfg.AllowedHeaders, mux),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	go func() {
		log.Info("M17 Web Client Server listening", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed", "err", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatal("Server shutdown failed", "err", err)
	}
}
