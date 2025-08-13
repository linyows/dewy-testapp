package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	version = "0.0.0-dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	startTime := time.Now()

	log.Printf("dewy-testapp v%s starting", version)

	var err error
	var listeners []net.Listener

	if os.Getenv("SERVER_STARTER_PORT") != "" {
		listeners, err = listener.ListenAll()
		if err != nil {
			log.Printf("Failed to get listeners from server-starter: %v", err)
			log.Printf("Falling back to standalone mode on port %s", fallbackPort)
			listeners = nil
		}
	}

	var mode string
	var servers []*http.Server
	var wg sync.WaitGroup

	if len(listeners) > 0 {
		// Server-starter mode
		mode = "server-starter"
		log.Printf("Server-starter mode: received %d listeners", len(listeners))
		servers = setupServerStarterMode(version, startTime, listeners, &wg)
	} else {
		// Standalone mode (fallback)
		mode = "standalone"
		log.Printf("Standalone mode: starting on port %s", fallbackPort)
		servers = setupStandaloneMode(version, startTime, fallbackPort, &wg)
	}

	if len(servers) == 0 {
		log.Fatal("No servers could be started")
	}

	log.Printf("All %d servers started successfully in %s mode", len(servers), mode)

	// Graceful shutdown
	shutdown := make(chan struct{})
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		sig := <-c
		log.Printf("Received signal %s, shutting down servers...", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for i, server := range servers {
			if err := server.Shutdown(ctx); err != nil {
				log.Printf("Server %d shutdown error: %v", i, err)
			}
		}
		close(shutdown)
	}()

	// Wait for shutdown signal or all servers to finish
	select {
	case <-shutdown:
		log.Println("Shutdown signal received")
	}

	wg.Wait()
	log.Println("All servers stopped gracefully")
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
			log.Printf("Starting HTTP server on %s (server-starter)", address)
			if err := srv.Serve(listener); err != http.ErrServerClosed {
				log.Printf("Server on %s failed: %v", address, err)
			}
		}(server, l, addr)

		log.Printf("Server-starter listener %d: %s", i, addr)
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
		log.Printf("Starting HTTP server on %s (standalone)", addr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("Standalone server failed: %v", err)
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
