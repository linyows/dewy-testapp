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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/linyows/dewy/sock"
)

type AppStatus struct {
	Version   string            `json:"version"`
	Commit    string            `json:"commit"`
	BuildDate string            `json:"build_date"`
	StartTime time.Time         `json:"start_time"`
	Uptime    string            `json:"uptime"`
	Listeners []string          `json:"listeners"`
	Endpoints map[string]string `json:"endpoints"`
	Mode      string            `json:"mode"` // "dewy" or "standalone"
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
	portsOpt   = flag.String("ports", "", "Comma-separated list of ports to listen on (e.g., '8080,8081,8082')")
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

	mode, listeners, err := openListeners(*portsOpt)
	if err != nil {
		slog.Error("Failed to open listeners", "error", err)
		os.Exit(1)
	}
	// Under dewy server --health-before-switch, dewy probes each new process
	// on its own socket before it switches traffic to it.
	health, err := sock.HealthListener()
	if err != nil {
		slog.Error("Failed to open the health check listener", "error", err)
		os.Exit(1)
	}

	listenerAddrs := make([]string, len(listeners))
	endpoints := make(map[string]string)
	for i, l := range listeners {
		listenerAddrs[i] = l.Addr().String()
		switch a := l.Addr().(type) {
		case *net.TCPAddr:
			port := strconv.Itoa(a.Port)
			endpoints["port_"+port] = "http://localhost:" + port
		case *net.UnixAddr:
			endpoints["unix_"+a.Name] = "unix:" + a.Name
		}
	}

	var servers []*http.Server
	var wg sync.WaitGroup
	serve := func(l net.Listener, kind string) {
		addr := l.Addr().String()
		srv := &http.Server{Handler: createHandler(version, startTime, listenerAddrs, addr, endpoints, mode)}
		servers = append(servers, srv)
		wg.Add(1)
		go func() {
			defer wg.Done()
			slog.Info("Starting HTTP server", "address", addr, "mode", mode, "listener", kind)
			if err := srv.Serve(l); err != http.ErrServerClosed {
				slog.Error("Server failed", "address", addr, "error", err)
			}
		}()
	}
	for _, l := range listeners {
		serve(l, "app")
	}
	if health != nil {
		serve(health, "health")
	}

	slog.Info("All servers started successfully", "servers_count", len(servers), "mode", mode)

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	sig := <-c
	slog.Info("Received signal, shutting down servers", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i, server := range servers {
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("Server shutdown error", "server_index", i, "error", err)
		}
	}

	wg.Wait()
	slog.Info("All servers stopped gracefully")
}

// openListeners returns the listeners to serve on, in this order of
// priority:
//
//  1. The ports given with -ports.
//  2. The TCP and Unix domain sockets dewy passed in DEWY_PORT (or
//     server-starter in SERVER_STARTER_PORT).
//  3. The fallback port.
//
// Ports are opened with sock.Listen, which returns the socket dewy opened
// for the port when there is one and binds the port otherwise.
func openListeners(ports string) (string, []net.Listener, error) {
	if ps := parsePorts(ports); len(ps) > 0 {
		lns, err := listenPorts(ps)
		return "standalone", lns, err
	}
	if sock.Inherited() {
		lns, err := sock.Listeners()
		if err != nil {
			return "", nil, err
		}
		// DEWY_PORT may list only UDP sockets, which this app does not
		// serve; it then falls back like a process started without dewy.
		if len(lns) > 0 {
			return "dewy", lns, nil
		}
	}
	lns, err := listenPorts([]string{fallbackPort})
	return "standalone", lns, err
}

func listenPorts(ports []string) ([]net.Listener, error) {
	lns := make([]net.Listener, 0, len(ports))
	for _, port := range ports {
		l, err := sock.Listen("tcp", ":"+port)
		if err != nil {
			for _, o := range lns {
				_ = o.Close()
			}
			return nil, fmt.Errorf("port %s: %w", port, err)
		}
		lns = append(lns, l)
	}
	return lns, nil
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
			"current_listener": currentAddr,
			"all_listeners":    listeners,
			"version":          version,
			"mode":             mode,
			"dewy_port_env":    os.Getenv(sock.EnvPort),
			"pid":              os.Getpid(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Mode-specific endpoint
	mux.HandleFunc("/mode", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"mode":            mode,
			"fallback_port":   fallbackPort,
			"dewy_active":     mode == "dewy",
			"listeners_count": len(listeners),
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

func parsePorts(portsStr string) []string {
	if portsStr == "" {
		return nil
	}
	parts := strings.Split(portsStr, ",")
	ports := make([]string, 0, len(parts))
	for _, p := range parts {
		port := strings.TrimSpace(p)
		if port != "" {
			ports = append(ports, port)
		}
	}
	return ports
}
