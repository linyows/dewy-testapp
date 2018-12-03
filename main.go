package main

import (
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/pkg/namesgenerator"
	"github.com/lestrrat-go/server-starter/listener"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var err error
	var listeners []net.Listener

	fmt.Printf("%#v\n", os.Args)

	port := os.Getenv("SERVER_STARTER_PORT")
	if len(port) > 0 {
		listeners, err = listener.ListenAll()
	} else {
		var l net.Listener
		l, err = net.Listen("tcp", ":3333")
		listeners = append(listeners, l)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen: %s\n", err)
		os.Exit(1)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().Format(time.RFC1123)
		reqPath := html.EscapeString(r.URL.Path)
		name := namesgenerator.GetRandomName(0)
		fmt.Fprintf(w, "%s -- request: %q, version: %s, name: %s\n", now, reqPath, version, name)
		io.Copy(w, r.Body)
	})

	for _, l := range listeners {
		http.Serve(l, handler)
	}

	loop := false
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)

	for loop {
		select {
		case <-sigCh:
			loop = false
		default:
			time.Sleep(time.Second)
		}
	}
}
