package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

type syscallConn interface {
	SyscallConn() (syscall.RawConn, error)
}

// dupFD returns a duplicate of the descriptor of c, as dewy passes it to the
// application. The sock package takes ownership of the duplicate.
func dupFD(t *testing.T, c syscallConn) int {
	t.Helper()
	rc, err := c.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var fd int
	var dupErr error
	if err := rc.Control(func(s uintptr) { fd, dupErr = syscall.Dup(int(s)) }); err != nil {
		t.Fatal(err)
	}
	if dupErr != nil {
		t.Fatal(dupErr)
	}
	return fd
}

// inheritTCP binds a TCP port and returns the port and a duplicate of its
// descriptor.
func inheritTCP(t *testing.T) (port int, fd int) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().(*net.TCPAddr).Port, dupFD(t, ln.(*net.TCPListener))
}

// inheritUnix binds a Unix domain socket and returns its path and a
// duplicate of its descriptor. The socket goes under /tmp, because the
// default temporary directory on macOS is too long for a socket path.
func inheritUnix(t *testing.T) (path string, fd int) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "testapp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path = filepath.Join(dir, "app.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return path, dupFD(t, ln.(*net.UnixListener))
}

func closeAll(t *testing.T, lns []net.Listener) {
	t.Cleanup(func() {
		for _, l := range lns {
			_ = l.Close()
		}
	})
}

// TestOpenListenersInherited covers the sockets passed by dewy. The sock
// package remembers the descriptors it has taken, by number, for the life of
// the process, as dewy passes them once at startup. A descriptor closed by
// one case could be handed out again by a later dup and look taken, so every
// socket is created before the first case runs.
func TestOpenListenersInherited(t *testing.T) {
	dewyPort, dewyTCPFD := inheritTCP(t)
	dewyPath, dewyUnixFD := inheritUnix(t)
	compatPort, compatFD := inheritTCP(t)
	portsPort, portsFD := inheritTCP(t)
	httpPath, httpFD := inheritUnix(t)

	t.Run("DEWY_PORT lists TCP and Unix domain sockets", func(t *testing.T) {
		t.Setenv("DEWY_PORT", fmt.Sprintf("%d=%d;%s=%d", dewyPort, dewyTCPFD, dewyPath, dewyUnixFD))

		mode, lns, err := openListeners("")
		if err != nil {
			t.Fatalf("openListeners() error = %v", err)
		}
		closeAll(t, lns)
		if mode != "dewy" {
			t.Errorf("mode = %q, want %q", mode, "dewy")
		}
		if len(lns) != 2 {
			t.Fatalf("got %d listeners, want the TCP and the Unix domain socket", len(lns))
		}
		if a, ok := lns[0].Addr().(*net.TCPAddr); !ok || a.Port != dewyPort {
			t.Errorf("listener 0 = %v, want TCP port %d", lns[0].Addr(), dewyPort)
		}
		if a, ok := lns[1].Addr().(*net.UnixAddr); !ok || a.Name != dewyPath {
			t.Errorf("listener 1 = %v, want Unix domain socket %s", lns[1].Addr(), dewyPath)
		}
	})

	t.Run("SERVER_STARTER_PORT is read without DEWY_PORT", func(t *testing.T) {
		t.Setenv("SERVER_STARTER_PORT", fmt.Sprintf("%d=%d", compatPort, compatFD))

		mode, lns, err := openListeners("")
		if err != nil {
			t.Fatalf("openListeners() error = %v", err)
		}
		closeAll(t, lns)
		if mode != "dewy" || len(lns) != 1 || lns[0].Addr().(*net.TCPAddr).Port != compatPort {
			t.Errorf("openListeners() = %q, %v, want the socket from SERVER_STARTER_PORT", mode, lns)
		}
	})

	t.Run("-ports uses the inherited socket of the port", func(t *testing.T) {
		// The inherited socket still holds the port, so binding it again
		// would fail: the listener must be the inherited one.
		t.Setenv("DEWY_PORT", fmt.Sprintf("%d=%d", portsPort, portsFD))

		mode, lns, err := openListeners(strconv.Itoa(portsPort))
		if err != nil {
			t.Fatalf("openListeners(%d) error = %v", portsPort, err)
		}
		closeAll(t, lns)
		if mode != "standalone" || len(lns) != 1 || lns[0].Addr().(*net.TCPAddr).Port != portsPort {
			t.Errorf("openListeners(%d) = %q, %v, want the inherited socket", portsPort, mode, lns)
		}
	})

	t.Run("the handler answers over a Unix domain socket", func(t *testing.T) {
		t.Setenv("DEWY_PORT", fmt.Sprintf("%s=%d", httpPath, httpFD))

		_, lns, err := openListeners("")
		if err != nil {
			t.Fatalf("openListeners() error = %v", err)
		}
		closeAll(t, lns)
		srv := &http.Server{Handler: createHandler("1.2.3", time.Now(), []string{httpPath}, httpPath, nil, "dewy")}
		go func() { _ = srv.Serve(lns[0]) }()
		t.Cleanup(func() { _ = srv.Close() })

		client := &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				Dial: func(string, string) (net.Conn, error) { return net.Dial("unix", httpPath) },
			},
		}
		res, err := client.Get("http://localhost/version")
		if err != nil {
			t.Fatalf("GET /version over %s: %v", httpPath, err)
		}
		defer func() { _ = res.Body.Close() }()
		body, _ := io.ReadAll(res.Body)
		if string(body) != "1.2.3\n" {
			t.Errorf("GET /version = %q, want %q", body, "1.2.3\n")
		}
	})
}

func TestOpenListenersPortsWithoutDewy(t *testing.T) {
	mode, lns, err := openListeners("0, 0")
	if err != nil {
		t.Fatalf("openListeners() error = %v", err)
	}
	closeAll(t, lns)
	if mode != "standalone" || len(lns) != 2 {
		t.Errorf("openListeners() = %q with %d listeners, want 2 standalone listeners", mode, len(lns))
	}
}

func TestOpenListenersRejectsInvalidDewyPort(t *testing.T) {
	t.Setenv("DEWY_PORT", "8080")
	if _, lns, err := openListeners(""); err == nil {
		closeAll(t, lns)
		t.Error("openListeners() error = nil, want an error for an entry without a descriptor")
	}
}
