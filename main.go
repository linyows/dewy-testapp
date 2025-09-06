package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/lestrrat-go/server-starter/listener"
)

type AppStatus struct {
	Version   string            `json:"version"`
	Commit    string            `json:"commit"`
	BuildDate string            `json:"build_date"`
	StartTime time.Time         `json:"start_time"`
	Uptime    string            `json:"uptime"`
	Listeners []string          `json:"listeners"`
	Endpoints map[string]string `json:"endpoints"`
	Mode      string            `json:"mode"` // "server-starter" or "standalone"
}

const (
	fallbackPort = "3333"
)

var (
	version    = "0.0.0-dev"
	commit     = "none"
	date       = "unknown"
	jsonLog    = flag.Bool("json", false, "Output logs in JSON format")
	versionOpt = flag.Bool("version", false, "Show version information")
	vOpt       = flag.Bool("v", false, "Show version information")
)

func main() {
	flag.Parse()

	if *versionOpt || *vOpt {
		fmt.Printf("%s\n", version)
		os.Exit(0)
	}

	if *jsonLog {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	}

	startTime := time.Now()

	slog.Info("dewy-testapp starting", "version", version)

	var err error
	var listeners []net.Listener

	if os.Getenv("SERVER_STARTER_PORT") != "" {
		listeners, err = listener.ListenAll()
		if err != nil {
			slog.Error("Failed to get listeners from server-starter", "error", err)
			slog.Info("Falling back to standalone mode", "port", fallbackPort)
			listeners = nil
		}
	}

	var mode string
	var servers []*http.Server
	var wg sync.WaitGroup

	if len(listeners) > 0 {
		// Server-starter mode
		mode = "server-starter"
		slog.Info("Server-starter mode", "listeners_count", len(listeners))
		servers = setupServerStarterMode(version, startTime, listeners, &wg)
	} else {
		// Standalone mode (fallback)
		mode = "standalone"
		slog.Info("Standalone mode starting", "port", fallbackPort)
		servers = setupStandaloneMode(version, startTime, fallbackPort, &wg)
	}

	if len(servers) == 0 {
		slog.Error("No servers could be started")
		os.Exit(1)
	}

	slog.Info("All servers started successfully", "servers_count", len(servers), "mode", mode)

	// Graceful shutdown
	shutdown := make(chan struct{})
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		sig := <-c
		slog.Info("Received signal, shutting down servers", "signal", sig.String())

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for i, server := range servers {
			if err := server.Shutdown(ctx); err != nil {
				slog.Error("Server shutdown error", "server_index", i, "error", err)
			}
		}
		close(shutdown)
	}()

	// Wait for shutdown signal or all servers to finish
	select {
	case <-shutdown:
		slog.Info("Shutdown signal received")
	}

	wg.Wait()
	slog.Info("All servers stopped gracefully")
}

func setupServerStarterMode(version string, startTime time.Time, listeners []net.Listener, wg *sync.WaitGroup) []*http.Server {
	servers := make([]*http.Server, len(listeners))
	listenerAddrs := make([]string, len(listeners))
	endpoints := make(map[string]string)

	for i, l := range listeners {
		addr := l.Addr().String()
		listenerAddrs[i] = addr

		// Extract port number and create endpoint information
		if tcpAddr, ok := l.Addr().(*net.TCPAddr); ok {
			port := fmt.Sprintf("%d", tcpAddr.Port)
			endpoints[fmt.Sprintf("port_%s", port)] = fmt.Sprintf("http://localhost:%s", port)
		}

		mux := createHandler(version, startTime, listenerAddrs, addr, endpoints, "server-starter")
		server := &http.Server{Handler: mux}
		servers[i] = server

		wg.Add(1)
		go func(srv *http.Server, listener net.Listener, address string) {
			defer wg.Done()
			slog.Info("Starting HTTP server", "address", address, "mode", "server-starter")
			if err := srv.Serve(listener); err != http.ErrServerClosed {
				slog.Error("Server failed", "address", address, "error", err)
			}
		}(server, l, addr)

		slog.Info("Server-starter listener", "index", i, "address", addr)
	}

	return servers
}

func setupStandaloneMode(version string, startTime time.Time, port string, wg *sync.WaitGroup) []*http.Server {
	addr := ":" + port
	listenerAddrs := []string{fmt.Sprintf("0.0.0.0:%s", port)}
	endpoints := map[string]string{
		fmt.Sprintf("port_%s", port): fmt.Sprintf("http://localhost:%s", port),
	}

	mux := createHandler(version, startTime, listenerAddrs, addr, endpoints, "standalone")
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("Starting HTTP server", "address", addr, "mode", "standalone")
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("Standalone server failed", "error", err)
		}
	}()

	return []*http.Server{server}
}

func createHandler(version string, startTime time.Time, listeners []string, currentAddr string, endpoints map[string]string, mode string) *http.ServeMux {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		status := AppStatus{
			Version:   version,
			Commit:    commit,
			BuildDate: date,
			StartTime: startTime,
			Uptime:    time.Since(startTime).String(),
			Listeners: listeners,
			Endpoints: endpoints,
			Mode:      mode,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	// Version endpoint
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s\n", version)
	})

	// Listener-specific endpoint
	mux.HandleFunc("/listener", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"current_listener":   currentAddr,
			"all_listeners":      listeners,
			"version":            version,
			"mode":               mode,
			"server_starter_env": os.Getenv("SERVER_STARTER_PORT"),
			"pid":                os.Getpid(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Mode-specific endpoint
	mux.HandleFunc("/mode", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"mode":                  mode,
			"fallback_port":         fallbackPort,
			"server_starter_active": mode == "server-starter",
			"listeners_count":       len(listeners),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Root endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "dewy-testapp v%s running on %s (%s mode)\n", version, currentAddr, mode)
	})

	return mux
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
